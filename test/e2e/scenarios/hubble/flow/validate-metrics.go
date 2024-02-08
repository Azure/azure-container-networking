package flow

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/test/internal/retry"
)

const (
	defaultRetryAttempts = 20
	defaultTimeout       = 300 * time.Second
	defaultRetryDelay    = 5 * time.Second
)

var requiredMetrics = []string{
	"hubble_flows_processed_total",
	"hubble_tcp_flags_total",
}

type ValidateHubbleFlowMetric struct {
	LocalPort string
}

func (v *ValidateHubbleFlowMetric) Run() error {
	defaultRetrier := retry.Retrier{Attempts: defaultRetryAttempts, Delay: defaultRetryDelay}
	promAddress := fmt.Sprintf("http://localhost:%s/metrics", v.LocalPort)
	log.Printf("require all metrics to be present: %+v\n", requiredMetrics)
	ctx := context.Background()
	var metrics map[string]struct{}
	scrapeMetricsFn := func() error {
		log.Printf("attempting scrape metrics on %s", promAddress)

		var err error
		metrics, err = getPrometheusMetrics(promAddress)
		if err != nil {
			return fmt.Errorf("failed to get prometheus metrics: %w", err)
		}
		return nil
	}

	portForwardCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	if err := defaultRetrier.Do(portForwardCtx, scrapeMetricsFn); err != nil {
		return fmt.Errorf("could not start port forward within %ds: %w	", defaultTimeout, err)
	}

	for _, reqMetric := range requiredMetrics {
		if _, exists := metrics[reqMetric]; !exists {
			return fmt.Errorf("scraping %s, did not find metric %s: ", promAddress, reqMetric) //nolint:goerr113,gocritic
		}
		log.Printf("found metric %s\n", reqMetric)
	}

	log.Printf("all metrics validated: %+v\n", requiredMetrics)
	return nil
}

func (v *ValidateHubbleFlowMetric) Prevalidate() error {
	return nil
}

func (v *ValidateHubbleFlowMetric) Stop() error {
	return nil
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
