//go:build connection

package connection

import (
	"context"
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/test/integration"
	"github.com/Azure/azure-container-networking/test/integration/goldpinger"
	k8sutils "github.com/Azure/azure-container-networking/test/internal/k8sutils"
	"github.com/Azure/azure-container-networking/test/internal/retry"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	apiv1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	LinuxDeployYamlPath        = "../manifests/datapath/linux-deployment.yaml"
	podLabelKey                = "app"
	podCount                   = 2
	nodepoolKey                = "agentpool"
	maxRetryDelaySeconds       = 10
	defaultTimeoutSeconds      = 120
	defaultRetryDelaySeconds   = 1
	goldpingerRetryCount       = 24
	goldpingerDelayTimeSeconds = 5
)

var (
	podPrefix        = flag.String("podName", "goldpinger", "Prefix for test pods")
	podNamespace     = flag.String("namespace", "datapath-linux", "Namespace for test pods")
	nodepoolSelector = flag.String("nodepoolSelector", "nodepool1", "Provides nodepool as a Node-Selector for pods")
	defaultRetrier   = retry.Retrier{
		Attempts: 10,
		Delay:    defaultRetryDelaySeconds * time.Second,
	}
)

/*
This test assumes that you have the current credentials loaded in your default kubeconfig for a
k8s cluster with a Linux nodepool consisting of at least 2 Linux nodes.
*** The expected nodepool name is npwin, if the nodepool has a diferent name ensure that you change nodepoolSelector with:
	-nodepoolSelector="yournodepoolname"

To run the test use one of the following commands:
go test -count=1 test/integration/datapath/datapath_linux_test.go -timeout 3m -tags connection -run ^TestDatapathLinux$ -tags=connection
   or
go test -count=1 test/integration/datapath/datapath_linux_test.go -timeout 3m -tags connection -run ^TestDatapathLinux$ -podName=acnpod -nodepoolSelector=npwina -tags=connection


This test checks pod to pod, pod to node, and pod to internet for datapath connectivity.

Timeout context is controled by the -timeout flag.

*/

func TestDatapathLinux(t *testing.T) {
	ctx := context.Background()

	t.Log("Create Clientset")
	clientset, err := k8sutils.MustGetClientset()
	if err != nil {
		require.NoError(t, err, "could not get k8s clientset: %v", err)
	}
	t.Log("Get REST config")
	restConfig := k8sutils.MustGetRestConfig(t)

	t.Log("Create Label Selectors")

	podLabelSelector := fmt.Sprintf("%s=%s", podLabelKey, *podPrefix)
	nodeLabelSelector := fmt.Sprintf("%s=%s", nodepoolKey, *nodepoolSelector)

	t.Log("Get Nodes")
	nodes, err := k8sutils.GetNodeListByLabelSelector(ctx, clientset, nodeLabelSelector)
	if err != nil {
		require.NoError(t, err, "could not get k8s node list: %v", err)
	}

	// Test Namespace
	t.Log("Create Namespace")
	err = k8sutils.MustCreateNamespace(ctx, clientset, *podNamespace)
	createPodFlag := !(apierrors.IsAlreadyExists(err))
	t.Logf("%v", createPodFlag)

	if createPodFlag {
		t.Log("Creating Linux pods through deployment")
		deployment, err := k8sutils.MustParseDeployment(LinuxDeployYamlPath)
		if err != nil {
			require.NoError(t, err)
		}

		// Fields for overwritting existing deployment yaml.
		// Defaults from flags will not change anything
		deployment.Spec.Selector.MatchLabels[podLabelKey] = *podPrefix
		deployment.Spec.Template.ObjectMeta.Labels[podLabelKey] = *podPrefix
		deployment.Spec.Template.Spec.NodeSelector[nodepoolKey] = *nodepoolSelector
		deployment.Name = *podPrefix
		deployment.Namespace = *podNamespace

		t.Logf("deployment Spec Template is %+v", deployment.Spec.Template)
		deploymentsClient := clientset.AppsV1().Deployments(*podNamespace)
		err = k8sutils.MustCreateDeployment(ctx, deploymentsClient, deployment)
		if err != nil {
			require.NoError(t, err)
		}
		t.Logf("podNamespace is %s", *podNamespace)
		t.Logf("podLabelSelector is %s", podLabelSelector)

		t.Log("Waiting for pods to be running state")
		err = k8sutils.WaitForPodsRunning(ctx, clientset, *podNamespace, podLabelSelector)
		if err != nil {
			require.NoError(t, err)
		}
		t.Log("Successfully created customer linux pods")
	} else {
		// Checks namespace already exists from previous attempt
		t.Log("Namespace already exists")

		t.Log("Checking for pods to be running state")
		err = k8sutils.WaitForPodsRunning(ctx, clientset, *podNamespace, podLabelSelector)
		if err != nil {
			require.NoError(t, err)
		}
	}
	t.Log("Checking Linux test environment")
	for _, node := range nodes.Items {

		pods, err := k8sutils.GetPodsByNode(ctx, clientset, *podNamespace, podLabelSelector, node.Name)
		if err != nil {
			require.NoError(t, err, "could not get k8s clientset: %v", err)
		}
		if len(pods.Items) <= 1 {
			t.Logf("%s", node.Name)
			require.NoError(t, errors.New("Less than 2 pods on node"))
		}
	}
	t.Log("Linux test environment ready")

	t.Run("Linux ping tests", func(t *testing.T) {
		// Check goldpinger health
		t.Run("all pods have IPs assigned", func(t *testing.T) {
			podsClient := clientset.CoreV1().Pods(*podNamespace)

			checkPodIPsFn := func() error {
				podList, err := podsClient.List(ctx, metav1.ListOptions{LabelSelector: "app=goldpinger"})
				t.Logf("podList is %+v", podList)
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
			err := defaultRetrier.Do(ctx, checkPodIPsFn)
			if err != nil {
				t.Fatalf("not all pods were allocated IPs: %v", err)
			}
			t.Log("all pods have been allocated IPs")
		})

		t.Run("all linux pods can ping each other", func(t *testing.T) {
			pfOpts := k8s.PortForwardingOpts{
				Namespace:     "default",
				LabelSelector: "type=goldpinger-pod",
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
				t.Fatalf("could not start port forward within %ds: %v", defaultTimeoutSeconds, err)
			}
			defer pf.Stop()

			gpClient := goldpinger.Client{Host: pf.Address()}

			clusterCheckCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
			defer cancel()
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
