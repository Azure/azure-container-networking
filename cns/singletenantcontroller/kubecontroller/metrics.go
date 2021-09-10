package kubecontroller

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	assignedIPs = prometheus.NewGauge(
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
)

func init() {
	metrics.Registry.MustRegister(
		assignedIPs,
		requestedIPs,
		unusedIPs,
	)
}
