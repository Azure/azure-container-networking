//go:build connection

package connection

import (
	"context"
	"flag"
	"fmt"
	"testing"

	"github.com/Azure/azure-container-networking/test/internal/datapath"
	"github.com/Azure/azure-container-networking/test/internal/k8sutils"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	apiv1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	WindowsDeployYamlPath = "../manifests/datapath/windows-deployment.yaml"
	podLabelKey           = "app"
	podCount              = 2
	nodepoolKey           = "agentpool"
)

var (
	podPrefix        = flag.String("podName", "datapod", "Prefix for test pods")
	podNamespace     = flag.String("namespace", "datapath-win", "Namespace for test pods")
	nodepoolSelector = flag.String("nodepoolSelector", "npwin", "Provides nodepool as a Node-Selector for pods")
)

/*
This test assumes that you have the current credentials loaded in your default kubeconfig for a
k8s cluster with a windows nodepool consisting of at least 2 windows nodes.
*** The expected nodepool name is npwin, if the nodepool has a diferent name ensure that you change nodepoolSelector with:
	-nodepoolSelector="yournodepoolname"

To run the test use one of the following commands:
go test -count=1 test/integration/datapath/datapath_win_test.go -timeout 3m -tags connection -run ^TestDatapathWin$ -tags=connection
   or
go test -count=1 test/integration/datapath/datapath_win_test.go -timeout 3m -tags connection -run ^TestDatapathWin$ -podName=acnpod -nodepoolSelector=npwina -tags=connection


This test checks pod to pod, pod to node, and pod to internet for datapath connectivity.

Timeout context is controled by the -timeout flag.

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

	if createPodFlag {
		t.Log("Creating Windows pods through deployment")
		deployment, err := k8sutils.MustParseDeployment(WindowsDeployYamlPath)
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

		deploymentsClient := clientset.AppsV1().Deployments(*podNamespace)
		err = k8sutils.MustCreateDeployment(ctx, deploymentsClient, deployment)
		if err != nil {
			require.NoError(t, err)
		}

		t.Log("Waiting for pods to be running state")
		err = k8sutils.WaitForPodsRunning(ctx, clientset, *podNamespace, podLabelSelector)
		if err != nil {
			require.NoError(t, err)
		}
		t.Log("Successfully created customer windows pods")
	} else {
		// Checks namespace already exists from previous attempt
		t.Log("Namespace already exists")

		t.Log("Checking for pods to be running state")
		err = k8sutils.WaitForPodsRunning(ctx, clientset, *podNamespace, podLabelSelector)
		if err != nil {
			require.NoError(t, err)
		}
	}
	t.Log("Checking Windows test environment ")
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
	t.Log("Windows test environment ready")

	t.Run("Windows ping tests pod -> node", func(t *testing.T) {
		// Windows ping tests between pods and node
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
			require.NoError(t, err, "Windows pod to node, ping test failed with: %+v", err)
			t.Logf("Windows pod to node, passed for node: %s", node.Name)
		}
	})

	t.Run("Windows ping tests pod -> pod", func(t *testing.T) {
		// Pod to pod same node
		for _, node := range nodes.Items {
			if node.Status.NodeInfo.OperatingSystem == string(apiv1.Windows) {
				t.Log("Windows ping tests (2) - Same Node")
				err := datapath.WindowsPodToPodPingTestSameNode(ctx, clientset, node.Name, *podNamespace, podLabelSelector, restConfig)
				require.NoError(t, err, "Windows pod to pod, same node, ping test failed with: %+v", err)
				t.Logf("Windows pod to windows pod, same node, passed for node: %s", node.ObjectMeta.Name)
			}
		}

		// Pod to pod different node
		for i := 0; i < len(nodes.Items); i++ {
			t.Log("Windows ping tests (2) - Different Node")
			firstNode := nodes.Items[i%2].Name
			secondNode := nodes.Items[(i+1)%2].Name
			err = datapath.WindowsPodToPodPingTestDiffNode(ctx, clientset, firstNode, secondNode, *podNamespace, podLabelSelector, restConfig)
			require.NoError(t, err, "Windows pod to pod, different node, ping test failed with: %+v", err)
			t.Logf("Windows pod to windows pod, different node, passed for node: %s -> %s", firstNode, secondNode)

		}
	})

	t.Run("Windows url tests pod -> internet", func(t *testing.T) {
		// From windows pod, IWR a URL
		for _, node := range nodes.Items {
			if node.Status.NodeInfo.OperatingSystem == string(apiv1.Windows) {
				t.Log("Windows ping tests (3) - Pod to Internet tests")
				err := datapath.WindowsPodToInternet(ctx, clientset, node.Name, *podNamespace, podLabelSelector, restConfig)
				require.NoError(t, err, "Windows pod to internet test failed with: %+v", err)
				t.Logf("Windows pod to Internet url tests")
			}
		}
	})
}
