package ipampool

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	ipamAllocatedIPCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_allocated_ips",
			Help: "CNS's allocated IP pool size.",
		},
	)
	ipamAssignedIPCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_assigned_ips",
			Help: "Assigned IP count.",
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
	ipamRequestedUnassignedIPConfigCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_requested_unassigned_ips",
			Help: "Future unassigned IP count assuming the Requested IP count is honored.",
		},
	)
	ipamUnassignedIPCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_unassigned_ips",
			Help: "Unassigned IP count.",
		},
	)
)

func init() {
	metrics.Registry.MustRegister(
		ipamAllocatedIPCount,
		ipamAssignedIPCount,
		ipamAvailableIPCount,
		ipamBatchSize,
		ipamMaxIPCount,
		ipamPendingProgramIPCount,
		ipamPendingReleaseIPCount,
		ipamRequestedIPConfigCount,
		ipamRequestedUnassignedIPConfigCount,
		ipamUnassignedIPCount,
	)
}

func observeIPPoolState(state ipPoolState, meta metaState) {
	ipamAllocatedIPCount.Set(float64(state.allocated))
	ipamAssignedIPCount.Set(float64(state.assigned))
	ipamAvailableIPCount.Set(float64(state.available))
	ipamBatchSize.Set(float64(meta.batch))
	ipamMaxIPCount.Set(float64(meta.max))
	ipamPendingProgramIPCount.Set(float64(state.pendingProgramming))
	ipamPendingReleaseIPCount.Set(float64(state.pendingRelease))
	ipamRequestedIPConfigCount.Set(float64(state.requested))
	ipamRequestedUnassignedIPConfigCount.Set(float64(state.requestedUnassigned))
	ipamUnassignedIPCount.Set(float64(state.unassigned))
}
