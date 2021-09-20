package restserver

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var httpRequestLatency = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name: "http_request_latency_seconds",
		Help: "Request latency in seconds by endpoint, verb, and response code.",
		//nolint:gomnd // default bucket consts
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1 ms to ~16 seconds
	},
	// TODO(rbtr):
	// there's no easy way to extract the HTTP response code from the response due to the
	// way the restserver is designed currently - but we should fix that and include "code" as
	// a label and value.
	[]string{"url", "verb"},
)

var ipAllocationLatency = prometheus.NewHistogram(
	prometheus.HistogramOpts{
		Name: "ip_allocation_latency_seconds",
		Help: "IP allocation latency in seconds",
		//nolint:gomnd // default bucket consts
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1 ms to ~16 seconds
	},
)

func init() {
	metrics.Registry.MustRegister(
		httpRequestLatency,
		ipAllocationLatency,
	)
}

func newHandlerFuncWithHistogram(handler http.HandlerFunc, histogram *prometheus.HistogramVec) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		defer func() {
			histogram.WithLabelValues(req.URL.RequestURI(), req.Method).Observe(time.Since(start).Seconds())
		}()
		handler(w, req)
	}
}
