//go:build lrp

package lrp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	k8s "github.com/Azure/azure-container-networking/test/integration"
	"github.com/Azure/azure-container-networking/test/integration/prometheus"
	"github.com/Azure/azure-container-networking/test/internal/kubernetes"
	"github.com/Azure/azure-container-networking/test/internal/retry"
	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	ciliumClientset "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/rand"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

const (
	ciliumConfigmapName       = "cilium-config"
	ciliumManifestsDir        = "../manifests/cilium/lrp/"
	enableLRPFlag             = "enable-local-redirect-policy"
	kubeSystemNamespace       = "kube-system"
	dnsService                = "kube-dns"
	retryAttempts             = 10
	retryDelay                = 5 * time.Second
	nodeLocalDNSLabelSelector = "k8s-app=node-local-dns"
	clientLabelSelector       = "lrp-test=true"
	coreDNSRequestCountTotal  = "coredns_dns_request_count_total"
	clientContainer           = "no-op"
	// Port constants for prometheus endpoints
	initialPrometheusPort   = 9253
	recreatedPrometheusPort = 9254
)

var (
	defaultRetrier                 = retry.Retrier{Attempts: retryAttempts, Delay: retryDelay}
	nodeLocalDNSDaemonsetPath      = ciliumManifestsDir + "node-local-dns-ds.yaml"
	tempNodeLocalDNSDaemonsetPath  = ciliumManifestsDir + "temp-daemonset.yaml"
	nodeLocalDNSConfigMapPath      = ciliumManifestsDir + "config-map.yaml"
	nodeLocalDNSServiceAccountPath = ciliumManifestsDir + "service-account.yaml"
	nodeLocalDNSServicePath        = ciliumManifestsDir + "service.yaml"
	lrpPath                        = ciliumManifestsDir + "lrp.yaml"
	numClients                     = 4
	clientPath                     = ciliumManifestsDir + "client-ds.yaml"
)

// getPrometheusAddress returns the prometheus metrics URL for the given port
func getPrometheusAddress(port int) string {
	return fmt.Sprintf("http://localhost:%d/metrics", port)
}

