package ipampool

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	ipamAllocatedIPCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_pod_allocated_ips",
			Help: "Count of IPs CNS has allocated to Pods.",
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
	ipamCurrentAvailableIPcount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_current_available_ips",
			Help: "Current available IP count.",
		},
	)
	ipamExpectedAvailableIPCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_expect_available_ips",
			Help: "Expected future available IP count assuming the Requested IP count is honored.",
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
	ipamTotalIPCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipam_total_ips",
			Help: "Count of total IP pool size allocated to CNS by DNC.",
		},
	)
)

func init() {
	metrics.Registry.MustRegister(
		ipamAllocatedIPCount,
		ipamAvailableIPCount,
		ipamBatchSize,
		ipamCurrentAvailableIPcount,
		ipamExpectedAvailableIPCount,
		ipamMaxIPCount,
		ipamPendingProgramIPCount,
		ipamPendingReleaseIPCount,
		ipamRequestedIPConfigCount,
		ipamTotalIPCount,
	)
}

func observeIPPoolState(state ipPoolState, meta metaState) {
	ipamAllocatedIPCount.Set(float64(state.allocatedToPods))
	ipamAvailableIPCount.Set(float64(state.available))
	ipamBatchSize.Set(float64(meta.batch))
	ipamCurrentAvailableIPcount.Set(float64(state.currentAvailableIPs))
	ipamExpectedAvailableIPCount.Set(float64(state.expectedAvailableIPs))
	ipamMaxIPCount.Set(float64(meta.max))
	ipamPendingProgramIPCount.Set(float64(state.pendingProgramming))
	ipamPendingReleaseIPCount.Set(float64(state.pendingRelease))
	ipamRequestedIPConfigCount.Set(float64(state.requestedIPs))
	ipamTotalIPCount.Set(float64(state.totalIPs))
}
