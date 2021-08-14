package restserver

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var requestLatency = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name: "request_latency",
		Help: "Request latency in seconds by endpoint, verb, and response code.",
		//nolint:gomnd
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1 ms to ~16 seconds
	},
	[]string{"url", "verb", "code"},
)

func init() {
	metrics.Registry.MustRegister(
		requestLatency,
	)
}

func newHandlerFuncWithHistogram(handler http.HandlerFunc, histogram *prometheus.HistogramVec) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		defer func() {
			histogram.WithLabelValues(req.URL.RequestURI(), req.Method, "0").Observe(time.Since(start).Seconds())
		}()
		handler(w, req)
	}
}
