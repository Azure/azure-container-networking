package nodenetworkconfig

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	allocatedIPs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "allocated_ips",
			Help: "Allocated IP count.",
		},
	)
	requestedIPs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "requested_ips",
			Help: "Requested IP count.",
		},
	)
	unusedIPs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "unused_ips",
			Help: "Unused IP count.",
		},
	)
	hasNNC = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "nnc_has_nodenetworkconfig",
			Help: "Has received a NodeNetworkConfig",
		},
	)
	ncs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "nnc_ncs",
			Help: "Network Container count in the NodeNetworkConfig",
		},
	)
)

func init() {
	metrics.Registry.MustRegister(
		allocatedIPs,
		requestedIPs,
		unusedIPs,
		hasNNC,
		ncs,
	)
}
