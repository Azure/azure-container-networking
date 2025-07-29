package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// managerStartFailures is a monotic counter which tracks the number of times the controller-runtime
	// manager failed to start. To drive alerting based on this metric, it is recommended to use the rate
	// of increase over a period of time. A positive rate of change indicates that the CNS is actively
	// failing and retrying.
	managerStartFailures = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "cns_ctrlmanager_start_failures_total",
			Help: "Number of times the controller-runtime manager failed to start.",
		},
	)
	// nncReconcilerStartFailures is a monotic counter which tracks the number of times the NNC reconciler
	// has failed to start within the timeout period. To drive alerting based on this metric, it is
	// recommended to use the rate of increase over a period of time. A positive rate of change indicates
	// that the CNS is actively failing and retrying.
	nncReconcilerStartFailures = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "cns_nnc_reconciler_start_failures_total",
			Help: "Number of times the NNC reconciler has failed to start within the timeout period.",
		},
	)
	// nncInitFailure is a monotic counter which tracks the number of times the initial NNC reconcile
	// has failed.
	nncInitFailure = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "cns_nnc_init_failures_total",
			Help: "Number of times the initial NNC reconcile has failed.",
		},
	)
	// hasNNCInitialized is a gauge which tracks whether the initial NNC reconcile has completed.
	hasNNCInitialized = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "cns_nnc_initialized",
			Help: "Whether the initial NNC reconcile has completed.",
		},
	)
)

func init() {
	metrics.Registry.MustRegister(
		managerStartFailures,
		nncReconcilerStartFailures,
		nncInitFailure,
		hasNNCInitialized,
	)
}
