//go:build connection

package connection

import (
	"context"
	"flag"
	"net"
	"testing"
	"time"

	k8s "github.com/Azure/azure-container-networking/test/integration"
	"github.com/Azure/azure-container-networking/test/integration/goldpinger"
	"github.com/Azure/azure-container-networking/test/internal/kubernetes"
	"github.com/Azure/azure-container-networking/test/internal/retry"
	"github.com/pkg/errors"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	podLabelKey                = "app"
	podCount                   = 2
	nodepoolKey                = "agentpool"
	LinuxDeployIPV4            = "../manifests/datapath/linux-deployment.yaml"
	LinuxDeployIPv6            = "../manifests/datapath/linux-deployment-ipv6.yaml"
	maxRetryDelaySeconds       = 10
	defaultTimeoutSeconds      = 120
	defaultRetryDelaySeconds   = 1
	goldpingerRetryCount       = 24
	goldpingerDelayTimeSeconds = 5
	gpFolder                   = "../manifests/goldpinger"
	gpClusterRolePath          = gpFolder + "/cluster-role.yaml"
	gpClusterRoleBindingPath   = gpFolder + "/cluster-role-binding.yaml"
	gpServiceAccountPath       = gpFolder + "/service-account.yaml"
	gpDaemonset                = gpFolder + "/daemonset.yaml"
	gpDaemonsetIPv6            = gpFolder + "/daemonset-ipv6.yaml"
	gpDeployment               = gpFolder + "/deployment.yaml"
)

var (
	podPrefix        = flag.String("podName", "goldpinger", "Prefix for test pods")
	podNamespace     = flag.String("namespace", "default", "Namespace for test pods")
	nodepoolSelector = flag.String("nodepoolSelector", "nodepool1", "Provides nodepool as a Linux Node-Selector for pods")
	// TODO: add flag to support dual nic scenario
	isDualStack    = flag.Bool("isDualStack", false, "whether system supports dualstack scenario")
	defaultRetrier = retry.Retrier{
		Attempts: 10,
		Delay:    defaultRetryDelaySeconds * time.Second,
	}
)

/*
This test assumes that you have the current credentials loaded in your default kubeconfig for a
k8s cluster with a Linux nodepool consisting of at least 2 Linux nodes.
*** The expected nodepool name is nodepool1, if the nodepool has a different name ensure that you change nodepoolSelector with:
		-nodepoolSelector="yournodepoolname"

To run the test use one of the following commands:
go test -count=1 test/integration/datapath/datapath_linux_test.go -timeout 3m -tags connection -run ^TestDatapathLinux$ -tags=connection,integration
   or
go test -count=1 test/integration/datapath/datapath_linux_test.go -timeout 3m -tags connection -run ^TestDatapathLinux$ -podName=acnpod -nodepoolSelector=aks-pool1 -tags=connection,integration


This test checks pod to pod, pod to node, pod to Internet check

Timeout context is controled by the -timeout flag.

*/

func setupLinuxEnvironment(t *testing.T) {
	ctx := context.Background()

	t.Log("Create Clientset")
	clientset := kubernetes.MustGetClientset()

	t.Log("Create Label Selectors")
	podLabelSelector := kubernetes.CreateLabelSelector(podLabelKey, podPrefix)
	nodeLabelSelector := kubernetes.CreateLabelSelector(nodepoolKey, nodepoolSelector)

	t.Log("Get Nodes")
	nodes, err := kubernetes.GetNodeListByLabelSelector(ctx, clientset, nodeLabelSelector)
	if err != nil {
		t.Fatalf("could not get k8s node list: %v", err)
	}

	t.Log("Creating Linux pods through deployment")

	// run goldpinger ipv4 and ipv6 test cases saperately
	var daemonset appsv1.DaemonSet
	var deployment appsv1.Deployment

	if *isDualStack {
		deployment = kubernetes.MustParseDeployment(LinuxDeployIPv6)
		daemonset = kubernetes.MustParseDaemonSet(gpDaemonsetIPv6)
	} else {
		deployment = kubernetes.MustParseDeployment(LinuxDeployIPV4)
		daemonset = kubernetes.MustParseDaemonSet(gpDaemonset)
	}

	// setup common RBAC, ClusteerRole, ClusterRoleBinding, ServiceAccount
	rbacSetupFn := kubernetes.MustSetUpClusterRBAC(ctx, clientset, gpClusterRolePath, gpClusterRoleBindingPath, gpServiceAccountPath)

	// Fields for overwritting existing deployment yaml.
	// Defaults from flags will not change anything
	deployment.Spec.Selector.MatchLabels[podLabelKey] = *podPrefix
	deployment.Spec.Template.ObjectMeta.Labels[podLabelKey] = *podPrefix
	deployment.Spec.Template.Spec.NodeSelector[nodepoolKey] = *nodepoolSelector
	deployment.Name = *podPrefix
	deployment.Namespace = *podNamespace
	daemonset.Namespace = *podNamespace

	deploymentsClient := clientset.AppsV1().Deployments(*podNamespace)
	kubernetes.MustCreateDeployment(ctx, deploymentsClient, deployment)

	daemonsetClient := clientset.AppsV1().DaemonSets(daemonset.Namespace)
	kubernetes.MustCreateDaemonset(ctx, daemonsetClient, daemonset)

	t.Cleanup(func() {
		t.Log("cleaning up resources")
		rbacSetupFn()

		if err := deploymentsClient.Delete(ctx, deployment.Name, metav1.DeleteOptions{}); err != nil {
			t.Log(err)
		}

		if err := daemonsetClient.Delete(ctx, daemonset.Name, metav1.DeleteOptions{}); err != nil {
			t.Log(err)
		}
	})

	t.Log("Waiting for pods to be running state")
	err = kubernetes.WaitForPodsRunning(ctx, clientset, *podNamespace, podLabelSelector)
	if err != nil {
		t.Fatalf("Pods are not in running state due to %+v", err)
	}

	if *isDualStack {
		t.Log("Successfully created customer dualstack Linux pods")
	} else {
		t.Log("Successfully created customer singlestack Linux pods")
	}

	t.Log("Checking Linux test environment")
	for _, node := range nodes.Items {
		pods, err := kubernetes.GetPodsByNode(ctx, clientset, *podNamespace, podLabelSelector, node.Name)
		if err != nil {
			t.Fatalf("could not get k8s clientset: %v", err)
		}
		if len(pods.Items) <= 1 {
			t.Fatalf("Less than 2 pods on node: %v", node.Name)
		}
	}

	t.Log("Linux test environment ready")
}