func setupLRP(t *testing.T, ctx context.Context) (*corev1.Pod, func()) {
	var cleanUpFns []func()
	success := false
	cleanupFn := func() {
		for len(cleanUpFns) > 0 {
			cleanUpFns[len(cleanUpFns)-1]()
			cleanUpFns = cleanUpFns[:len(cleanUpFns)-1]
		}
	}
	defer func() {
		if !success {
			cleanupFn()
		}
	}()

	config := kubernetes.MustGetRestConfig()
	cs := kubernetes.MustGetClientset()

	ciliumCS, err := ciliumClientset.NewForConfig(config)
	require.NoError(t, err)

	svc, err := kubernetes.GetService(ctx, cs, kubeSystemNamespace, dnsService)
	require.NoError(t, err)
	kubeDNS := svc.Spec.ClusterIP

	// ensure lrp flag is enabled
	ciliumCM, err := kubernetes.GetConfigmap(ctx, cs, kubeSystemNamespace, ciliumConfigmapName)
	require.NoError(t, err)
	require.Equal(t, "true", ciliumCM.Data[enableLRPFlag], "enable-local-redirect-policy not set to true in cilium-config")

	// 1.17 and 1.13 cilium versions of both files are identical
	// read file
	nodeLocalDNSContent, err := os.ReadFile(nodeLocalDNSDaemonsetPath)
	require.NoError(t, err)
	// replace pillar dns
	replaced := strings.ReplaceAll(string(nodeLocalDNSContent), "__PILLAR__DNS__SERVER__", kubeDNS)
	// Write the updated content back to the file
	err = os.WriteFile(tempNodeLocalDNSDaemonsetPath, []byte(replaced), 0o644)
	require.NoError(t, err)
	defer func() {
		err := os.Remove(tempNodeLocalDNSDaemonsetPath)
		require.NoError(t, err)
	}()

	// list out and select node of choice
	nodeList, err := kubernetes.GetNodeList(ctx, cs)
	require.NotEmpty(t, nodeList.Items)
	selectedNode := TakeOne(nodeList.Items).Name

	// deploy node local dns preqreqs and pods
	_, cleanupConfigMap := kubernetes.MustSetupConfigMap(ctx, cs, nodeLocalDNSConfigMapPath)
	cleanUpFns = append(cleanUpFns, cleanupConfigMap)
	_, cleanupServiceAccount := kubernetes.MustSetupServiceAccount(ctx, cs, nodeLocalDNSServiceAccountPath)
	cleanUpFns = append(cleanUpFns, cleanupServiceAccount)
	_, cleanupService := kubernetes.MustSetupService(ctx, cs, nodeLocalDNSServicePath)
	cleanUpFns = append(cleanUpFns, cleanupService)
	nodeLocalDNSDS, cleanupNodeLocalDNS := kubernetes.MustSetupDaemonset(ctx, cs, tempNodeLocalDNSDaemonsetPath)
	cleanUpFns = append(cleanUpFns, cleanupNodeLocalDNS)
	kubernetes.WaitForPodDaemonset(ctx, cs, nodeLocalDNSDS.Namespace, nodeLocalDNSDS.Name, nodeLocalDNSLabelSelector)
	require.NoError(t, err)
	// select a local dns pod after they start running
	pods, err := kubernetes.GetPodsByNode(ctx, cs, nodeLocalDNSDS.Namespace, nodeLocalDNSLabelSelector, selectedNode)
	require.NoError(t, err)
	selectedLocalDNSPod := TakeOne(pods.Items).Name

	// deploy lrp
	_, cleanupLRP := kubernetes.MustSetupLRP(ctx, ciliumCS, lrpPath)
	cleanUpFns = append(cleanUpFns, cleanupLRP)

	// create client pods
	clientDS, cleanupClient := kubernetes.MustSetupDaemonset(ctx, cs, clientPath)
	cleanUpFns = append(cleanUpFns, cleanupClient)
	kubernetes.WaitForPodDaemonset(ctx, cs, clientDS.Namespace, clientDS.Name, clientLabelSelector)
	require.NoError(t, err)
	// select a client pod after they start running
	clientPods, err := kubernetes.GetPodsByNode(ctx, cs, clientDS.Namespace, clientLabelSelector, selectedNode)
	require.NoError(t, err)
	selectedClientPod := TakeOne(clientPods.Items)

	t.Logf("Selected node: %s, node local dns pod: %s, client pod: %s\n", selectedNode, selectedLocalDNSPod, selectedClientPod.Name)

	// port forward to local dns pod on same node (separate thread)
	pf, err := k8s.NewPortForwarder(config, k8s.PortForwardingOpts{
		Namespace: nodeLocalDNSDS.Namespace,
		PodName:   selectedLocalDNSPod,
		LocalPort: initialPrometheusPort,
		DestPort:  initialPrometheusPort,
	})
	require.NoError(t, err)
	pctx := context.Background()
	portForwardCtx, cancel := context.WithTimeout(pctx, (retryAttempts+1)*retryDelay)
	cleanUpFns = append(cleanUpFns, cancel)

	err = defaultRetrier.Do(portForwardCtx, func() error {
		t.Logf("attempting port forward to a pod with label %s, in namespace %s...", nodeLocalDNSLabelSelector, nodeLocalDNSDS.Namespace)
		return errors.Wrap(pf.Forward(portForwardCtx), "could not start port forward")
	})
	require.NoError(t, err, "could not start port forward within %d", (retryAttempts+1)*retryDelay)
	cleanUpFns = append(cleanUpFns, pf.Stop)

	t.Log("started port forward")

	success = true
	return &selectedClientPod, cleanupFn
}

