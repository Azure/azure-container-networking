package prometheus

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

var (
	errNoMetricFamilyFound = errors.New("no metric family found")
	errNoMetricFound       = errors.New("no metric found")
)

// GetMetrics issues a web request to the specified url and parses any metrics returned
func GetMetrics(url string) (map[string]*io_prometheus_client.MetricFamily, error) {
	client := http.Client{}
	resp, err := client.Get(url) //nolint
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP request failed with status: %v", resp.Status) //nolint:goerr113,gocritic
	}

	metrics, err := ParseReaderMetrics(resp.Body)
	if err != nil {
		return nil, err
	}

	return metrics, nil
}

func ParseReaderMetrics(input io.Reader) (map[string]*io_prometheus_client.MetricFamily, error) {
	var parser expfmt.TextParser
	return parser.TextToMetricFamilies(input) //nolint
}

func ParseStringMetrics(input string) (map[string]*io_prometheus_client.MetricFamily, error) {
	var parser expfmt.TextParser
	reader := strings.NewReader(input)
	return parser.TextToMetricFamilies(reader) //nolint
}

// SelectMetric retrieves a particular metric from a map of MetricFamily based on the name (key) and
// the provided label kv pairs. Every label kv pair on the metric must match for it to be returned
// For example, to match the following metric: my_metric{a="1",b="udp"} 7
// name must be "my_metric", and the map of matchLabels must be exactly {"a": "1", "b": "udp"}
func SelectMetric(metrics map[string]*io_prometheus_client.MetricFamily, name string, matchLabels map[string]string) (*io_prometheus_client.Metric, error) {
	metricFamily := metrics[name]
	if metricFamily == nil {
		return nil, errNoMetricFamilyFound
	}

	// gets all label combinations and their values and then checks each one
	metricList := metricFamily.GetMetric()
	for _, metric := range metricList {
		// number of kv pairs in this label must match expected
		if len(metric.GetLabel()) != len(matchLabels) {
			continue
		}

		// search this label to see if it matches all our expected labels
		allKVMatch := true
		for _, kvPair := range metric.GetLabel() {
			if matchLabels[kvPair.GetName()] != kvPair.GetValue() {
				allKVMatch = false
				break
			}
		}

		// metric with label that matches all kv pairs
		if allKVMatch {
			return metric, nil
		}
	}
	return nil, errNoMetricFound
}

// GetMetric is a convenience function to issue a web request to the specified url and then
// select a particular metric that exactly matches the name and labels. The metric is then returned
// and values can be retrieved based on what type of metric it is, for example .GetCounter().GetValue()
func GetMetric(url, name string, labels map[string]string) (*io_prometheus_client.Metric, error) {
	metrics, err := GetMetrics(url)
	if err != nil {
		return nil, err
	}
	return SelectMetric(metrics, name, labels)
}
