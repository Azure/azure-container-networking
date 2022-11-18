package clustersubnetstate

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Constants to describe the error state boolean values for the cluster subnet state
const (
	cssReconcilerCRDWatcherStateLabel = "css_reconciler_crd_watcher_status"
)

var cssReconcilerErrorCount = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "cluster_subnet_state_reconciler_crd_watcher_status_count_total",
		Help: "Number of errors in reconciler while watching CRD for subnet exhaustion",
	},
	[]string{cssReconcilerCRDWatcherStateLabel},
)

func init() {
	metrics.Registry.MustRegister(
		cssReconcilerErrorCount,
	)
}
