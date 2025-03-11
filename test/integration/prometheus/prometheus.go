package prometheus

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

var (
	errNoMetricFamilyFound = errors.New("no metric family found")
	errNoMetricFound       = errors.New("no metric found")
)

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

func SelectMetric(metrics map[string]*io_prometheus_client.MetricFamily, name string, expected map[string]string) (float64, error) {
	metricFamily := metrics[name]
	if metricFamily == nil {
		return 0, errNoMetricFamilyFound
	}

	// gets all label combinations and their values and then checks each one
	metricList := metricFamily.GetMetric()
	for _, metric := range metricList {
		// number of kv pairs in this label must match expected
		if len(metric.GetLabel()) != len(expected) {
			continue
		}

		// search this label to see if it matches all our expected labels
		allKVMatch := true
		for _, kvPair := range metric.GetLabel() {
			if expected[kvPair.GetName()] != kvPair.GetValue() {
				allKVMatch = false
				break
			}
		}

		// metric with label that matches all kv pairs
		if allKVMatch {
			log.Printf("Metric Found: %+v \n", *metric.GetCounter().Value)
			return *metric.GetCounter().Value, nil
		}
	}
	return 0, errNoMetricFound
}

// GetMetric is a convenience function to issue a web request to the specified url and then
// select a particular metric that exactly matches the name and labels
func GetMetric(url string, name string, labels map[string]string) (float64, error) {
	metrics, err := GetMetrics(url)
	if err != nil {
		return 0, err
	}
	return SelectMetric(metrics, name, labels)
}
