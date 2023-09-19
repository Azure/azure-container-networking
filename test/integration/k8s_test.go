//go:build integration

package k8s

import (
	"context"
	//"dnc/test/integration/goldpinger"
	"errors"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/test/integration/goldpinger"
	"github.com/Azure/azure-container-networking/test/internal/kubernetes"
	"github.com/Azure/azure-container-networking/test/internal/retry"
	v1 "k8s.io/client-go/kubernetes/typed/apps/v1"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	subnetIDNodeLabelEnvVar   = "DELEGATED_SUBNET_ID_NODE_LABEL"
	subnetNameNodeLabelEnvVar = "SUBNET_NAME_NODE_LABEL"

	gpFolder                 = "manifests/goldpinger"
	gpClusterRolePath        = gpFolder + "/cluster-role.yaml"
	gpClusterRoleBindingPath = gpFolder + "/cluster-role-binding.yaml"
	gpServiceAccountPath     = gpFolder + "/service-account.yaml"
	gpDaemonset              = gpFolder + "/daemonset.yaml"
	gpDeployment             = gpFolder + "/deployment.yaml"
	retryAttempts            = 60
	retryDelaySec            = 5 * time.Second
)

var (
	defaultRetrier      = retry.Retrier{Attempts: retryAttempts, Delay: retryDelaySec}
	delegatedSubnetID   = flag.String("delegated-subnet-id", "", "delegated subnet id for node labeling")
	delegatedSubnetName = flag.String("subnet-name", "", "subnet name for node labeling")
)

func shouldLabelNodes() bool {
	if *delegatedSubnetID == "" {
		*delegatedSubnetID = os.Getenv(subnetIDNodeLabelEnvVar)
	}
	if *delegatedSubnetName == "" {
		*delegatedSubnetName = os.Getenv(subnetNameNodeLabelEnvVar)
	}
	return *delegatedSubnetID != "" && *delegatedSubnetName != ""
}

/*

In order to run the tests below, you need a k8s cluster and its kubeconfig.
If no kubeconfig is passed, the test will attempt to find one in the default location for kubectl config.
The test will also attempt to label the nodes if the appropriate flags or environment variables are set.
Run the tests as follows:

go test -v . [-args test-kubeconfig=<...> delegated-subnet-id=<...> subnet-name=<...>]

todo: consider adding the following scenarios
- [x] All pods should be assigned an IP.
- [ ] All pod IPs should belong to the delegated subnet and not overlap host subnet.
- [x] All pods should be able to ping each other.
- [ ] All pods should be able to ping nodes. Daemonset with hostnetworking?
- [ ] All pods should be able to reach public internet. Enable hosts to ping in goldpinger deployment.

- [x] All scenarios above should be valid during deployment scale up
- [x] All scenarios above should be valid during deployment scale down

- [ ] All scenarios above should be valid during node scale up (i.e more nodes == more NNCs)
- [ ] All scenarios above should be valid during node scale down

todo:
  - Need hook for `az aks scale --g <resource group> --n <cluster name> --node-count <prev++> --nodepool-name <np name>`
  - Need hook for pubsub client to verify that no secondary CAs are leaked
  - Check CNS ipam pool?
  - Check NNC in apiserver?
*/