func testLRPCase(t *testing.T, ctx context.Context, clientPod corev1.Pod, clientCmd []string, expectResponse, expectErrMsg string,
	shouldError, countShouldIncrease bool, prometheusAddress string) {

	config := kubernetes.MustGetRestConfig()
	cs := kubernetes.MustGetClientset()

	// labels for target lrp metric
	metricLabels := map[string]string{
		"family": "1",
		"proto":  "udp",
		"server": "dns://0.0.0.0:53",
		"zone":   ".",
	}

	// curl to the specified prometheus address
	beforeMetric, err := prometheus.GetMetric(prometheusAddress, coreDNSRequestCountTotal, metricLabels)
	require.NoError(t, err)
	beforeValue := beforeMetric.GetCounter().GetValue()
	t.Logf("Before DNS request - metric count: %.0f", beforeValue)

	t.Log("calling command from client")

	val, errMsg, err := kubernetes.ExecCmdOnPod(ctx, cs, clientPod.Namespace, clientPod.Name, clientContainer, clientCmd, config, false)
	if shouldError {
		require.Error(t, err, "stdout: %s, stderr: %s", string(val), string(errMsg))
	} else {
		require.NoError(t, err, "stdout: %s, stderr: %s", string(val), string(errMsg))
	}

	require.Contains(t, string(val), expectResponse)
	require.Contains(t, string(errMsg), expectErrMsg)

	// in case there is time to propagate
	time.Sleep(500 * time.Millisecond)

	// curl again and see count diff
	afterMetric, err := prometheus.GetMetric(prometheusAddress, coreDNSRequestCountTotal, metricLabels)
	require.NoError(t, err)
	afterValue := afterMetric.GetCounter().GetValue()
	t.Logf("After DNS request - metric count: %.0f (diff: %.0f)", afterValue, afterValue-beforeValue)

	if countShouldIncrease {
		require.Greater(t, afterValue, beforeValue, "dns metric count did not increase after command - before: %.0f, after: %.0f", beforeValue, afterValue)
	} else {
		require.Equal(t, afterValue, beforeValue, "dns metric count increased after command - before: %.0f, after: %.0f", beforeValue, afterValue)
	}
}

// TestLRP tests if the local redirect policy in a cilium cluster is functioning
// The test assumes the current kubeconfig points to a cluster with cilium (1.16+), cns,
// and kube-dns already installed. The lrp feature flag should be enabled in the cilium config
// Does not check if cluster is in a stable state
// Resources created are automatically cleaned up
// From the lrp folder, run: go test ./ -v -tags "lrp" -run ^TestLRP$
func TestLRP(t *testing.T) {
	ctx := context.Background()

	selectedPod, cleanupFn := setupLRP(t, ctx)
	defer cleanupFn()
	require.NotNil(t, selectedPod)

	// Get the kube-dns service IP for DNS requests
	cs := kubernetes.MustGetClientset()
	svc, err := kubernetes.GetService(ctx, cs, kubeSystemNamespace, dnsService)
	require.NoError(t, err)
	kubeDNS := svc.Spec.ClusterIP

	t.Logf("LRP Test Starting...")

	// Basic LRP test - using initial port from setupLRP
	testLRPCase(t, ctx, *selectedPod, []string{
		"nslookup", "google.com", kubeDNS,
	}, "", "", false, true, getPrometheusAddress(initialPrometheusPort))

	t.Logf("LRP Test Completed")

	t.Logf("LRP Lifecycle Test Starting")

	// Run LRP Lifecycle test
	testLRPLifecycle(t, ctx, *selectedPod, kubeDNS)

	t.Logf("LRP Lifecycle Test Completed")
}

