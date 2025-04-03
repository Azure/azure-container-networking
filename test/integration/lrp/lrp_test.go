//go:build lrp

package lrp

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	k8s "github.com/Azure/azure-container-networking/test/integration"
	"github.com/Azure/azure-container-networking/test/integration/prometheus"
	"github.com/Azure/azure-container-networking/test/internal/kubernetes"
	"github.com/Azure/azure-container-networking/test/internal/retry"
	ciliumClientset "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/rand"
	corev1 "k8s.io/api/core/v1"
)

const (
	ciliumConfigmapName       = "cilium-config"
	ciliumManifestsDir        = "../manifests/cilium/lrp/"
	enableLRPFlag             = "enable-local-redirect-policy"
	kubeSystemNamespace       = "kube-system"
	dnsService                = "kube-dns"
	retryAttempts             = 10
	retryDelay                = 5 * time.Second
	promAddress               = "http://localhost:9253/metrics"
	nodeLocalDNSLabelSelector = "k8s-app=node-local-dns"
	clientLabelSelector       = "lrp-test=true"
	coreDNSRequestCountTotal  = "coredns_dns_request_count_total"
	clientContainer           = "no-op"
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
		LocalPort: 9253,
		DestPort:  9253,
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
	shouldError, countShouldIncrease bool) {

	config := kubernetes.MustGetRestConfig()
	cs := kubernetes.MustGetClientset()

	// labels for target lrp metric
	metricLabels := map[string]string{
		"family": "1",
		"proto":  "udp",
		"server": "dns://0.0.0.0:53",
		"zone":   ".",
	}

	// curl localhost:9253/metrics
	beforeMetric, err := prometheus.GetMetric(promAddress, coreDNSRequestCountTotal, metricLabels)
	require.NoError(t, err)

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
	afterMetric, err := prometheus.GetMetric(promAddress, coreDNSRequestCountTotal, metricLabels)
	require.NoError(t, err)

	if countShouldIncrease {
		require.Greater(t, afterMetric.GetCounter().GetValue(), beforeMetric.GetCounter().GetValue(), "dns metric count did not increase after command")
	} else {
		require.Equal(t, afterMetric.GetCounter().GetValue(), beforeMetric.GetCounter().GetValue(), "dns metric count increased after command")
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

	testLRPCase(t, ctx, *selectedPod, []string{
		"nslookup", "google.com", "10.0.0.10",
	}, "", "", false, true)
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
