package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	// registers the pprof handlers on the default mux, which is mounted at the pprof
	// prefix when profiling is enabled.
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
	// the API does. One at a time keeps peak memory to a single copy of the cache; excess
	// requests are shed rather than queued.
	maxConcurrentCacheRequests = 1
)

type NPMRestServer struct {
	listeningAddress string
	router           *mux.Router
}

// newServer builds the API server with the deadlines and bounds that keep a slow or unfinished
// request from holding resources indefinitely. It is a separate constructor so tests can assert
// the server that is actually served, rather than the constants it is built from.
func newServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		Addr:              addr,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}
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
		rs.router.Handle(api.NPMMgrPath, loopbackOnly(rs.npmCacheHandler(npmEncoder))).Methods(http.MethodGet)
	}

	if config.Toggles.EnablePprof {
		// net/http/pprof registers every profile handler on the default mux under this
		// prefix, including subpaths such as /debug/pprof/goroutine that naming the
		// handlers individually used to miss. The prefix has no trailing slash so that
		// /debug/pprof still reaches the mux, which redirects it to the index. Mounting at
		// the pprof prefix rather than at /debug/ also keeps anything else later registered
		// on the default mux from being served here.
		rs.router.PathPrefix("/debug/pprof").Handler(loopbackOnly(http.DefaultServeMux))
	}

	// use default listening address if none is specified
	if rs.listeningAddress == "" {
		rs.listeningAddress = fmt.Sprintf("%s:%d", config.ListeningAddress, config.ListeningPort)
	}

	srv := newServer(rs.listeningAddress, rs.router)

	var lc net.ListenConfig
	listener, err := lc.Listen(context.Background(), "tcp", rs.listeningAddress)
	if err != nil {
		klog.Errorf("Failed to start NPM HTTP Server with error: %+v", err)
		return
	}

	klog.Infof("Starting NPM HTTP API on %s... ", rs.listeningAddress)
	// A graceful close is not a failure, so it must not be reported as one.
	if err := srv.Serve(netutil.LimitListener(listener, maxConcurrentConns)); err != nil && !errors.Is(err, http.ErrServerClosed) {
		klog.Errorf("NPM HTTP Server stopped with error: %+v", err)
	}
}

// loopbackOnly serves a request only when it originated on the node itself. The debug route
// returns NPM's whole policy cache and the pprof routes expose the process, and both are
// served on the host network of a privileged process, so every pod on the node can otherwise
// reach them by reading its own node address. A pod has its own network namespace and cannot
// reach the node's loopback, while the on-node tooling that consumes these routes connects
// over localhost, so this keeps the routes available to their only caller and out of reach of
// a tenant workload. The Prometheus routes are deliberately not wrapped: they are scraped
// from off the node.
func loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, err = w.Write(b)
		if err != nil {
			log.Errorf("failed to write resp: %v", err)
		}
	})
}
