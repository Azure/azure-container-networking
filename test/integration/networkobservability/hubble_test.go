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
)

const (
	retryAttempts = 10
	retryDelay    = 5 * time.Second
	promAddress   = "http://localhost:9965/metrics"
	labelSelector = "k8s-app=cilium"
	namespace     = "kube-system"
)

var (
	defaultRetrier  = retry.Retrier{Attempts: retryAttempts, Delay: retryDelay}
	requiredMetrics = []string{
		"hubble_flows_processed_total",
		"hubble_tcp_flags_total",
	}
)

func TestEndpoints(t *testing.T) {
	config := kubernetes.MustGetRestConfig()
	ctx := context.Background()
	clusterCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	pingCheckFn := func() error {
		var pf *k8s.PortForwarder
		pf, err := k8s.NewPortForwarder(config, t, k8s.PortForwardingOpts{
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

		// verify that the response contains the required metrics
		for _, reqMetric := range requiredMetrics {
			if val, exists := metrics[reqMetric]; !exists {
				return fmt.Errorf("scraping %s, did not find metric %s", val, promAddress) //nolint:goerr113,gocritic
			}
		}
		t.Logf("all metrics validated: %+v", requiredMetrics)
		return nil
	}

	if err := defaultRetrier.Do(clusterCtx, pingCheckFn); err != nil {
		t.Fatalf("metrics check failed with error: %v", err)
	}
}

func getPrometheusMetrics(url string) (map[string]struct{}, error) {
	client := http.Client{}
	resp, err := client.Get(url) //nolint
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP request failed with status: %v", resp.Status) //nolint:goerr113,gocritic
	}

	metricsData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading HTTP response body failed: %w", err)
	}

	metrics := parseMetrics(string(metricsData))
	return metrics, nil
}

func parseMetrics(metricsData string) map[string]struct{} {
	// Create a map to store the strings before the first '{'.
	metrics := make(map[string]struct{})

	// sample metrics
	// hubble_tcp_flags_total{destination="",family="IPv4",flag="RST",source="kube-system/metrics-server"} 980
	// hubble_tcp_flags_total{destination="",family="IPv4",flag="SYN",source="kube-system/ama-metrics"} 1777
	// we only want the metric name for the time being
	// label order/parseing can happen later
	lines := strings.Split(metricsData, "\n")
	// Iterate through each line.
	for _, line := range lines {
		// Find the index of the first '{' character.
		index := strings.Index(line, "{")
		if index >= 0 {
			// Extract the string before the first '{'.
			str := strings.TrimSpace(line[:index])
			// Store the string in the map.
			metrics[str] = struct{}{}
		}
	}

	return metrics
}
