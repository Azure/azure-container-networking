package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	SubnetLabel                = "subnet"
	SubnetCIDRLabel            = "subnet_cidr"
	PodnetARMIDLabel           = "podnet_arm_id"
	customerMetricLabel        = "customer_metric"
	customerMetricLabelValue   = "customer metric"
	SubnetExhaustionStateLabel = "subnet_exhaustion_state"
	SubnetIPExhausted          = 1
	SubnetIPNotExhausted       = 0
)

var (
	IpamAllocatedIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_pod_allocated_ips",
			Help:        "IPs currently in use by Pods on this CNS Node.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{SubnetLabel, SubnetCIDRLabel, PodnetARMIDLabel},
	)
	IpamAvailableIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_available_ips",
			Help:        "IPs available on this CNS Node for use by a Pod.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{SubnetLabel, SubnetCIDRLabel, PodnetARMIDLabel},
	)
	IpamBatchSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_batch_size",
			Help:        "IPAM IP pool scaling batch size.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{SubnetLabel, SubnetCIDRLabel, PodnetARMIDLabel},
	)
	IpamCurrentAvailableIPcount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_current_available_ips",
			Help:        "Current available IP count.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{SubnetLabel, SubnetCIDRLabel, PodnetARMIDLabel},
	)
	IpamExpectedAvailableIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_expect_available_ips",
			Help:        "Expected future available IP count assuming the Requested IP count is honored.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{SubnetLabel, SubnetCIDRLabel, PodnetARMIDLabel},
	)
	IpamMaxIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_max_ips",
			Help:        "Maximum Secondary IPs allowed on this Node.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{SubnetLabel, SubnetCIDRLabel, PodnetARMIDLabel},
	)
	IpamPendingProgramIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_pending_programming_ips",
			Help:        "IPs reserved but not yet available (Pending Programming).",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{SubnetLabel, SubnetCIDRLabel, PodnetARMIDLabel},
	)
	IpamPendingReleaseIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_pending_release_ips",
			Help:        "IPs reserved but not available anymore (Pending Release).",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{SubnetLabel, SubnetCIDRLabel, PodnetARMIDLabel},
	)
	IpamPrimaryIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_primary_ips",
			Help:        "NC Primary IP count (reserved from Pod Subnet for DNS and IMDS SNAT).",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{SubnetLabel, SubnetCIDRLabel, PodnetARMIDLabel},
	)
	IpamRequestedIPConfigCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_requested_ips",
			Help:        "Secondary Pod Subnet IPs requested by this CNS Node (for Pods).",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{SubnetLabel, SubnetCIDRLabel, PodnetARMIDLabel},
	)
	IpamSecondaryIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_secondary_ips",
			Help:        "Node NC Secondary IP count (reserved usable by Pods).",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{SubnetLabel, SubnetCIDRLabel, PodnetARMIDLabel},
	)
	IpamTotalIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_total_ips",
			Help:        "Count of total IP pool size allocated to CNS by DNC.",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{SubnetLabel, SubnetCIDRLabel, PodnetARMIDLabel},
	)
	IpamSubnetExhaustionState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_ipam_subnet_exhaustion_state",
			Help:        "IPAM view of subnet exhaustion state",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{SubnetLabel, SubnetCIDRLabel, PodnetARMIDLabel},
	)
	IpamSubnetExhaustionCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cx_ipam_subnet_exhaustion_state_count_total",
			Help: "Count of the number of times the ipam pool monitor sees subnet exhaustion",
		},
		[]string{SubnetLabel, SubnetCIDRLabel, PodnetARMIDLabel, SubnetExhaustionStateLabel},
	)
)

func init() {
	metrics.Registry.MustRegister(
		IpamAllocatedIPCount,
		IpamAvailableIPCount,
		IpamBatchSize,
		IpamCurrentAvailableIPcount,
		IpamExpectedAvailableIPCount,
		IpamMaxIPCount,
		IpamPendingProgramIPCount,
		IpamPendingReleaseIPCount,
		IpamPrimaryIPCount,
		IpamSecondaryIPCount,
		IpamRequestedIPConfigCount,
		IpamTotalIPCount,
		IpamSubnetExhaustionState,
		IpamSubnetExhaustionCount,
	)
}
