//go:build swiftv2

package swiftv2

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	k8s "github.com/Azure/azure-container-networking/test/integration"
	"github.com/Azure/azure-container-networking/test/integration/goldpinger"
	"github.com/Azure/azure-container-networking/test/internal/kubernetes"
	"github.com/Azure/azure-container-networking/test/internal/retry"
	"github.com/pkg/errors"
	k8scorev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclientset "k8s.io/client-go/kubernetes"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	podLabelKey                = "kubernetes.azure.com/pod-network-instance"
	podCount                   = 2
	nodepoolKey                = "agentpool"
	LinuxDeployIPV4            = "../manifests/datapath/linux-deployment.yaml"
	podNetworkYaml             = "../manifests/swiftv2/podnetwork.yaml"
	mtpodYaml                  = "../manifests/swiftv2/mtpod0.yaml"
	pniYaml                    = "../manifests/swiftv2/pni.yaml"
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
	IpsInAnotherCluster        = "172.25.0.27"
)

var (
	podPrefix        = flag.String("podnetworkinstance", "pni1", "the pni pod used")
	podNamespace     = flag.String("namespace", "default", "Namespace for test pods")
	nodepoolSelector = flag.String("nodelabel", "mtapool", "One of the node label and the key is agentpool")
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
	podLabelSelector := kubernetes.CreateLabelSelector(podLabelKey, podPrefix)
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

func TestDatapathLinux(t *testing.T) {
	ctx := context.Background()

	t.Log("Get REST config")
	restConfig := kubernetes.MustGetRestConfig()

	t.Log("Get REST Client from REST config")

	//crdClient, err := kubernetes.GetRESTClientForMultitenantCRD(*kubernetes.Kubeconfig)

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

func GetMultitenantPodNetworkConfig(t *testing.T, ctx context.Context, kubeconfig, namespace, name string) v1alpha1.MultitenantPodNetworkConfig {
	crdClient, err := kubernetes.GetRESTClientForMultitenantCRD(*kubernetes.Kubeconfig)
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

// PodExecWithError executes a command in a pod by using its first container.
// It returns the stdout, stderr and error.
func PodExecWithError(
	t *testing.T,
	kubeconfig string,
	podName string, namespace string,
	command []string,
) (string, string, error) {
	clientcmdConfig, err := clientcmd.Load([]byte(kubeconfig))
	if err != nil {
		return "", "", fmt.Errorf("failed to load kube config: %w", err)
	}

	directClientcmdConfig := clientcmd.NewNonInteractiveClientConfig(
		*clientcmdConfig,
		"", // default context
		&clientcmd.ConfigOverrides{},
		nil, // config access
	)

	clientRestConfig, err := directClientcmdConfig.ClientConfig()
	if err != nil {
		return "", "", fmt.Errorf("failed to create kube client config: %w", err)
	}

	clientRestConfig.Timeout = 10 * time.Minute

	client, err := k8sclientset.NewForConfig(clientRestConfig)
	if err != nil {
		return "", "", fmt.Errorf("failed to create kube clientset: %w", err)
	}

	pod, err := client.CoreV1().Pods(namespace).Get(context.TODO(), podName, k8smetav1.GetOptions{})
	if err != nil {
		return "", "", fmt.Errorf("get pod: %w", err)
	}
	containerName := pod.Spec.Containers[0].Name
	req := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		Param("container", containerName)

	req.VersionedParams(&k8scorev1.PodExecOptions{
		Command:   command,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
		Container: containerName,
	}, k8sscheme.ParameterCodec)

	var stdout, stderr bytes.Buffer
	executor, err := remotecommand.NewSPDYExecutor(clientRestConfig, "POST", req.URL())
	if err != nil {
		return "", "", fmt.Errorf("NewSPDYExecutor: %w", err)
	}

	// NOTE: remotecommand is not a Kubernetes pod resource API used here, but a tool API.
	ctx := context.Background()
	// yes, 3 mins is a magic number
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	t.Logf("executing command: %s", strings.Join(command, " "))
	readStreamErr := executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})

	// FIXME(hbc): Windows validation expect stdout/stderr output even seeing error
	//             therefore we need to return the stdout/stderr output here
	stdoutRead := strings.TrimSpace(stdout.String())
	stderrRead := strings.TrimSpace(stderr.String())
	return stdoutRead, stderrRead, readStreamErr
}

func TestSwiftv2PodToPod(t *testing.T) {
	var (
		kubeconfig string
		numNodes   int
	)

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

	t.Log("Waiting for pods to be running state")
	err = kubernetes.WaitForPodsRunning(ctx, clientset, *podNamespace, podLabelSelector)
	if err != nil {
		t.Fatalf("Pods are not in running state due to %+v", err)
	}

	t.Log("Successfully created customer Linux pods")

	t.Log("Checking swiftv2 multitenant pods number and get IPs")
	allPods := make([]v1.Pod, numNodes)
	ipsToPing := make([]string, 0, numNodes)
	for _, node := range nodes.Items {
		pods, err := kubernetes.GetPodsByNode(ctx, clientset, *podNamespace, podLabelSelector, node.Name)
		if err != nil {
			t.Fatalf("could not get k8s clientset: %v", err)
		}
		if len(pods.Items) < 1 {
			t.Fatalf("No pod on node: %v", node.Name)
		}
		for _, pod := range pods.Items {
			allPods = append(allPods, pod)
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
	}
	ipsToPing = append(ipsToPing, IpsInAnotherCluster)
	t.Log("Linux test environment ready")

	for _, pod := range allPods {
		for _, ip := range ipsToPing {
			t.Logf("ping from pod %q to %q", pod.Name, ip)
			stdout, stderr, err := PodExecWithError(
				t,
				kubeconfig,
				pod.Name,
				pod.Namespace,
				[]string{"ping", "-c", "3", ip},
			)
			if err != nil {
				t.Errorf("ping %q failed: error: %s, stdout: %s, stderr: %s", ip, err, stdout, stderr)
			}
		}
	}
	return
}
