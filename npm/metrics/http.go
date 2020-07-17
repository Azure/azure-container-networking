package metrics

import (
	"net/http"
	"time"

	"github.com/Azure/azure-container-networking/log"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	// HTTPPort is the port used by the HTTP server (includes a preceding colon)
	HTTPPort = ":8000"

	//MetricsPath is the path for the Prometheus metrics endpoint (includes preceding slash)
	MetricsPath = "/metrics"
)

var started = false
var handler http.Handler

// StartHTTP starts a HTTP server in a Go routine with endpoint on port 8000. Metrics are exposed on the endpoint /metrics.
// By being exposed, the metrics can be scraped by a Prometheus Server or Container Insights.
// The function will pause for delayAmountAfterStart seconds after starting the HTTP server for the first time.
func StartHTTP(delayAmountAfterStart int) {
	if started {
		return
	}
	started = true

	http.Handle(MetricsPath, getHandler())
	log.Logf("Starting Prometheus HTTP Server")
	go http.ListenAndServe(HTTPPort, nil)
	time.Sleep(time.Second * time.Duration(delayAmountAfterStart))
}

// getHandler returns the HTTP handler for the metrics endpoint
func getHandler() http.Handler {
	if handler == nil {
		handler = promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	}
	return handler
}
