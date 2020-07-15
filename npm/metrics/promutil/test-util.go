package promutil

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// NotifyIfErrors writes any non-nil errors to a testing utility
func NotifyIfErrors(t *testing.T, errors ...error) {
	allGood := true
	for _, err := range errors {
		if err != nil {
			allGood = false
			break
		}
	}
	if !allGood {
		t.Errorf("Encountered these errors while getting metric values: ")
		for _, err := range errors {
			if err != nil {
				t.Errorf("%v", err)
			}
		}
	}
}

// GetValue is used for validation. It returns a Gauge metric's value.
func GetValue(gaugeMetric prometheus.Gauge) (int, error) {
	dtoMetric, err := getDTOMetric(gaugeMetric)
	if err != nil {
		return 0, err
	}
	return int(dtoMetric.Gauge.GetValue()), nil
}

// GetVecValue is used for validation. It returns a Gauge Vec metric's value.
func GetVecValue(gaugeVecMetric *prometheus.GaugeVec, labels prometheus.Labels) (int, error) {
	return GetValue(gaugeVecMetric.With(labels))
}

// GetCountValue is used for validation. It returns the number of times a Summary metric has recorded an observation.
func GetCountValue(summaryMetric prometheus.Summary) (int, error) {
	dtoMetric, err := getDTOMetric(summaryMetric)
	if err != nil {
		return 0, err
	}
	return int(dtoMetric.Summary.GetSampleCount()), nil
}

func getDTOMetric(collector prometheus.Collector) (*dto.Metric, error) {
	channel := make(chan prometheus.Metric, 1)
	collector.Collect(channel)
	metric := &dto.Metric{}
	err := (<-channel).Write(metric)
	return metric, err
}
