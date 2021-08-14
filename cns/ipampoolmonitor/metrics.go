package ipampoolmonitor

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	ipamAllocatedIPCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_allocated_ip",
			Help: "Allocated IP count.",
		},
	)
	ipamAvailableIPCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_available_ip",
			Help: "Available IP count.",
		},
	)
	ipamBatchSize = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_batch",
			Help: "IPAM IP pool batch size.",
		},
	)
	ipamFreeIPCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_free_ip",
			Help: "Free IP count.",
		},
	)
	ipamIPPool = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_ip_pool",
			Help: "IP pool size.",
		},
	)
	ipamMaxIPCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_max_ip",
			Help: "Maximum IP count.",
		},
	)
	ipamPendingProgramIPCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_pending_programming_ip",
			Help: "Pending programming IP count.",
		},
	)
	ipamPendingReleaseIPCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_pending_release_ip",
			Help: "Pending release IP count.",
		},
	)
)

func init() {
	metrics.Registry.MustRegister(
		ipamAllocatedIPCount,
		ipamAvailableIPCount,
		ipamBatchSize,
		ipamFreeIPCount,
		ipamIPPool,
		ipamMaxIPCount,
		ipamPendingProgramIPCount,
		ipamPendingReleaseIPCount,
	)
}