func TestDatapathLinux(t *testing.T) {
	ctx := context.Background()

	t.Log("Get REST config")
	restConfig := kubernetes.MustGetRestConfig()

	t.Log("Create Clientset")
	clientset := kubernetes.MustGetClientset()

	setupLinuxEnvironment(t)
	podLabelSelector := kubernetes.CreateLabelSelector(podLabelKey, podPrefix)

	t.Run("Linux ping tests", func(t *testing.T) {
		// Check goldpinger health
		t.Run("all pods have IPs assigned", func(t *testing.T) {
			err := kubernetes.WaitForPodsRunning(ctx, clientset, *podNamespace, podLabelSelector)
			if err != nil {
				t.Fatalf("Pods are not in running state due to %+v", err)
			}
			t.Log("all pods have been allocated IPs")
		})

		if *isDualStack {
			t.Run("Linux dualstack overlay tests", func(t *testing.T) {
				t.Run("test dualstack overlay", func(t *testing.T) {
					podsClient := clientset.CoreV1().Pods(*podNamespace)

					checkPodIPsFn := func() error {
						podList, err := podsClient.List(ctx, metav1.ListOptions{LabelSelector: "app=goldpinger"})
						if err != nil {
							return err
						}

						for _, pod := range podList.Items {
							podIPs := pod.Status.PodIPs
							if len(podIPs) < 2 {
								return errors.New("a pod only gets one IP")
							}
							if net.ParseIP(podIPs[0].IP).To4() == nil || net.ParseIP(podIPs[1].IP).To16() == nil {
								return errors.New("a pod does not have both ipv4 and ipv6 address")
							}
						}
						return nil
					}
					err := defaultRetrier.Do(ctx, checkPodIPsFn)
					if err != nil {
						t.Fatalf("dualstack overlay pod properties check is failed due to: %v", err)
					}

					t.Log("all dualstack linux pods properties have been verified")
				})
			})
		}

		t.Run("all linux pods can ping each other", func(t *testing.T) {
			clusterCheckCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
			defer cancel()

			pfOpts := k8s.PortForwardingOpts{
				Namespace:     *podNamespace,
				LabelSelector: podLabelSelector,
				LocalPort:     9090,
				DestPort:      8080,
			}

			pf, err := k8s.NewPortForwarder(restConfig, t, pfOpts)
			if err != nil {
				t.Fatal(err)
			}

			portForwardCtx, cancel := context.WithTimeout(ctx, defaultTimeoutSeconds*time.Second)
			defer cancel()

			portForwardFn := func() error {
				err := pf.Forward(portForwardCtx)
				if err != nil {
					t.Logf("unable to start port forward: %v", err)
					return err
				}
				return nil
			}

			if err := defaultRetrier.Do(portForwardCtx, portForwardFn); err != nil {
				t.Fatalf("could not start port forward within %d: %v", defaultTimeoutSeconds, err)
			}
			defer pf.Stop()

			gpClient := goldpinger.Client{Host: pf.Address()}
			clusterCheckFn := func() error {
				clusterState, err := gpClient.CheckAll(clusterCheckCtx)
				if err != nil {
					return err
				}
				stats := goldpinger.ClusterStats(clusterState)
				stats.PrintStats()
				if stats.AllPingsHealthy() {
					return nil
				}

				return errors.New("not all pings are healthy")
			}
			retrier := retry.Retrier{Attempts: goldpingerRetryCount, Delay: goldpingerDelayTimeSeconds * time.Second}
			if err := retrier.Do(clusterCheckCtx, clusterCheckFn); err != nil {
				t.Fatalf("goldpinger pods network health could not reach healthy state after %d seconds: %v", goldpingerRetryCount*goldpingerDelayTimeSeconds, err)
			}

			t.Log("all pings successful!")
		})
	})
}
