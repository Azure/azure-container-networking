package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	_ "net/http/pprof"
	"time"

	"github.com/Azure/azure-container-networking/log"
	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	"github.com/Azure/azure-container-networking/npm/http/api"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"golang.org/x/net/netutil"
	"k8s.io/klog"

	"github.com/gorilla/mux"
)

const (
	// The NPM API listens on the host network of a privileged process, so any pod on the node
	// can reach it. Without deadlines a client that opens connections and then reads its
	// response one byte at a time keeps a request, and the response buffer built for it, alive
	// indefinitely. These deadlines bound how long any single client can hold those resources.
	// They are generous enough for a Prometheus scrape of this endpoint.
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 60 * time.Second
	idleTimeout       = 120 * time.Second
	maxHeaderBytes    = 1 << 16 // 64 KiB

	// maxConcurrentConns bounds how many connections the API serves at once. Each in-flight
	// request to the cache handler buffers a full copy of the policy cache, so without a
	// ceiling the number of concurrent clients alone decides how much memory NPM allocates.
	maxConcurrentConns = 32

	// maxConcurrentCacheRequests bounds how many cache encodings run at once. The encoding
	// holds the cache lock and buffers the whole payload, so it is the most expensive thing
	// the API does and is kept well below the connection ceiling.
	maxConcurrentCacheRequests = 2
)

type NPMRestServer struct {
	listeningAddress string
	router           *mux.Router
}

func NPMRestServerListenAndServe(config npmconfig.Config, npmEncoder json.Marshaler) {
	rs := NPMRestServer{}

	rs.router = mux.NewRouter()

	// prometheus handlers
	if config.Toggles.EnablePrometheusMetrics {
		rs.router.Handle(api.NodeMetricsPath, metrics.GetHandler(metrics.NodeMetrics))
		rs.router.Handle(api.ClusterMetricsPath, metrics.GetHandler(metrics.ClusterMetrics))
	}

	// the nil check is for fan-out npm
	if config.Toggles.EnableHTTPDebugAPI && npmEncoder != nil {
		// ACN CLI debug handlers
		rs.router.Handle(api.NPMMgrPath, rs.npmCacheHandler(npmEncoder)).Methods(http.MethodGet)
	}

	if config.Toggles.EnablePprof {
		rs.router.PathPrefix("/debug/").Handler(http.DefaultServeMux)
		rs.router.HandleFunc("/debug/pprof/", pprof.Index)
		rs.router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		rs.router.HandleFunc("/debug/pprof/profile", pprof.Profile)
		rs.router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		rs.router.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	// use default listening address if none is specified
	if rs.listeningAddress == "" {
		rs.listeningAddress = fmt.Sprintf("%s:%d", config.ListeningAddress, config.ListeningPort)
	}

	srv := &http.Server{
		Handler:           rs.router,
		Addr:              rs.listeningAddress,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}

	listener, err := net.Listen("tcp", rs.listeningAddress)
	if err != nil {
		klog.Errorf("Failed to start NPM HTTP Server with error: %+v", err)
		return
	}

	klog.Infof("Starting NPM HTTP API on %s... ", rs.listeningAddress)
	klog.Errorf("Failed to start NPM HTTP Server with error: %+v", srv.Serve(netutil.LimitListener(listener, maxConcurrentConns)))
}

func (n *NPMRestServer) npmCacheHandler(npmCacheEncoder json.Marshaler) http.Handler {
	// Admit only a few encodings at a time. Each one takes the cache lock and buffers the
	// entire policy cache, so concurrent requests multiply both the lock hold time and the
	// memory in flight.
	inFlight := make(chan struct{}, maxConcurrentCacheRequests)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case inFlight <- struct{}{}:
			defer func() { <-inFlight }()
		default:
			http.Error(w, "too many concurrent cache requests", http.StatusServiceUnavailable)
			return
		}

		b, err := json.Marshal(npmCacheEncoder)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		_, err = w.Write(b)
		if err != nil {
			log.Errorf("failed to write resp: %v", err)
		}
	})
}
