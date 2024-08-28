//go:build networkobservability

package networkobservability

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	k8s "github.com/Azure/azure-container-networking/test/integration"
	"github.com/Azure/azure-container-networking/test/internal/kubernetes"
	"github.com/Azure/azure-container-networking/test/internal/retry"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/require"
)

const (
	retryAttempts               = 10
	retryDelay                  = 5 * time.Second
	promAddress                 = "http://localhost:9965/metrics"
	labelSelector               = "k8s-app=cilium"
	namespace                   = "kube-system"
	hubbleflowsprocessedtotal   = "hubble_flows_processed_total"
	hubbletcpflagstotal         = "hubble_tcp_flags_total"
	hubblednsresponsestotal     = "hubble_dns_responses_total"
	hubblednsresponsetypestotal = "hubble_dns_response_types_total"
	hubblednsqueriestotal       = "hubble_dns_queries_total"
	hubbledroptotal             = "hubble_drop_total"
)

var (
	defaultRetrier = retry.Retrier{Attempts: retryAttempts, Delay: retryDelay}
	hubbleKeys     = map[string][]string{
		hubbleflowsprocessedtotal:   flow,
		hubbletcpflagstotal:         flow,
		hubblednsresponsestotal:     dns,
		hubblednsresponsetypestotal: dns,
		hubblednsqueriestotal:       dns,
		hubbledroptotal:             drop,
	}
	dns  = []string{"query"}
	flow = []string{"source", "destination"}
	drop = []string{"source", "source"}
)

func TestPromtheusStringInputParser(t *testing.T) {
	input := `
	hubble_tcp_flags_total{destination="",family="IPv4",flag="RST",source="kube-system/metrics-server"} 980
	`
	metrics, err := parseStringPrometheusMetrics(input)
	if err != nil {
		t.Fail()
	}

	if len(metrics) != 1 {
		t.Fail()
	}

	verify := metrics[hubbletcpflagstotal]

	if verify.GetName() != hubbletcpflagstotal {
		t.Fail()
	}

	if len(verify.GetMetric()) != 1 {
		t.Fail()
	}

	kv := verify.GetMetric()[0]

	testMetrichubbletcpflagstotal(t, kv, "RST", "kube-system/metrics-server", 980)
}

func TestPromtheusStringThreeInputParser(t *testing.T) {
	input := `
	hubble_tcp_flags_total{destination="",family="IPv4",flag="RST",source="kube-system/metrics-server"} 980
	hubble_tcp_flags_total{destination="",family="IPv4",flag="SYN",source="kube-system/ama-metrics"} 1777
	hubble_flows_processed_total{destination="kube-system/coredns-76b9877f49-2p4fc",protocol="UDP",source="",subtype="to-stack",type="Trace",verdict="FORWARDED"} 3
	`
	metrics, err := parseStringPrometheusMetrics(input)
	if err != nil {
		t.Fail()
	}

	if len(metrics) != 2 {
		t.Fail()
	}

	tcpflagtotalkey := metrics[hubbletcpflagstotal]

	if tcpflagtotalkey.GetName() != hubbletcpflagstotal {
		t.Fail()
	}

	if len(tcpflagtotalkey.GetMetric()) != 2 {
		t.Fail()
	}

	testMetrichubbletcpflagstotal(t, tcpflagtotalkey.GetMetric()[0], "RST", "kube-system/metrics-server", 980)
	testMetrichubbletcpflagstotal(t, tcpflagtotalkey.GetMetric()[1], "SYN", "kube-system/ama-metrics", 1777)

	hubbleflowproccessed := metrics[hubbleflowsprocessedtotal]

	if hubbleflowproccessed.GetName() != hubbleflowsprocessedtotal {
		t.Fail()
	}

	testMetrichubbleflowsprocessedtotal(t, hubbleflowproccessed.GetMetric()[0])
}

func TestLabelCheck(t *testing.T) {
	input := `
	hubble_drop_total{protocol="TCP",reason="POLICY_DENIED",source="kube-system/ama-metrics-node-99c95",destination="kube-system/coredns-76b9877f49-2p4fc"} 24
	hubble_dns_response_types_total{destination="kube-system/agnhost-a",qtypes="A",query="google.com.",source="kube-system/coredns-76b9877f49-2p4fc",type="A"} 1
	hubble_dns_queries_total{destination="kube-system/coredns-76b9877f49-2p4fc",ips_returned="0",qtypes="A",query="google.com.",rcode="",source="kube-system/agnhost-a"} 1
	hubble_dns_responses_total{destination="kube-system/agnhost-a",ips_returned="0",qtypes="A",query="google.com.cluster.local.",rcode="Non-Existent Domain",source="kube-system/coredns-7"} 1
	hubble_tcp_flags_total{destination="",family="IPv4",flag="RST",source="kube-system/metrics-server"} 980
	hubble_flows_processed_total{destination="kube-system/coredns-76b9877f49-2p4fc",protocol="UDP",source="",subtype="to-stack",type="Trace",verdict="FORWARDED"} 3
	`
	metrics, err := parseStringPrometheusMetrics(input)
	if err != nil {
		t.Fail()
	}

	if !verifyLabelsPresent(metrics) {
		t.Fail()
	}
}

