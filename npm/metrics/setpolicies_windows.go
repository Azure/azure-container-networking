package metrics

import "github.com/prometheus/client_golang/prometheus"

func RecordSetPolicyLatency(timer *Timer, op OperationKind, isNested bool) {
	nested := "false"
	if isNested {
		nested = "true"
	}
	labels := prometheus.Labels{
		operationLabel: string(op),
		isNestedLabel:  nested,
	}
	setPolicyLatency.With(labels).Observe(timer.timeElapsed())
}

func IncSetPolicyFailures(op OperationKind, isNested bool) {
	nested := "false"
	if isNested {
		nested = "true"
	}
	labels := prometheus.Labels{
		operationLabel: string(op),
		isNestedLabel:  nested,
	}
	setPolicyFailures.With(labels).Inc()
}

func RecordGetNetworkLatency(timer *Timer) {
	getNetworkLatency.Observe(timer.timeElapsed())
}

func IncGetNetworkFailures() {
	getNetworkFailures.Inc()
}

func TotalGetNetworkLatencyCalls() (int, error) {
	return histogramCount(getNetworkLatency)
}

func TotalGetNetworkFailures() (int, error) {
	return counterValue(getNetworkFailures)
}

func TotalSetPolicyLatencyCalls(op OperationKind, isNested bool) (int, error) {
	nested := "false"
	if isNested {
		nested = "true"
	}
	return histogramVecCount(setPolicyLatency, prometheus.Labels{
		operationLabel: string(op),
		isNestedLabel:  nested,
	})
}

func TotalSetPolicyFailures(op OperationKind, isNested bool) (int, error) {
	nested := "false"
	if isNested {
		nested = "true"
	}
	return counterValue(setPolicyFailures.With(prometheus.Labels{
		operationLabel: string(op),
		isNestedLabel:  nested,
	}))
}