// testLRPLifecycle performs testing of Local Redirect Policy functionality
// including pod restarts, resource recreation, and cilium command validation
func testLRPLifecycle(t *testing.T, ctx context.Context, clientPod corev1.Pod, kubeDNS string) {
	config := kubernetes.MustGetRestConfig()
	cs := kubernetes.MustGetClientset()


	// Step 1: Validate LRP using cilium commands
	t.Log("Step 1: Validating LRP using cilium commands")
	validateCiliumLRP(t, ctx, cs, config)

	// Step 2: Restart busybox pods and verify LRP still works
	t.Log("Step 2: Restarting client pods to test persistence")
	restartedPod := restartClientPodsAndGetPod(t, ctx, cs, clientPod)

	// Step 3: Verify metrics after restart
	t.Log("Step 3: Verifying LRP functionality after pod restart")
	testLRPCase(t, ctx, restartedPod, []string{
		"nslookup", "google.com", kubeDNS,
	}, "", "", false, true, getPrometheusAddress(initialPrometheusPort))

	// Step 4: Validate cilium commands still show LRP
	t.Log("Step 4: Re-validating cilium LRP after restart")
	validateCiliumLRP(t, ctx, cs, config)

	// Step 5: Delete and recreate resources & restart nodelocaldns daemonset
	t.Log("Step 5: Testing resource deletion and recreation")
	recreatedPod := deleteAndRecreateResources(t, ctx, cs, clientPod)

	// Step 6: Re-establish port forward to new node-local-dns pod and validate metrics
	t.Log("Step 6: Re-establishing port forward to new node-local-dns pod for metrics validation")

	// Get the new node-local-dns pod on the same node as our recreated client pod
	nodeName := recreatedPod.Spec.NodeName
	newNodeLocalDNSPods, err := kubernetes.GetPodsByNode(ctx, cs, kubeSystemNamespace, nodeLocalDNSLabelSelector, nodeName)
	require.NoError(t, err)
	require.NotEmpty(t, newNodeLocalDNSPods.Items, "No node-local-dns pod found on node %s after restart", nodeName)

	newNodeLocalDNSPod := TakeOne(newNodeLocalDNSPods.Items)
	t.Logf("Setting up port forward to new node-local-dns pod: %s", newNodeLocalDNSPod.Name)

	// Setup new port forward to the new node-local-dns pod
	newPf, err := k8s.NewPortForwarder(config, k8s.PortForwardingOpts{
		Namespace: newNodeLocalDNSPod.Namespace,
		PodName:   newNodeLocalDNSPod.Name,
		LocalPort: recreatedPrometheusPort, // Use different port to avoid conflicts
		DestPort:  initialPrometheusPort,
	})
	require.NoError(t, err)

	newPortForwardCtx, newCancel := context.WithTimeout(ctx, (retryAttempts+1)*retryDelay)
	defer newCancel()

	err = defaultRetrier.Do(newPortForwardCtx, func() error {
		t.Logf("attempting port forward to new node-local-dns pod %s...", newNodeLocalDNSPod.Name)
		return errors.Wrap(newPf.Forward(newPortForwardCtx), "could not start port forward to new pod")
	})
	require.NoError(t, err, "could not start port forward to new node-local-dns pod")
	defer newPf.Stop()

	t.Log("Port forward to new node-local-dns pod established")

	// Use testLRPCase function with the new prometheus address
	t.Log("Validating metrics with new node-local-dns pod")
	testLRPCase(t, ctx, recreatedPod, []string{
		"nslookup", "github.com", kubeDNS,
	}, "", "", false, true, getPrometheusAddress(recreatedPrometheusPort))

	t.Logf("SUCCESS: Metrics validation passed - traffic is being redirected to new node-local-dns pod %s", newNodeLocalDNSPod.Name)

	// Step 7: Final cilium validation after node-local-dns restart
	t.Log("Step 7: Final cilium validation - ensuring LRP is still active after node-local-dns restart")
	validateCiliumLRP(t, ctx, cs, config)

}

