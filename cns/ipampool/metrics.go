package ipampool

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	ipamAllocatedIPCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_allocated_ips",
			Help: "Allocated IP count.",
		},
	)
	ipamAvailableIPCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_available_ips",
			Help: "Available IP count.",
		},
	)
	ipamBatchSize = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_batch_size",
			Help: "IPAM IP pool batch size.",
		},
	)
	ipamFreeIPCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_free_ips",
			Help: "Free IP count.",
		},
	)
	ipamIPPool = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_ip_pool_size",
			Help: "IP pool size.",
		},
	)
	ipamMaxIPCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_max_ips",
			Help: "Maximum IP count.",
		},
	)
	ipamPendingProgramIPCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_pending_programming_ips",
			Help: "Pending programming IP count.",
		},
	)
	ipamPendingReleaseIPCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_pending_release_ips",
			Help: "Pending release IP count.",
		},
	)
	ipamRequestedIPConfigCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_requested_ips",
			Help: "Requested IP count.",
		},
	)
	ipamUnallocatedIPCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_unallocated_ips",
			Help: "Unallocated IP count.",
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