func TestPodScaling(t *testing.T) {
	clientset, err := kubernetes.MustGetClientset()
	if err != nil {
		t.Fatal(err)
	}

	restConfig := kubernetes.MustGetRestConfig(t)
	deployment, err := kubernetes.MustParseDeployment(gpDeployment)
	if err != nil {
		t.Fatal(err)
	}

	daemonset, err := kubernetes.MustParseDaemonSet(gpDaemonset)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	if shouldLabelNodes() {
		kubernetes.MustLabelSwiftNodes(ctx, t, clientset, *delegatedSubnetID, *delegatedSubnetName)
	} else {
		t.Log("swift node labels not passed or set. skipping labeling")
	}

	rbacCleanUpFn, err := kubernetes.MustSetUpClusterRBAC(ctx, clientset, gpClusterRolePath, gpClusterRoleBindingPath, gpServiceAccountPath)
	if err != nil {
		t.Log(os.Getwd())
		t.Fatal(err)
	}

	deploymentsClient := clientset.AppsV1().Deployments(deployment.Namespace)
	err = kubernetes.MustCreateDeployment(ctx, deploymentsClient, deployment)
	if err != nil {
		t.Fatal(err)
	}

	daemonsetClient := clientset.AppsV1().DaemonSets(daemonset.Namespace)
	err = kubernetes.MustCreateDaemonset(ctx, daemonsetClient, daemonset)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		t.Log("cleaning up resources")
		rbacCleanUpFn()

		if err := deploymentsClient.Delete(ctx, deployment.Name, metav1.DeleteOptions{}); err != nil {
			t.Log(err)
		}

		if err := daemonsetClient.Delete(ctx, daemonset.Name, metav1.DeleteOptions{}); err != nil {
			t.Log(err)
		}
	})

	podsClient := clientset.CoreV1().Pods(deployment.Namespace)

	gpPodScaleCounts := []int{2, 10, 100, 2}
	for _, c := range gpPodScaleCounts {
		count := c

		t.Run(fmt.Sprintf("replica count %d", count), func(t *testing.T) {
			replicaCtx, cancel := context.WithTimeout(ctx, (retryAttempts+1)*retryDelaySec)
			defer cancel()

			if err := updateReplicaCount(t, replicaCtx, deploymentsClient, deployment.Name, count); err != nil {
				t.Fatalf("could not scale deployment: %v", err)
			}

			t.Log("checking that all pods have IPs assigned")

			checkPodIPsFn := func() error {
				podList, err := podsClient.List(ctx, metav1.ListOptions{LabelSelector: "app=goldpinger"})
				if err != nil {
					return err
				}

				if len(podList.Items) == 0 {
					return errors.New("no pods scheduled")
				}

				for _, pod := range podList.Items {
					if pod.Status.Phase == apiv1.PodPending {
						return errors.New("some pods still pending")
					}
				}

				for _, pod := range podList.Items {
					if pod.Status.PodIP == "" {
						return errors.New("a pod has not been allocated an IP")
					}
				}

				return nil
			}

			if err := defaultRetrier.Do(ctx, checkPodIPsFn); err != nil {
				t.Fatalf("not all pods were allocated IPs: %v", err)
			}

			t.Log("all pods have been allocated IPs")
			t.Log("checking that all pods can ping each other")

			clusterCheckCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
			defer cancel()

			pfOpts := PortForwardingOpts{
				Namespace:     "default",
				LabelSelector: "type=goldpinger-pod",
				LocalPort:     9090,
				DestPort:      8080,
			}

			pingCheckFn := func() error {
				pf, err := NewPortForwarder(restConfig, t, pfOpts)
				if err != nil {
					t.Fatalf("could not build port forwarder: %v", err)
				}

				portForwardCtx, cancel := context.WithTimeout(ctx, (retryAttempts+1)*retryDelaySec)
				defer cancel()

				portForwardFn := func() error {
					t.Log("attempting port forward")

					if err := pf.Forward(portForwardCtx); err != nil {
						return fmt.Errorf("could not start port forward: %w", err)
					}

					return nil
				}

				if err := defaultRetrier.Do(portForwardCtx, portForwardFn); err != nil {
					t.Fatalf("could not start port forward within %v: %v", retryDelaySec.String(), err)
				}

				go pf.KeepAlive(clusterCheckCtx)

				defer pf.Stop()

				gpClient := goldpinger.Client{Host: pf.Address()}

				clusterState, err := gpClient.CheckAll(clusterCheckCtx)
				if err != nil {
					return fmt.Errorf("could not check all goldpinger pods: %w", err)
				}

				stats := goldpinger.ClusterStats(clusterState)
				stats.PrintStats()
				if stats.AllPingsHealthy() {
					return nil
				}

				return errors.New("not all pings are healthy")
			}

			if err := defaultRetrier.Do(clusterCheckCtx, pingCheckFn); err != nil {
				t.Fatalf("cluster could not reach healthy state: %v", err)
			}

			t.Log("all pings successful!")
		})
	}
}

func updateReplicaCount(t *testing.T, ctx context.Context, deployments v1.DeploymentInterface, name string, replicas int) error {
	return defaultRetrier.Do(ctx, func() error {
		res, err := deployments.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		t.Logf("setting deployment %s to %d replicas", name, replicas)
		res.Spec.Replicas = kubernetes.Int32ToPtr(int32(replicas))
		_, err = deployments.Update(ctx, res, metav1.UpdateOptions{})
		return err
	})
}
