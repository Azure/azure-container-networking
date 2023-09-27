//go:build connection

package connection

import (
	"context"
	"flag"
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/test/internal/datapath"
	"github.com/Azure/azure-container-networking/test/internal/kubernetes"
	"github.com/Azure/azure-container-networking/test/validate"
	"github.com/stretchr/testify/require"
	apiv1 "k8s.io/api/core/v1"
)

const (
	WindowsDeployYamlPath = "../manifests/datapath/windows-deployment.yaml"
	podLabelKey           = "app"
	podCount              = 2
	nodepoolKey           = "agentpool"
)

var (
	podPrefix        = flag.String("podName", "datapod", "Prefix for test pods")
	podNamespace     = flag.String("namespace", "windows-datapath-test", "Namespace for test pods")
	nodepoolSelector = flag.String("nodepoolSelector", "npwin", "Provides nodepool as a windows Node-Selector for pods")
	restartKubeproxy = flag.Bool("restartKubeproxy", false, "restarts kubeproxy on the windows node")
)

/*
This test assumes that you have the current credentials loaded in your default kubeconfig for a
k8s cluster with a windows nodepool consisting of at least 2 windows nodes.
*** The expected nodepool name is npwin, if the nodepool has a diferent name ensure that you change nodepoolSelector with:
	-nodepoolSelector="yournodepoolname"

To run the test use one of the following commands:
go test -count=1 test/integration/datapath/datapath_windows_test.go -timeout 3m -tags connection -run ^TestDatapathWin$ -tags=connection
   or
go test -count=1 test/integration/datapath/datapath_windows_test.go -timeout 3m -tags connection -run ^TestDatapathWin$ -podName=acnpod -nodepoolSelector=npwina -tags=connection


This test checks pod to pod, pod to node, and pod to internet for datapath connectivity.

Timeout context is controled by the -timeout flag.

*/

func setupWindowsEnvironment(t *testing.T) {
	ctx := context.Background()

	t.Log("Get REST config")
	restConfig := kubernetes.MustGetRestConfig(t)

	t.Log("Create Clientset")
	clientset, err := kubernetes.MustGetClientset()
	if err != nil {
		t.Fatal(err)
	}

	if *restartKubeproxy {
		validator, err := validate.CreateValidator(ctx, clientset, restConfig, *podNamespace, "cniv2", false, "windows")
		require.NoError(t, err)
		err = validator.RestartKubeProxyService(ctx)
		require.NoError(t, err)
	}

	t.Log("Create Label Selectors")
	podLabelSelector := kubernetes.CreateLabelSelector(podLabelKey, podPrefix)
	nodeLabelSelector := kubernetes.CreateLabelSelector(nodepoolKey, nodepoolSelector)

	t.Log("Get Nodes")
	nodes, err := kubernetes.GetNodeListByLabelSelector(ctx, clientset, nodeLabelSelector)
	if err != nil {
		t.Fatal(err)
	}

	// Create namespace if it doesn't exist
	namespaceExists, err := kubernetes.NamespaceExists(ctx, clientset, *podNamespace)
	if err != nil {
		t.Fatalf("failed to check if namespace %s exists due to: %v", *podNamespace, err)
	}

	if !namespaceExists {
		// Test Namespace
		t.Log("Create Namespace")
		err := kubernetes.MustCreateNamespace(ctx, clientset, *podNamespace)
		if err != nil {
			t.Fatalf("failed to create pod namespace %s due to: %v", *podNamespace, err)
		}

		t.Log("Creating Windows pods through deployment")
		deployment, err := kubernetes.MustParseDeployment(WindowsDeployYamlPath)
		if err != nil {
			t.Fatal(err)
		}

		// Fields for overwritting existing deployment yaml.
		// Defaults from flags will not change anything
		deployment.Spec.Selector.MatchLabels[podLabelKey] = *podPrefix
		deployment.Spec.Template.ObjectMeta.Labels[podLabelKey] = *podPrefix
		deployment.Spec.Template.Spec.NodeSelector[nodepoolKey] = *nodepoolSelector
		deployment.Name = *podPrefix
		deployment.Namespace = *podNamespace

		deploymentsClient := clientset.AppsV1().Deployments(*podNamespace)
		err = kubernetes.MustCreateDeployment(ctx, deploymentsClient, deployment)
		if err != nil {
			t.Fatal(err)
		}

		t.Log("Waiting for pods to be running state")
		err = kubernetes.WaitForPodsRunning(ctx, clientset, *podNamespace, podLabelSelector)
		if err != nil {
			t.Fatal(err)
		}
		t.Log("Successfully created customer windows pods")
	} else {
		// Checks namespace already exists from previous attempt
		t.Log("Namespace already exists")

		t.Log("Checking for pods to be running state")
		err = kubernetes.WaitForPodsRunning(ctx, clientset, *podNamespace, podLabelSelector)
		if err != nil {
			t.Fatal(err)
		}
	}

	t.Log("Checking Windows test environment")
	for _, node := range nodes.Items {

		pods, err := kubernetes.GetPodsByNode(ctx, clientset, *podNamespace, podLabelSelector, node.Name)
		if err != nil {
			t.Fatal(err)
		}
		if len(pods.Items) <= 1 {
			t.Fatalf("Less than 2 pods on node: %v", node.Name)
		}
	}
	t.Log("Windows test environment ready")
}

