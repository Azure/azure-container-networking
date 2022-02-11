package ipampool

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const subnetLabel string = "Subnet"
const subnetCIDRLabel string = "SubnetCIDR"

var (
	ipamAllocatedIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ipam_allocated_ips",
			Help: "CNS's allocated IP pool size.",
		},
		[]string{subnetLabel, subnetCIDRLabel},
	)
	ipamAssignedIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ipam_assigned_ips",
			Help: "Assigned IP count.",
		},
		[]string{subnetLabel, subnetCIDRLabel},
	)
	ipamAvailableIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ipam_available_ips",
			Help: "Available IP count.",
		},
		[]string{subnetLabel, subnetCIDRLabel},
	)
	ipamBatchSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ipam_batch_size",
			Help: "IPAM IP pool batch size.",
		},
		[]string{subnetLabel, subnetCIDRLabel},
	)
	ipamMaxIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ipam_max_ips",
			Help: "Maximum IP count.",
		},
		[]string{subnetLabel, subnetCIDRLabel},
	)
	ipamPendingProgramIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ipam_pending_programming_ips",
			Help: "Pending programming IP count.",
		},
		[]string{subnetLabel, subnetCIDRLabel},
	)
	ipamPendingReleaseIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ipam_pending_release_ips",
			Help: "Pending release IP count.",
		},
		[]string{subnetLabel, subnetCIDRLabel},
	)
	ipamRequestedIPConfigCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ipam_requested_ips",
			Help: "Requested IP count.",
		},
		[]string{subnetLabel, subnetCIDRLabel},
	)
	ipamRequestedUnassignedIPConfigCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ipam_requested_unassigned_ips",
			Help: "Future unassigned IP count assuming the Requested IP count is honored.",
		},
		[]string{subnetLabel, subnetCIDRLabel},
	)
	ipamUnassignedIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ipam_unassigned_ips",
			Help: "Unassigned IP count.",
		},
		[]string{subnetLabel, subnetCIDRLabel},
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

func observeIPPoolState(state ipPoolState, meta metaState, labels []string) {
	ipamAllocatedIPCount.WithLabelValues(labels...).Set(float64(state.allocated))
	ipamAssignedIPCount.WithLabelValues(labels...).Set(float64(state.assigned))
	ipamAvailableIPCount.WithLabelValues(labels...).Set(float64(state.available))
	ipamBatchSize.WithLabelValues(labels...).Set(float64(meta.batch))
	ipamMaxIPCount.WithLabelValues(labels...).Set(float64(meta.max))
	ipamPendingProgramIPCount.WithLabelValues(labels...).Set(float64(state.pendingProgramming))
	ipamPendingReleaseIPCount.WithLabelValues(labels...).Set(float64(state.pendingRelease))
	ipamRequestedIPConfigCount.WithLabelValues(labels...).Set(float64(state.requested))
	ipamRequestedUnassignedIPConfigCount.WithLabelValues(labels...).Set(float64(state.requestedUnassigned))
	ipamUnassignedIPCount.WithLabelValues(labels...).Set(float64(state.unassigned))
}