// validateCiliumLRP checks that LRP is properly configured in cilium
func validateCiliumLRP(t *testing.T, ctx context.Context, cs *k8sclient.Clientset, config *rest.Config) {
	ciliumPods, err := cs.CoreV1().Pods(kubeSystemNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "k8s-app=cilium",
	})
	require.NoError(t, err)
	require.NotEmpty(t, ciliumPods.Items)
	ciliumPod := TakeOne(ciliumPods.Items)

	// Get Kubernetes version to determine validation approach
	serverVersion, err := cs.Discovery().ServerVersion()
	require.NoError(t, err)
	t.Logf("Detected Kubernetes version: %s", serverVersion.String())

	// Get kube-dns service IP for validation
	svc, err := kubernetes.GetService(ctx, cs, kubeSystemNamespace, dnsService)
	require.NoError(t, err)
	kubeDNSIP := svc.Spec.ClusterIP

	// IMPORTANT: Get node-local-dns pod IP on the SAME node as the cilium pod we're using
	selectedNode := ciliumPod.Spec.NodeName
	t.Logf("Using cilium pod %s on node %s for validation", ciliumPod.Name, selectedNode)

	// Get node-local-dns pod specifically on the same node as our cilium pod
	nodeLocalDNSPods, err := kubernetes.GetPodsByNode(ctx, cs, kubeSystemNamespace, nodeLocalDNSLabelSelector, selectedNode)
	require.NoError(t, err)
	require.NotEmpty(t, nodeLocalDNSPods.Items, "No node-local-dns pod found on node %s", selectedNode)

	// Use the first (and should be only) node-local-dns pod on this node
	nodeLocalDNSPod := nodeLocalDNSPods.Items[0]
	nodeLocalDNSIP := nodeLocalDNSPod.Status.PodIP
	require.NotEmpty(t, nodeLocalDNSIP, "node-local-dns pod %s has no IP address", nodeLocalDNSPod.Name)

	t.Logf("Validating LRP: kubeDNS IP=%s, nodeLocalDNS IP=%s (pod: %s), node=%s",
		kubeDNSIP, nodeLocalDNSIP, nodeLocalDNSPod.Name, selectedNode)

	// Check cilium lrp list
	lrpListCmd := []string{"cilium", "lrp", "list"}
	lrpOutput, _, err := kubernetes.ExecCmdOnPod(ctx, cs, ciliumPod.Namespace, ciliumPod.Name, "cilium-agent", lrpListCmd, config, false)
	require.NoError(t, err)

	// Validate the LRP output structure more thoroughly
	lrpOutputStr := string(lrpOutput)
	require.Contains(t, lrpOutputStr, "nodelocaldns", "LRP not found in cilium lrp list")

	// Parse LRP list output to validate structure
	lrpLines := strings.Split(lrpOutputStr, "\n")
	nodelocaldnsFound := false

	for _, line := range lrpLines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "nodelocaldns") && strings.Contains(line, "kube-system") {
			// Validate that the line contains expected components
			require.Contains(t, line, "kube-dns", "LRP line should reference kube-dns service")
			nodelocaldnsFound = true
			t.Logf("Found nodelocaldns LRP entry: %s", line)
			break
		}
	}

	require.True(t, nodelocaldnsFound, "nodelocaldns LRP entry not found with expected structure in output: %s", lrpOutputStr)

	// Check cilium service list for localredirect
	serviceListCmd := []string{"cilium", "service", "list"}
	serviceOutput, _, err := kubernetes.ExecCmdOnPod(ctx, cs, ciliumPod.Namespace, ciliumPod.Name, "cilium-agent", serviceListCmd, config, false)
	require.NoError(t, err)
	require.Contains(t, string(serviceOutput), "LocalRedirect", "LocalRedirect not found in cilium service list")

	// Validate LocalRedirect entries
	serviceLines := strings.Split(string(serviceOutput), "\n")
	tcpFound := false
	udpFound := false
	legacyFound := false

	for _, line := range serviceLines {
		if strings.Contains(line, "LocalRedirect") && strings.Contains(line, kubeDNSIP) {
			// Check if this line contains the expected frontend (kube-dns) and backend (node-local-dns) IPs
			if strings.Contains(line, nodeLocalDNSIP) {
				// Check for both modern format (with /TCP or /UDP) and legacy format (without protocol)
				if strings.Contains(line, "/TCP") {
					tcpFound = true
					t.Logf("Found TCP LocalRedirect: %s", strings.TrimSpace(line))
				} else if strings.Contains(line, "/UDP") {
					udpFound = true
					t.Logf("Found UDP LocalRedirect: %s", strings.TrimSpace(line))
				} else {
					legacyFound = true
					t.Logf("Found legacy LocalRedirect: %s", strings.TrimSpace(line))
				}
			}
		}
	}

	// Validate that we found either legacy format or modern format entries
	t.Log("Validating LocalRedirect entries - accepting either legacy format or modern TCP/UDP format")
	require.True(t, legacyFound || (tcpFound && udpFound), "Either legacy LocalRedirect entry OR both TCP and UDP entries must be found with frontend IP %s and backend IP %s on node %s", kubeDNSIP, nodeLocalDNSIP, selectedNode)

	t.Logf("Cilium LRP List Output:\n%s", string(lrpOutput))
	t.Logf("Cilium Service List Output:\n%s", string(serviceOutput))
}

