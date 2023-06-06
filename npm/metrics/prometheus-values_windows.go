package metrics

import "github.com/prometheus/client_golang/prometheus"

func counterValue(counter prometheus.Counter) (int, error) {
	dtoMetric, err := getDTOMetric(counter)
	if err != nil {
		return 0, err
	}
	return int(dtoMetric.Counter.GetValue()), nil
}

func histogramVecCount(histogramVec *prometheus.HistogramVec, labels prometheus.Labels) (int, error) {
	collector, ok := histogramVec.With(labels).(prometheus.Collector)
	if !ok {
		return 0, errNotCollector
	}
	return histogramCount(collector)
}

func histogramCount(histogram prometheus.Collector) (int, error) {
	dtoMetric, err := getDTOMetric(histogram)
	if err != nil {
		return 0, err
	}
	return int(dtoMetric.Histogram.GetSampleCount()), nil
}
