package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

func RecordIPTablesRestoreLatency(timer *Timer, op OperationKind) {
	labels := prometheus.Labels{
		operationLabel: string(op),
	}
	itpablesRestoreLatency.With(labels).Observe(timer.timeElapsed())
}

func RecordIPTablesDeleteLatency(timer *Timer) {
	iptablesDeleteLatency.Observe(timer.timeElapsed())
}

func IncIPTablesRestoreFailures(op OperationKind) {
	labels := prometheus.Labels{
		operationLabel: string(op),
	}
	iptablesRestoreFailures.With(labels).Inc()
}

func TotalIPTablesRestoreLatencyCalls(op OperationKind) (int, error) {
	return histogramVecCount(itpablesRestoreLatency, prometheus.Labels{
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

func TotalIPTablesRestoreFailures(op OperationKind) (int, error) {
	return counterValue(iptablesRestoreFailures.With(prometheus.Labels{
		operationLabel: string(op),
	}))
}
