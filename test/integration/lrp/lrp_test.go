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
)

const (
	ciliumManifestsDir  = "../manifests/cilium/lrp/"
	kubeSystemNamespace = "kube-system"
	dnsService          = "kube-dns"
	// envCiliumDir              = "DIR"
	retryAttempts             = 5
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

// TestLRP tests if the local redirect policy in a cilium cluster is functioning
// The test assumes the current kubeconfig points to a cluster with cilium and kube-dns already installed
// and with the lrp feature flag enabled in the cilium config
// run go test from the lrp directory
func TestLRP(t *testing.T) {
	config := kubernetes.MustGetRestConfig()
	ctx := context.Background()
	// clusterCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)

	cs := kubernetes.MustGetClientset()

	ciliumCS, err := ciliumClientset.NewForConfig(config)
	require.NoError(t, err)

	svc, err := kubernetes.GetService(ctx, cs, kubeSystemNamespace, dnsService)
	require.NoError(t, err)
	kubeDNS := svc.Spec.ClusterIP

	// 1.17 and 1.13 cilium versions of both files are identical
	// directory := os.Getenv(string(envCiliumDir))
	// read file
	nodeLocalDNSContent, err := os.ReadFile(nodeLocalDNSDaemonsetPath)
	require.NoError(t, err)
	// replace pillar dns
	replaced := strings.ReplaceAll(string(nodeLocalDNSContent), "__PILLAR__DNS__SERVER__", kubeDNS)
	// Write the updated content back to the file
	err = os.WriteFile(tempNodeLocalDNSDaemonsetPath, []byte(replaced), 0644)
	require.NoError(t, err)
	defer func() {
		err := os.Remove(tempNodeLocalDNSDaemonsetPath)
		require.NoError(t, err)
	}()

	// deploy configmap
	_, cleanupConfigMap := kubernetes.MustSetupConfigMap(ctx, cs, nodeLocalDNSConfigMapPath)
	defer cleanupConfigMap()

	// deploy node local dns pods
	nodeLocalDNSDS, cleanupNodeLocalDNS := kubernetes.MustSetupDaemonset(ctx, cs, tempNodeLocalDNSDaemonsetPath)
	defer cleanupNodeLocalDNS()
	_, cleanupServiceAccount := kubernetes.MustSetupServiceAccount(ctx, cs, nodeLocalDNSServiceAccountPath)
	defer cleanupServiceAccount()
	_, cleanupService := kubernetes.MustSetupService(ctx, cs, nodeLocalDNSServicePath)
	defer cleanupService()

	// deploy lrp
	_, cleanupLRP := kubernetes.MustSetupLRP(ctx, ciliumCS, lrpPath)
	defer cleanupLRP()

	// get busybox node, and then get local dns pod on same node
	clientDS, cleanupClient := kubernetes.MustSetupDaemonset(ctx, cs, clientPath)
	defer cleanupClient()

	err = kubernetes.WaitForPodsRunning(ctx, cs, "", "")
	require.NoError(t, err)

	// list out node local dns pods
	nodeList, err := kubernetes.GetNodeList(ctx, cs)
	require.NotEmpty(t, nodeList.Items)
	selectedNode := TakeOne(nodeList.Items).Name

	pods, err := kubernetes.GetPodsByNode(ctx, cs, nodeLocalDNSDS.Namespace, nodeLocalDNSLabelSelector, selectedNode)
	require.NoError(t, err)
	selectedLocalDNSPod := TakeOne(pods.Items).Name

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
	defer cancel()

	err = defaultRetrier.Do(portForwardCtx, func() error {
		t.Logf("attempting port forward to a pod with label %s, in namespace %s...", nodeLocalDNSLabelSelector, nodeLocalDNSDS.Namespace)
		return errors.Wrap(pf.Forward(portForwardCtx), "could not start port forward")
	})
	require.NoError(t, err, "could not start port forward within %d", (retryAttempts+1)*retryDelay)
	defer pf.Stop()

	t.Log("started port forward")

	time.Sleep(time.Second * 10)

	// labels
	metricLabels := map[string]string{
		"family": "1",
		"proto":  "udp",
		"server": "dns://0.0.0.0:53",
		"zone":   ".",
	}

	// curl localhost:9253/metrics | grep coredns_dns_request_count_total
	beforeCount, err := prometheus.GetMetric(promAddress, coreDNSRequestCountTotal, metricLabels)
	require.NoError(t, err)

	// select a pod
	clientPods, err := kubernetes.GetPodsByNode(ctx, cs, clientDS.Namespace, clientLabelSelector, selectedNode)
	require.NoError(t, err)
	selectedClientPod := TakeOne(clientPods.Items).Name

	time.Sleep(1 * time.Second)
	t.Log("calling nslookup from client")
	// nslookup to 10.0.0.10
	val, err := kubernetes.ExecCmdOnPod(ctx, cs, clientDS.Namespace, selectedClientPod, clientContainer, []string{
		"nslookup", "google.com", "10.0.0.10",
	}, config)
	require.NoError(t, err, string(val))
	// can connect
	require.Contains(t, string(val), "Server:")

	time.Sleep(1 * time.Second)

	// curl again and see count increases
	afterCount, err := prometheus.GetMetric(promAddress, coreDNSRequestCountTotal, metricLabels)
	require.NoError(t, err)

	// count should go up
	require.Greater(t, afterCount, beforeCount, "dns metric count did not increase after nslookup")
}
func TakeOne[T any](slice []T) T {
	if len(slice) == 0 {
		var zero T
		return zero
	}
	return slice[0]
}
