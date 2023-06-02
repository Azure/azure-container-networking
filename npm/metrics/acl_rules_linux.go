package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

func RecordIPTablesBackgroundRestoreLatency(timer *Timer, op OperationKind) {
	labels := prometheus.Labels{
		operationLabel: string(op),
	}
	iptablesBackgroundRestoreLatency.With(labels).Observe(timer.timeElapsed())
}

func RecordIPTablesDeleteLatency(timer *Timer) {
	iptablesDeleteLatency.Observe(timer.timeElapsed())
}

func IncIPTablesBackgroundRestoreFailures(op OperationKind) {
	labels := prometheus.Labels{
		operationLabel: string(op),
	}
	iptablesBackgroundRestoreFailures.With(labels).Inc()
}

func TotalIPTablesBackgroundRestoreLatencyCalls(op OperationKind) (int, error) {
	return histogramVecCount(iptablesBackgroundRestoreLatency, prometheus.Labels{
		operationLabel: string(op),
	})
}

func TotalIPTablesDeleteLatencyCalls() (int, error) {
	collector, ok := iptablesDeleteLatency.(prometheus.Collector)
	if !ok {
		return 0, errNotCollector
	}
	return histogramCount(collector)
}

func TotalIPTablesBackgroundRestoreFailures(op OperationKind) (int, error) {
	return counterValue(iptablesBackgroundRestoreFailures.With(prometheus.Labels{
		operationLabel: string(op),
	}))
}