// restartClientPodsAndGetPod restarts the client daemonset and returns a new pod reference
func restartClientPodsAndGetPod(t *testing.T, ctx context.Context, cs *k8sclient.Clientset, originalPod corev1.Pod) corev1.Pod {
	// Get the node name for consistent testing
	nodeName := originalPod.Spec.NodeName

	// Restart the daemonset (assumes it's named "lrp-test" based on the manifest)
	err := kubernetes.MustRestartDaemonset(ctx, cs, originalPod.Namespace, "lrp-test")
	require.NoError(t, err)

	// Wait for the daemonset to be ready
	kubernetes.WaitForPodDaemonset(ctx, cs, originalPod.Namespace, "lrp-test", clientLabelSelector)

	// Get the new pod on the same node
	clientPods, err := kubernetes.GetPodsByNode(ctx, cs, originalPod.Namespace, clientLabelSelector, nodeName)
	require.NoError(t, err)
	require.NotEmpty(t, clientPods.Items)

	return TakeOne(clientPods.Items)
}

// deleteAndRecreateResources deletes and recreates client pods and LRP, returning new pod
func deleteAndRecreateResources(t *testing.T, ctx context.Context, cs *k8sclient.Clientset, originalPod corev1.Pod) corev1.Pod {
	config := kubernetes.MustGetRestConfig()
	ciliumCS, err := ciliumClientset.NewForConfig(config)
	require.NoError(t, err)

	nodeName := originalPod.Spec.NodeName

	// Delete client daemonset
	dsClient := cs.AppsV1().DaemonSets(originalPod.Namespace)
	clientDS := kubernetes.MustParseDaemonSet(clientPath)
	kubernetes.MustDeleteDaemonset(ctx, dsClient, clientDS)

	// Delete LRP
	lrpContent, err := os.ReadFile(lrpPath)
	require.NoError(t, err)
	var lrp ciliumv2.CiliumLocalRedirectPolicy
	err = yaml.Unmarshal(lrpContent, &lrp)
	require.NoError(t, err)

	lrpClient := ciliumCS.CiliumV2().CiliumLocalRedirectPolicies(lrp.Namespace)
	kubernetes.MustDeleteCiliumLocalRedirectPolicy(ctx, lrpClient, lrp)

	// Wait for client pods to be deleted
	t.Log("Waiting for client pods to be deleted...")
	err = kubernetes.WaitForPodsDelete(ctx, cs, originalPod.Namespace, clientLabelSelector)
	require.NoError(t, err)

	// Wait for LRP to be deleted by polling
	t.Log("Waiting for LRP to be deleted...")
	err = kubernetes.WaitForLRPDelete(ctx, ciliumCS, lrp)
	require.NoError(t, err)

	// Recreate LRP
	_, cleanupLRP := kubernetes.MustSetupLRP(ctx, ciliumCS, lrpPath)
	t.Cleanup(cleanupLRP)

	// Restart node-local-dns pods to pick up new LRP configuration
	t.Log("Restarting node-local-dns pods after LRP recreation")
	err = kubernetes.MustRestartDaemonset(ctx, cs, kubeSystemNamespace, "node-local-dns")
	require.NoError(t, err)
	kubernetes.WaitForPodDaemonset(ctx, cs, kubeSystemNamespace, "node-local-dns", nodeLocalDNSLabelSelector)

	// Recreate client daemonset
	_, cleanupClient := kubernetes.MustSetupDaemonset(ctx, cs, clientPath)
	t.Cleanup(cleanupClient)

	// Wait for pods to be ready
	kubernetes.WaitForPodDaemonset(ctx, cs, clientDS.Namespace, clientDS.Name, clientLabelSelector)

	// Get new pod on the same node
	clientPods, err := kubernetes.GetPodsByNode(ctx, cs, clientDS.Namespace, clientLabelSelector, nodeName)
	require.NoError(t, err)
	require.NotEmpty(t, clientPods.Items)

	return TakeOne(clientPods.Items)
}

// TakeOne takes one item from the slice randomly; if empty, it returns the empty value for the type
// Use in testing only
func TakeOne[T any](slice []T) T {
	if len(slice) == 0 {
		var zero T
		return zero
	}
	rand.Seed(uint64(time.Now().UnixNano()))
	return slice[rand.Intn(len(slice))]
}
