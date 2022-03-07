package ipampool

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	subnetLabel     = "subnet"
	subnetCIDRLabel = "subnet_cidr"
)

var (
	ipamAllocatedIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ipam_pod_allocated_ips",
			Help: "Count of IPs CNS has allocated to Pods.",
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
	ipamCurrentAvailableIPcount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ipam_current_available_ips",
			Help: "Current available IP count.",
		},
		[]string{subnetLabel, subnetCIDRLabel},
	)
	ipamExpectedAvailableIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ipam_expect_available_ips",
			Help: "Expected future available IP count assuming the Requested IP count is honored.",
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
	ipamTotalIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ipam_total_ips",
			Help: "Count of total IP pool size allocated to CNS by DNC.",
		},
		[]string{subnetLabel, subnetCIDRLabel},
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

func observeIPPoolState(state ipPoolState, meta metaState, labels []string) {
	ipamAllocatedIPCount.WithLabelValues(labels...).Set(float64(state.allocatedToPods))
	ipamAvailableIPCount.WithLabelValues(labels...).Set(float64(state.available))
	ipamBatchSize.WithLabelValues(labels...).Set(float64(meta.batch))
	ipamCurrentAvailableIPcount.WithLabelValues(labels...).Set(float64(state.currentAvailableIPs))
	ipamExpectedAvailableIPCount.WithLabelValues(labels...).Set(float64(state.expectedAvailableIPs))
	ipamMaxIPCount.WithLabelValues(labels...).Set(float64(meta.max))
	ipamPendingProgramIPCount.WithLabelValues(labels...).Set(float64(state.pendingProgramming))
	ipamPendingReleaseIPCount.WithLabelValues(labels...).Set(float64(state.pendingRelease))
	ipamRequestedIPConfigCount.WithLabelValues(labels...).Set(float64(state.requestedIPs))
	ipamTotalIPCount.WithLabelValues(labels...).Set(float64(state.totalIPs))
}
