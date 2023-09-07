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
			Help:        "IPs currently in use by Pods on this CNS Node.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
	)
	ipamAvailableIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_available_ips",
			Help:        "IPs available on this CNS Node for use by a Pod.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
	)
	ipamBatchSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_batch_size",
			Help:        "IPAM IP pool scaling batch size.",
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
			Help:        "Maximum Secondary IPs allowed on this Node.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
	)
	ipamPendingProgramIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_pending_programming_ips",
			Help:        "IPs reserved but not yet available (Pending Programming).",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
	)
	ipamPendingReleaseIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_pending_release_ips",
			Help:        "IPs reserved but not available anymore (Pending Release).",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
	)
	ipamPrimaryIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_primary_ips",
			Help:        "NC Primary IP count (reserved from Pod Subnet for DNS and IMDS SNAT).",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
	)
	ipamRequestedIPConfigCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_requested_ips",
			Help:        "Secondary Pod Subnet IPs requested by this CNS Node (for Pods).",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
	)
	ipamSecondaryIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_secondary_ips",
			Help:        "Node NC Secondary IP count (reserved usable by Pods).",
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
	ipamSubnetExhaustionState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_subnet_exhaustion_state",
			Help:        "CNS view of subnet exhaustion state",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
	)
	ipamTotalIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_total_ips",
			Help:        "Total IPs reserved from the Pod Subnet by this Node.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{subnetLabel, subnetCIDRLabel, podnetARMIDLabel},
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
		ipamSecondaryIPCount,
		ipamSubnetExhaustionCount,
		ipamSubnetExhaustionState,
		ipamTotalIPCount,
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
	ipamSecondaryIPCount.WithLabelValues(labels...).Set(float64(state.secondaryIPs))
	ipamTotalIPCount.WithLabelValues(labels...).Set(float64(state.secondaryIPs + int64(len(meta.primaryIPAddresses))))
	if meta.exhausted {
		ipamSubnetExhaustionState.WithLabelValues(labels...).Set(float64(subnetIPExhausted))
	} else {
		ipamSubnetExhaustionState.WithLabelValues(labels...).Set(float64(subnetIPNotExhausted))
	}
}
