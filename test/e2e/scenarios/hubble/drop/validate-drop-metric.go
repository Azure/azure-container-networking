package drop

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"reflect"
	"time"

	"github.com/Azure/azure-container-networking/test/internal/retry"
	promclient "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

var ErrNoMetricFound = fmt.Errorf("no metric found")

const (
	destinationKey = "destination"
	sourceKey      = "source"
	protcolKey     = "protocol"
	reason         = "reason"

	sleepDelay           = 5 * time.Second
	defaultTimeout       = 300 * time.Second
	defaultRetryDelay    = 5 * time.Second
	defaultRetryAttempts = 60
)

type ValidateHubbleDropMetric struct {
	PortForwardedHubblePort string // presumably port-forwarded to a cilium pod
	Source                  string
	Protocol                string
	Reason                  string
}

func (v *ValidateHubbleDropMetric) Run() error {
	defaultRetrier := retry.Retrier{Attempts: defaultRetryAttempts, Delay: defaultRetryDelay}

	promAddress := fmt.Sprintf("http://localhost:%s/metrics", v.PortForwardedHubblePort)
	ctx := context.Background()
	pctx, cancel := context.WithCancel(ctx)
	defer cancel()

	validMetric := map[string]string{
		destinationKey: "",
		sourceKey:      v.Source,
		protcolKey:     v.Protocol,
		reason:         v.Reason,
	}

	metrics := map[string]*promclient.MetricFamily{}
	scrapeMetricsFn := func() error {
		log.Printf("checking for drop metrics on %s", promAddress)
		var err error
		metrics, err = getPrometheusDropMetrics(promAddress)
		if err != nil {
			return fmt.Errorf("could not start port forward within %ds: %w	", defaultTimeout, err)
		}

		err = verifyLabelsPresent(metrics, validMetric)
		if err != nil {
			return fmt.Errorf("failed to find metric matching %+v: %w", validMetric, ErrNoMetricFound)
		}

		return nil
	}

	err := defaultRetrier.Do(pctx, scrapeMetricsFn)
	if err != nil {
		return fmt.Errorf("failed to get prometheus metrics: %w", err)
	}

	log.Printf("found metric matching %+v\n", validMetric)
	return nil
}

func verifyLabelsPresent(data map[string]*promclient.MetricFamily, validMetric map[string]string) error {
	for _, metric := range data {
		if metric.GetName() == "hubble_drop_total" {
			for _, metric := range metric.GetMetric() {

				// get all labels and values on the metric
				metricLabels := map[string]string{}
				for _, label := range metric.GetLabel() {
					metricLabels[label.GetName()] = label.GetValue()
				}
				if reflect.DeepEqual(metricLabels, validMetric) {
					return nil
				}
			}
		}
	}

	return fmt.Errorf("failed to find metric matching %+v: %w", validMetric, ErrNoMetricFound)
}

func getPrometheusDropMetrics(url string) (map[string]*promclient.MetricFamily, error) {
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

func parseReaderPrometheusMetrics(input io.Reader) (map[string]*promclient.MetricFamily, error) {
	var parser expfmt.TextParser
	return parser.TextToMetricFamilies(input) //nolint
}

func (v *ValidateHubbleDropMetric) Prevalidate() error {
	return nil
}

func (v *ValidateHubbleDropMetric) Stop() error {
	return nil
}
