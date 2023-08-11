package metrics

import "github.com/prometheus/client_golang/prometheus"

// RecordACLLatency should be used in Windows DP to record the latency of individual ACL operations.
func RecordACLLatency(timer *Timer, op OperationKind) {
	labels := prometheus.Labels{
		operationLabel: string(op),
	}
	aclLatency.With(labels).Observe(timer.timeElapsedSeconds())
}

// IncACLFailures should be used in Windows DP to record the number of failures for individual ACL operations.
func IncACLFailures(op OperationKind) {
	labels := prometheus.Labels{
		operationLabel: string(op),
	}
	aclFailures.With(labels).Inc()
}

func TotalACLLatencyCalls(op OperationKind) (int, error) {
	return histogramVecCount(aclLatency, prometheus.Labels{
		operationLabel: string(op),
	})
}

func TotalACLFailures(op OperationKind) (int, error) {
	return counterValue(aclFailures.With(prometheus.Labels{
		operationLabel: string(op),
	}))
}
