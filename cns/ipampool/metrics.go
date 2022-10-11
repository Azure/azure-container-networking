package ipampool

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	subnetLabel                = "subnet"
	subnetCIDRLabel            = "subnet_cidr"
	podnetARMIDLabel           = "podnet_arm_id"
	customerMetricLabel        = "customer_metric"
	customerMetricLabelValue   = "customer metric"
	subnetExhaustionStateLabel = "subnet_exhaustion_state"
	subnetIPExhausted          = 1
	subnetIPNotExhausted       = 0
)

var (
	ipamAllocatedIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_pod_allocated_ips",
			Help:        "Count of IPs CNS has allocated to Pods.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
	)
	ipamAvailableIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_available_ips",
			Help:        "Available IP count.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
	)
	ipamBatchSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_batch_size",
			Help:        "IPAM IP pool batch size.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
	)
	ipamCurrentAvailableIPcount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_current_available_ips",
			Help:        "Current available IP count.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
	)
	ipamExpectedAvailableIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_expect_available_ips",
			Help:        "Expected future available IP count assuming the Requested IP count is honored.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
	)
	ipamMaxIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_max_ips",
			Help:        "Maximum IP count.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
	)
	ipamPendingProgramIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_pending_programming_ips",
			Help:        "Pending programming IP count.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
	)
	ipamPendingReleaseIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_pending_release_ips",
			Help:        "Pending release IP count.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
	)
	ipamPrimaryIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_primary_ips",
			Help:        "NC Primary IP count.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
	)
	ipamRequestedIPConfigCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_requested_ips",
			Help:        "Requested IP count.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
	)
	ipamTotalIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_total_ips",
			Help:        "Count of total IP pool size allocated to CNS by DNC.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
	)
	ipamSubnetExhaustionState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_subnet_exhaustion_state",
			Help:        "IPAM view of subnet exhaustion state",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
	)
	ipamSubnetExhaustionCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cx_ipam_subnet_exhaustion_state_count_total",
			Help: "Count of the number of times the ipam pool monitor sees subnet exhaustion",
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel, subnetExhaustionStateLabel},
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
		ipamPrimaryIPCount,
		ipamRequestedIPConfigCount,
		ipamTotalIPCount,
		ipamSubnetExhaustionState,
		ipamSubnetExhaustionCount,
	)
}

func observeIPPoolState(state ipPoolState, meta metaState) {
	labels := []string{meta.subnet, meta.subnetCIDR, meta.subnetARMID}
	ipamAllocatedIPCount.WithLabelValues(labels...).Set(float64(state.allocatedToPods))
	ipamAvailableIPCount.WithLabelValues(labels...).Set(float64(state.available))
	ipamBatchSize.WithLabelValues(labels...).Set(float64(meta.batch))
	ipamCurrentAvailableIPcount.WithLabelValues(labels...).Set(float64(state.currentAvailableIPs))
	ipamExpectedAvailableIPCount.WithLabelValues(labels...).Set(float64(state.expectedAvailableIPs))
	ipamMaxIPCount.WithLabelValues(labels...).Set(float64(meta.max))
	ipamPendingProgramIPCount.WithLabelValues(labels...).Set(float64(state.pendingProgramming))
	ipamPendingReleaseIPCount.WithLabelValues(labels...).Set(float64(state.pendingRelease))
	ipamPrimaryIPCount.WithLabelValues(labels...).Set(float64(len(meta.primaryIPAddresses)))
	ipamRequestedIPConfigCount.WithLabelValues(labels...).Set(float64(state.requestedIPs))
	ipamTotalIPCount.WithLabelValues(labels...).Set(float64(state.totalIPs))
	if meta.exhausted {
		ipamSubnetExhaustionState.WithLabelValues(labels...).Set(float64(subnetIPExhausted))
	} else {
		ipamSubnetExhaustionState.WithLabelValues(labels...).Set(float64(subnetIPNotExhausted))
	}
}