func TestDatapathWin(t *testing.T) {
	ctx := context.Background()

	t.Log("Get REST config")
	restConfig := kubernetes.MustGetRestConfig(t)

	t.Log("Create Clientset")
	clientset, err := kubernetes.MustGetClientset()
	if err != nil {
		t.Fatalf("could not get k8s clientset: %v", err)
	}

	setupWindowsEnvironment(t)
	podLabelSelector := kubernetes.CreateLabelSelector(podLabelKey, podPrefix)
	nodeLabelSelector := kubernetes.CreateLabelSelector(nodepoolKey, nodepoolSelector)

	t.Log("Get Nodes")
	nodes, err := kubernetes.GetNodeListByLabelSelector(ctx, clientset, nodeLabelSelector)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("Windows ping tests pod -> node", func(t *testing.T) {
		// Windows ping tests between pods and node
		for _, node := range nodes.Items {
			t.Log("Windows ping tests (1)")
			nodeIP := ""
			nodeIPv6 := ""
			for _, address := range node.Status.Addresses {
				if address.Type == "InternalIP" {
					nodeIP = address.Address
					if net.ParseIP(address.Address).To16() != nil {
						nodeIPv6 = address.Address
					}
					break
				}
			}

			err := datapath.WindowsPodToNode(ctx, clientset, node.Name, nodeIP, *podNamespace, podLabelSelector, restConfig)
			if err != nil {
				require.NoError(t, err)
			}
			t.Logf("Windows pod to node, passed for node: %s", node.Name)

			// windows ipv6 connectivity
			if nodeIPv6 != "" {
				err := datapath.WindowsPodToNode(ctx, clientset, node.Name, nodeIPv6, *podNamespace, podLabelSelector, restConfig)
				if err != nil {
					require.NoError(t, err)
				}
				t.Logf("Windows pod to node via ipv6, passed for node: %s", node.Name)
			}
		}
	})

	t.Run("Windows ping tests pod -> pod", func(t *testing.T) {
		// Pod to pod same node
		for _, node := range nodes.Items {
			if node.Status.NodeInfo.OperatingSystem == string(apiv1.Windows) {
				t.Log("Windows ping tests (2) - Same Node")
				err := datapath.WindowsPodToPodPingTestSameNode(ctx, clientset, node.Name, *podNamespace, podLabelSelector, restConfig)
				if err != nil {
					require.NoError(t, err)
				}
				t.Logf("Windows pod to windows pod, same node, passed for node: %s", node.ObjectMeta.Name)
			}
		}

		// Pod to pod different node
		for i := 0; i < len(nodes.Items); i++ {
			t.Log("Windows ping tests (2) - Different Node")
			firstNode := nodes.Items[i%2].Name
			secondNode := nodes.Items[(i+1)%2].Name
			err := datapath.WindowsPodToPodPingTestDiffNode(ctx, clientset, firstNode, secondNode, *podNamespace, podLabelSelector, restConfig)
			if err != nil {
				require.NoError(t, err)
			}
			t.Logf("Windows pod to windows pod, different node, passed for node: %s -> %s", firstNode, secondNode)

		}
	})

	t.Run("Windows url tests pod -> internet", func(t *testing.T) {
		// From windows pod, IWR a URL
		for _, node := range nodes.Items {
			if node.Status.NodeInfo.OperatingSystem == string(apiv1.Windows) {
				t.Log("Windows ping tests (3) - Pod to Internet tests")
				err := datapath.WindowsPodToInternet(ctx, clientset, node.Name, *podNamespace, podLabelSelector, restConfig)
				if err != nil {
					require.NoError(t, err)
				}
				t.Logf("Windows pod to Internet url tests")
			}
		}
	})
}
