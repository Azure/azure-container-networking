package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// managerStartFailures is a monotic counter which tracks the number of times the controller-runtime
// manager failed to start. To drive alerting based on this metric, it is recommended to use the rate
// of increase over a period of time. A positive rate of change indicates that the CNS is actively
// failing and retrying.
var managerStartFailures = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "manager_start_failures_total",
		Help: "Number of times the controller-runtime manager failed to start.",
	},
)

func init() {
	metrics.Registry.MustRegister(
		managerStartFailures,
	)
}
