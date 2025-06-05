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
		Name: "cns_ctrlmanager_start_failures_total",
		Help: "Number of times the controller-runtime manager failed to start.",
	},
)

// managerStartTimeouts is a monotic counter which tracks the number of times the controller-runtime
// manager timed out during startup. To drive alerting based on this metric, it is recommended to use the rate
// of increase over a period of time. A positive rate of change indicates that the CNS is actively
// failing with timeouts.
var managerStartTimeouts = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "cns_ctrlmanager_start_timeouts_total",
		Help: "Number of times the controller-runtime manager timed out during startup.",
	},
)

// multitenantControllerStartTimeouts is a monotic counter which tracks the number of times the multitenant
// controller timed out during startup. To drive alerting based on this metric, it is recommended to use the rate
// of increase over a period of time. A positive rate of change indicates that the CNS is actively
// failing with timeouts.
var multitenantControllerStartTimeouts = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "cns_multitenantcontroller_start_timeouts_total",
		Help: "Number of times the multitenant controller timed out during startup.",
	},
)

// nncReconcilerStartFailures is a monotic counter which tracks the number of times the NNC reconciler
// has failed to start within the timeout period. To drive alerting based on this metric, it is
// recommended to use the rate of increase over a period of time. A positive rate of change indicates
// that the CNS is actively failing and retrying.
var nncReconcilerStartFailures = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "cns_nnc_reconciler_start_failures_total",
		Help: "Number of times the NNC reconciler has failed to start within the timeout period.",
	},
)

func init() {
	metrics.Registry.MustRegister(
		managerStartFailures,
		managerStartTimeouts,
		multitenantControllerStartTimeouts,
		nncReconcilerStartFailures,
	)
}
