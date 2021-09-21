package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/pprof"
	_ "net/http/pprof"

	"github.com/Azure/azure-container-networking/log"
	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	"github.com/Azure/azure-container-networking/npm/http/api"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"k8s.io/klog"

	"github.com/gorilla/mux"
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
		rs.router.Handle(api.NodeMetricsPath, metrics.GetHandler(true))
		rs.router.Handle(api.ClusterMetricsPath, metrics.GetHandler(false))
	}

	if config.Toggles.EnableHTTPDebugAPI {
		// ACN CLI debug handlerss
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
		Handler: rs.router,
		Addr:    rs.listeningAddress,
	}

	klog.Infof("Starting NPM HTTP API on %s... ", rs.listeningAddress)
	klog.Errorf("Failed to start NPM HTTP Server with error: %+v", srv.ListenAndServe())
}

func (n *NPMRestServer) npmCacheHandler(npmCacheEncoder json.Marshaler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := json.Marshal(npmCacheEncoder)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		_, err = w.Write(b)
		if err != nil {
			log.Errorf("failed to write resp: %w", err)
		}
	})
}
