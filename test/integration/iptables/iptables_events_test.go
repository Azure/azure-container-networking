//go:build ebpf

package iptables

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/test/internal/kubernetes"
	ciliumClientset "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "k8s.io/client-go/kubernetes"
)

const (
	// azure-iptables-monitor runs as a sidecar (and init) container in the cilium
	// daemonset on eBPF host-routing clusters. It compares every node's iptables rules
	// against the allowlist regexes in the allowed-iptables-patterns ConfigMap and, on a
	// mismatch, emits a Warning node Event and labels the CiliumNode.
	// See azure-iptables-monitor/iptables_monitor.go.
	iptablesMonitorContainer     = "azure-iptables-monitor"
	iptablesMonitorInitContainer = "azure-iptables-monitor-init"
	ciliumLabelSelector          = "k8s-app=cilium"
	kubeSystemNamespace          = "kube-system"

	// userIPTablesRulesLabel is patched onto each CiliumNode by the monitor:
	// "true" when unexpected (user) iptables rules are found, "false" otherwise.
	userIPTablesRulesLabel = "kubernetes.azure.com/user-iptables-rules"

	// Event reasons/namespace emitted by azure-iptables-monitor.
	reasonUnexpectedIPTables = "UnexpectedIPTablesRules"
	reasonBlockedIPTables    = "BlockedIPTablesRule"
	iptablesEventNamespace   = "default"
)

// TestNoUnexpectedIPTablesEvents is an end-of-run validation for eBPF host-routing
// clusters. Throughout the e2e run, azure-iptables-monitor checks each node's iptables
// rules against the allowed-iptables-patterns allowlist; if cilium (or anything else)
// programs a rule that is not covered by the allowlist, the monitor emits a Warning node
// Event (UnexpectedIPTablesRules / BlockedIPTablesRule) and labels the CiliumNode
// user-iptables-rules=true. This test asserts neither signal is present on the cluster.
//
// It runs only on eBPF host-routing clusters (skips if the monitor isn't deployed) and is
// intended to run last, after all other eBPF e2e tests.
// Run: go test . -v -tags "ebpf" -count=1 -run ^TestNoUnexpectedIPTablesEvents$
func TestNoUnexpectedIPTablesEvents(t *testing.T) {
	ctx := context.Background()

	cs := kubernetes.MustGetClientset()
	config := kubernetes.MustGetRestConfig()
	ciliumCS, err := ciliumClientset.NewForConfig(config)
	require.NoError(t, err)

	// azure-iptables-monitor is only present on eBPF host-routing clusters.
	if !iptablesMonitorDeployed(ctx, t, cs) {
		t.Skipf("%s not deployed (eBPF host-routing only); skipping iptables-monitor k8s event validation", iptablesMonitorContainer)
	}

	// Warning events emitted by the monitor over the course of the run (one per node per
	// flagged check cycle). On a healthy eBPF host-routing cluster there should be none.
	bad := unexpectedIPTablesEvents(ctx, t, cs)
	require.Empty(t, bad, "%s reported unexpected/blocked iptables rules during the eBPF run:\n%s", iptablesMonitorContainer, formatEvents(bad))

	// Current label state across all CiliumNodes as a complementary point-in-time signal.
	flagged := flaggedCiliumNodes(ctx, t, ciliumCS)
	require.Empty(t, flagged, "CiliumNode(s) currently labeled %s=true: %s", userIPTablesRulesLabel, strings.Join(flagged, ", "))
}

// iptablesMonitorDeployed reports whether the azure-iptables-monitor container is part of
// the cilium daemonset pods (i.e. this is an eBPF host-routing cluster).
func iptablesMonitorDeployed(ctx context.Context, t *testing.T, cs *k8sclient.Clientset) bool {
	pods, err := cs.CoreV1().Pods(kubeSystemNamespace).List(ctx, metav1.ListOptions{LabelSelector: ciliumLabelSelector})
	require.NoError(t, err)
	for i := range pods.Items {
		for _, c := range pods.Items[i].Spec.Containers {
			if c.Name == iptablesMonitorContainer {
				return true
			}
		}
		for _, c := range pods.Items[i].Spec.InitContainers {
			if c.Name == iptablesMonitorInitContainer {
				return true
			}
		}
	}
	return false
}

// unexpectedIPTablesEvents returns all azure-iptables-monitor Warning events on the cluster
// (UnexpectedIPTablesRules / BlockedIPTablesRule), across every node.
func unexpectedIPTablesEvents(ctx context.Context, t *testing.T, cs *k8sclient.Clientset) []corev1.Event {
	events, err := cs.CoreV1().Events(iptablesEventNamespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)

	var bad []corev1.Event
	for i := range events.Items {
		e := events.Items[i]
		if e.Source.Component != iptablesMonitorContainer {
			continue
		}
		if e.Type != corev1.EventTypeWarning {
			continue
		}
		if e.Reason != reasonUnexpectedIPTables && e.Reason != reasonBlockedIPTables {
			continue
		}
		bad = append(bad, e)
	}
	return bad
}

// flaggedCiliumNodes returns the names of CiliumNodes currently labeled
// user-iptables-rules=true by the monitor.
func flaggedCiliumNodes(ctx context.Context, t *testing.T, ciliumCS ciliumClientset.Interface) []string {
	nodes, err := ciliumCS.CiliumV2().CiliumNodes().List(ctx, metav1.ListOptions{})
	require.NoError(t, err)

	var flagged []string
	for i := range nodes.Items {
		if nodes.Items[i].Labels[userIPTablesRulesLabel] == "true" {
			flagged = append(flagged, nodes.Items[i].Name)
		}
	}
	return flagged
}

func formatEvents(events []corev1.Event) string {
	var b strings.Builder
	for _, e := range events {
		fmt.Fprintf(&b, "  - node=%s [%s] reason=%s at=%s: %s\n",
			e.InvolvedObject.Name, e.Type, e.Reason, eventTime(e), e.Message)
	}
	return b.String()
}

func eventTime(e corev1.Event) string {
	if !e.LastTimestamp.IsZero() {
		return e.LastTimestamp.String()
	}
	if !e.EventTime.IsZero() {
		return e.EventTime.String()
	}
	return e.FirstTimestamp.String()
}
