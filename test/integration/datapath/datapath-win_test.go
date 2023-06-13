package connection

import (
	"context"
	"flag"
	"fmt"
	"testing"

	"github.com/Azure/azure-container-networking/test/internal/datapath"
	"github.com/Azure/azure-container-networking/test/internal/k8sutils"
	"github.com/stretchr/testify/require"

	apiv1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	WindowsDeployYamlPath = "../manifests/datapath/windowsdeploy.yaml"
	podLabelKey           = "app"
	podCount              = 2
	nodepoolKey           = "agentpool"
)

var (
	podPrefix            = flag.String("podName", "datapod", "Prefix for test pods")
	podNamespace         = flag.String("namespace", "datapath-win", "Namespace for test pods")
	nodepoolNodeSelector = flag.String("nodepoolLabelSelector", "npwin", "Provides nodepool as a Node-Selector for pods")
)

/*
This test assumes that you have the current credentials loaded in your kubeconfig for a
k8s cluster with a windows nodepool consisting of at least 2 windows nodes.

To run the test use the following command as an example:
go test -timeout 3m -count=1 test/integration/datapath/datapath-win_test.go -podName=acnpod -nodepoolLabelSelector=npwina

This test checks pod to pod, pod to node, and pod to internet for datapath connectivity.

Timeout context is controled by the -timeout flag.
***Test takes 70s ( 35s for test - 35s for image creation)

*/

func TestDatapathWin(t *testing.T) {
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
	nodeLabelSelector := fmt.Sprintf("%s=%s", nodepoolKey, *nodepoolNodeSelector)

	// Get NodeList WindowsNodePoolName
	t.Log("Get Nodes")
	nodes, err := k8sutils.GetNodeListByLabelSelector(ctx, clientset, nodeLabelSelector)
	if err != nil {
		require.NoError(t, err, "could not get k8s node list: %v", err)
	}

	// Test Namespace
	t.Log("Create Namespace")
	err = k8sutils.CreateNamespace(ctx, clientset, *podNamespace)
	createPodFlag := apierrors.IsAlreadyExists(err)

	if !createPodFlag {
		t.Log("Creating Windows pods through deployment")
		deployment, err := k8sutils.MustParseDeployment(WindowsDeployYamlPath)
		if err != nil {
			t.Fatal(err)
		}

		// Fields for overwritting existing deployment yaml.
		// Defaults from flags will not change anything
		deployment.Spec.Selector.MatchLabels[podLabelKey] = *podPrefix
		deployment.Spec.Template.ObjectMeta.Labels[podLabelKey] = *podPrefix
		deployment.Spec.Template.Spec.NodeSelector[nodepoolKey] = *nodepoolNodeSelector
		deployment.Namespace = *podNamespace

		deploymentsClient := clientset.AppsV1().Deployments(*podNamespace)
		err = k8sutils.MustCreateDeployment(ctx, deploymentsClient, deployment)
		if err != nil {
			t.Fatal(err)
		}

		t.Log("Checking pods are running")
		err = k8sutils.WaitForPodsRunning(ctx, clientset, *podNamespace, podLabelSelector)
		if err != nil {
			t.Fatal(err)
		}
		t.Log("Successfully created customer windows pods")
	} else if createPodFlag {
		// Checks namespace already exists from previous test
		t.Log("Namespace already exists")

		t.Log("Checking Windows test environment ")
		for _, node := range nodes.Items {

			pods, err := k8sutils.GetPodsByNode(ctx, clientset, *podNamespace, podLabelSelector, node.Name)
			if err != nil {
				require.NoError(t, err, "could not get k8s clientset: %v", err)
			}
			if len(pods.Items) < 2 {
				require.NoError(t, err, "Only %d pods on node %s, requires at least 2 pods", len(pods.Items), node.Name)
			}
		}
		t.Log("Windows test environment ready")
	} else {
		t.Fatal("Create test environment skipped. Tearing test down.")
	}

	t.Run("Windows ping tests pod -> node", func(t *testing.T) {
		// Windows ping tests between pods and node
		t.Log("Windows Pod to Host Ping tests")
		for _, node := range nodes.Items {
			t.Log("Windows ping tests (1)")
			nodeIP := ""
			for _, address := range node.Status.Addresses {
				if address.Type == "InternalIP" {
					nodeIP = address.Address
					// Multiple addresses exist, break once Internal IP found.
					// Cannot call directly
					break
				}
			}

			err := datapath.WindowsPodToNode(ctx, clientset, node.Name, nodeIP, *podNamespace, podLabelSelector, restConfig)
			require.NoError(t, err, "Windows pod to node, ping test failed with %+v", err)
			t.Logf("Windows pod to node, passed for node: %s", node.Name)
		}
	})

	t.Run("Windows ping tests pod -> pod", func(t *testing.T) {
		// Get NodeList WindowsNodePoolName
		nodes, err := k8sutils.GetNodeListByLabelSelector(ctx, clientset, nodeLabelSelector)
		if err != nil {
			require.NoError(t, err, "could not get k8s node list: %v", err)
		}

		// Windows pod ping tests
		for _, node := range nodes.Items {
			if node.Status.NodeInfo.OperatingSystem == string(apiv1.Windows) {
				// Pod to pod same node
				t.Log("Windows ping tests (2) - Same Node")
				err := datapath.WindowsPodToPodPingTestSameNode(ctx, clientset, node.Name, *podNamespace, podLabelSelector, restConfig)
				require.NoError(t, err, "Windows pod to pod, same node, ping test failed with %+v", err)
				t.Logf("Windows pod to windows pod, same node, passed for node: %s", node.ObjectMeta.Name)
			}
		}

		// Pod to pod different node
		for i := 0; i < len(nodes.Items); i++ {
			t.Log("Windows ping tests (2) - Different Node")
			firstNode := nodes.Items[i%2].Name
			secondNode := nodes.Items[(i+1)%2].Name
			err = datapath.WindowsPodToPodPingTestDiffNode(ctx, clientset, firstNode, secondNode, *podNamespace, podLabelSelector, restConfig)
			require.NoError(t, err, "Windows pod to pod, different node, ping test failed with %+v", err)
			t.Logf("Windows pod to windows pod, different node, passed for node: %s -> %s", firstNode, secondNode)

		}
	})

	t.Run("Windows url tests pod -> internet", func(t *testing.T) {
		// From windows pod to IWR a URL
		t.Log("Windows ping tests (3) - Pod to Internet tests")
		nodes, err := k8sutils.GetNodeListByLabelSelector(ctx, clientset, nodeLabelSelector)
		if err != nil {
			require.NoError(t, err, "could not get k8s node list: %v", err)
		}
		for _, node := range nodes.Items {
			if node.Status.NodeInfo.OperatingSystem == string(apiv1.Windows) {
				err := datapath.WindowsPodToInternet(ctx, clientset, node.Name, *podNamespace, podLabelSelector, restConfig)
				require.NoError(t, err, "Windows pod to internet url %+v", err)
				t.Logf("Windows pod to Internet url tests")
			}
		}
	})
}
