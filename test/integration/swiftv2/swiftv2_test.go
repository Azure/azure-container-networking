//go:build swiftv2

package swiftv2

import (
	"context"
	"flag"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/Azure/azure-container-networking/test/internal/kubernetes"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuberneteslib "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

const (
	pniKey                   = "kubernetes.azure.com/pod-network-instance"
	podCount                 = 2
	nodepoolKey              = "agentpool"
	podNetworkYaml           = "../manifests/swiftv2/podnetwork.yaml"
	mtpodYaml                = "../manifests/swiftv2/mtpod0.yaml"
	pniYaml                  = "../manifests/swiftv2/pni.yaml"
	maxRetryDelaySeconds     = 10
	defaultTimeoutSeconds    = 120
	defaultRetryDelaySeconds = 1
	IpsInAnotherCluster      = "172.25.0.7"
	namespace                = "default"
)

var (
	podPrefix        = flag.String("podnetworkinstance", "pni1", "the pni pod used")
	podNamespace     = flag.String("namespace", "default", "Namespace for test pods")
	nodepoolSelector = flag.String("nodelabel", "mtapool", "One of the node label and the key is agentpool")
)

/*
This test assumes that you have the current credentials loaded in your default kubeconfig for a
k8s cluster with a Linux nodepool consisting of at least 2 Linux nodes.
*** The expected nodepool name is mtapool, if the nodepool has a different name ensure that you change nodepoolSelector with:
		-nodepoolSelector="yournodepoolname"

This test checks pod to pod, pod to node, pod to Internet check

Timeout context is controled by the -timeout flag.

*/

func setupLinuxEnvironment(t *testing.T) {
	ctx := context.Background()

	t.Log("Create Clientset")
	clientset := kubernetes.MustGetClientset()

	t.Log("Create Label Selectors")
	podLabelSelector := kubernetes.CreateLabelSelector(pniKey, podPrefix)
	nodeLabelSelector := kubernetes.CreateLabelSelector(nodepoolKey, nodepoolSelector)

	t.Log("Get Nodes")
	nodes, err := kubernetes.GetNodeListByLabelSelector(ctx, clientset, nodeLabelSelector)
	if err != nil {
		t.Fatalf("could not get k8s node list: %v", err)
	}

	t.Log("Waiting for pods to be running state")
	err = kubernetes.WaitForPodsRunning(ctx, clientset, *podNamespace, podLabelSelector)
	if err != nil {
		t.Fatalf("Pods are not in running state due to %+v", err)
	}

	t.Log("Successfully created customer Linux pods")

	t.Log("Checking swiftv2 multitenant pods number")
	for _, node := range nodes.Items {
		pods, err := kubernetes.GetPodsByNode(ctx, clientset, *podNamespace, podLabelSelector, node.Name)
		if err != nil {
			t.Fatalf("could not get k8s clientset: %v", err)
		}
		if len(pods.Items) < 1 {
			t.Fatalf("No pod on node: %v", node.Name)
		}
	}

	t.Log("Linux test environment ready")
}

func GetMultitenantPodNetworkConfig(t *testing.T, ctx context.Context, kubeconfig, namespace, name string) v1alpha1.MultitenantPodNetworkConfig {
	config := kubernetes.MustGetRestConfig()
	crdClient, err := kubernetes.GetRESTClientForMultitenantCRDFromConfig(config)
	t.Logf("config is %s", config)
	if err != nil {
		t.Fatalf("failed to get multitenant crd rest client: %s", err)
	}
	var mtpnc v1alpha1.MultitenantPodNetworkConfig
	err = crdClient.Get().Namespace(namespace).Resource("multitenantpodnetworkconfigs").Name(name).Do(ctx).Into(&mtpnc)
	if err != nil {
		t.Errorf("failed to retrieve multitenantpodnetworkconfig: error: %s", err)
	}
	if mtpnc.Status.MacAddress == "" || mtpnc.Status.PrimaryIP == "" {
		t.Errorf("mtpnc.Status.MacAddress is %v or mtpnc.Status.PrimaryIP is %v and at least one of them is Empty, ",
			mtpnc.Status.MacAddress, mtpnc.Status.PrimaryIP)
	}
	return mtpnc
}

func TestSwiftv2PodToPod(t *testing.T) {
	var (
		kubeconfig string
		numNodes   int
	)

	kubeconfigPath := *kubernetes.GetKubeconfig()
	t.Logf("TestSwiftv2PodToPod kubeconfig is %v", kubeconfigPath)

	ctx := context.Background()

	t.Log("Create Clientset")
	clientset := kubernetes.MustGetClientset()
	t.Log("Get Clientset config")
	restConfig := kubernetes.MustGetRestConfig()
	t.Log("rest config is", restConfig)

	t.Log("Create Label Selectors")
	podLabelSelector := kubernetes.CreateLabelSelector(pniKey, podPrefix)

	t.Log("Successfully created customer Linux pods")

	t.Log("Checking swiftv2 multitenant pods number and get IPs")
	ipsToPing := make([]string, 0, numNodes)

	podsClient := clientset.CoreV1().Pods(namespace)
	allPods, err := podsClient.List(ctx, metav1.ListOptions{LabelSelector: podLabelSelector})
	if err != nil {
		t.Fatalf("could not get pods from clientset: %v", err)
	}
	for _, pod := range allPods.Items {
		t.Logf("Pod name is %s", pod.Name)
		mtpnc := GetMultitenantPodNetworkConfig(t, ctx, kubeconfig, pod.Namespace, pod.Name)
		if len(pod.Status.PodIPs) != 1 {
			t.Fatalf("Pod doesn't have any IP associated.")
		}
		// remove /32 from PrimaryIP
		splitcidr := strings.Split(mtpnc.Status.PrimaryIP, "/")
		if len(splitcidr) != 2 {
			t.Fatalf("Split Pods IP with its cidr failed.")
		}
		ipsToPing = append(ipsToPing, splitcidr[0])
	}
	ipsToPing = append(ipsToPing, IpsInAnotherCluster)
	t.Log("Linux test environment ready")

	for _, pod := range allPods.Items {
		for _, ip := range ipsToPing {
			t.Logf("ping from pod %q to %q", pod.Name, ip)
			result := podTest(t, ctx, clientset, pod, []string{"ping", "-c", "3", ip}, restConfig)
			if result != nil {
				t.Errorf("ping %q failed: error: %s", ip, result)
			}
		}
	}
	return
}

func podTest(t *testing.T, ctx context.Context, clientset *kuberneteslib.Clientset, srcPod v1.Pod, cmd []string, rc *restclient.Config) error {
	output, err := kubernetes.ExecCmdOnPod(ctx, clientset, srcPod.Namespace, srcPod.Name, cmd, rc)
	t.Logf(string(output))
	if err != nil {
		t.Errorf("failed to execute command on pod: %v", srcPod.Name)
	}
	return err
}
