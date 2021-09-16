package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// getValue returns a Gauge metric's value.
// This function is slow.
func getValue(gaugeMetric prometheus.Gauge) (int, error) {
	dtoMetric, err := getDTOMetric(gaugeMetric)
	if err != nil {
		return 0, err
	}
	return int(dtoMetric.Gauge.GetValue()), nil
}

// getVecValue returns a Gauge Vec metric's value, or 0 if the label doesn't exist for the metric.
// This function is slow.
func getVecValue(gaugeVecMetric *prometheus.GaugeVec, labels prometheus.Labels) (int, error) {
	return getValue(gaugeVecMetric.With(labels))
}

// getCountValue returns the number of times a Summary metric has recorded an observation.
// This function is slow.
func getCountValue(summaryMetric prometheus.Summary) (int, error) {
	dtoMetric, err := getDTOMetric(summaryMetric)
	if err != nil {
		return 0, err
	}
	return int(dtoMetric.Summary.GetSampleCount()), nil
}

// This function is slow.
func getQuantiles(summaryMetric prometheus.Summary) ([]*dto.Quantile, error) {
	dtoMetric, err := getDTOMetric(summaryMetric)
	if err != nil {
		return nil, err
	}
	return dtoMetric.Summary.GetQuantile(), nil
}

// This function is slow.
func getDTOMetric(collector prometheus.Collector) (*dto.Metric, error) {
	channel := make(chan prometheus.Metric, 1)
	collector.Collect(channel)
	metric := &dto.Metric{}
	err := (<-channel).Write(metric)
	if err != nil {
		err = fmt.Errorf("error while extracting Prometheus metric value: %w", err)
	}
	return metric, err
}