func TestPromtheusInvalidStringInputParser(t *testing.T) {
	input := `
	This clearly should fail. If it ever passes blame Prometheus. 
	`
	_, err := parseStringPrometheusMetrics(input)
	require.Error(t, err)
}

func testMetrichubbleflowsprocessedtotal(t *testing.T, metric *io_prometheus_client.Metric) {
	kv := metric

	if len(kv.GetLabel()) != 6 {
		t.Fail()
	}

	l1 := kv.GetLabel()[0]

	if l1.GetName() != "source" && l1.GetValue() != "kube-system/coredns-76b9877f49-2p4fc" {
		t.Fail()
	}

	if kv.GetUntyped().GetValue() != 3 {
		t.Fail()
	}
}

func testMetrichubbletcpflagstotal(t *testing.T, metric *io_prometheus_client.Metric, flag, source string, value float64) {
	kv := metric

	if len(kv.GetLabel()) != 4 {
		t.Fail()
	}

	l1 := kv.GetLabel()[0]
	l2 := kv.GetLabel()[1]
	l3 := kv.GetLabel()[2]
	l4 := kv.GetLabel()[3]

	if l1.GetName() != "destination" && l1.GetValue() != "" {
		t.Fail()
	}

	if l2.GetName() != "family" && l2.GetValue() != "IPv4" {
		t.Fail()
	}

	if l3.GetName() != "flag" && l3.GetValue() != flag {
		t.Fail()
	}

	if l4.GetName() != "source" && l4.GetValue() != source {
		t.Fail()
	}

	if kv.GetUntyped().GetValue() != value {
		t.Fail()
	}
}

func verifyLabelsPresent(data map[string]*io_prometheus_client.MetricFamily) bool {
	for k, reqMetric := range hubbleKeys {
		if val, exists := data[k]; exists {
			// go through each metric from the response and verify that each metric has the appropriate labels
			labelNames := make(map[string]bool)
			for _, metrics := range val.GetMetric() {
				for _, labels := range metrics.GetLabel() {
					labelNames[labels.GetName()] = true
				}
			}

			for _, req := range reqMetric {
				if _, ok := labelNames[req]; !ok {
					return false
				}
			}
		}
	}

	return true
}

func TestEndpoints(t *testing.T) {
	config := kubernetes.MustGetRestConfig()
	ctx := context.Background()
	clusterCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	pingCheckFn := func() error {
		var pf *k8s.PortForwarder
		pf, err := k8s.NewPortForwarder(config, k8s.PortForwardingOpts{
			Namespace:     namespace,
			LabelSelector: labelSelector,
			LocalPort:     9965,
			DestPort:      9965,
		})
		if err != nil {
			t.Error(err)
		}
		pctx := context.Background()

		portForwardCtx, cancel := context.WithTimeout(pctx, (retryAttempts+1)*retryDelay)
		defer cancel()

		portForwardFn := func() error {
			t.Logf("attempting port forward to a pod with label %s, in namespace %s...", labelSelector, namespace)
			if err = pf.Forward(portForwardCtx); err != nil {
				return fmt.Errorf("could not start port forward: %w", err)
			}
			return nil
		}

		if err = defaultRetrier.Do(portForwardCtx, portForwardFn); err != nil {
			t.Fatalf("could not start port forward within %d: %v", (retryAttempts+1)*retryDelay, err)
		}
		defer pf.Stop()

		// scrape the hubble metrics
		metrics, err := getPrometheusMetrics(promAddress)
		if err != nil {
			return fmt.Errorf("scraping %s, failed with error: %w", promAddress, err)
		}

		if !verifyLabelsPresent(metrics) {
			t.Fail()
		}

		return nil
	}

	if err := defaultRetrier.Do(clusterCtx, pingCheckFn); err != nil {
		t.Fatalf("metrics check failed with error: %v", err)
	}
}

func getPrometheusMetrics(url string) (map[string]*io_prometheus_client.MetricFamily, error) {
	client := http.Client{}
	resp, err := client.Get(url) //nolint
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP request failed with status: %v", resp.Status) //nolint:goerr113,gocritic
	}

	metrics, err := parseReaderPrometheusMetrics(resp.Body)
	if err != nil {
		return nil, err
	}

	return metrics, nil
}

func parseReaderPrometheusMetrics(input io.Reader) (map[string]*io_prometheus_client.MetricFamily, error) {
	var parser expfmt.TextParser
	return parser.TextToMetricFamilies(input) //nolint
}

func parseStringPrometheusMetrics(input string) (map[string]*io_prometheus_client.MetricFamily, error) {
	var parser expfmt.TextParser
	reader := strings.NewReader(input)
	return parser.TextToMetricFamilies(reader) //nolint
}
