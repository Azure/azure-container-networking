package server

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	_ "net/http/pprof"

	"k8s.io/klog"

	"github.com/Azure/azure-container-networking/npm/cache"
	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	"github.com/Azure/azure-container-networking/npm/http/api"
	"github.com/Azure/azure-container-networking/npm/metrics"

	"github.com/Azure/azure-container-networking/npm"
	"github.com/gorilla/mux"
)

type NPMRestServer struct {
	listeningAddress string
	server           *http.Server
	router           *mux.Router
}

func (n *NPMRestServer) NPMRestServerListenAndServe(config npmconfig.Config, npmEncoder npm.NetworkPolicyManagerEncoder) {
	n.router = mux.NewRouter()

	//prometheus handlers
	if config.Toggles.EnablePrometheusMetrics {
		rs.router.Handle(api.NodeMetricsPath, metrics.GetHandler(true))
		rs.router.Handle(api.ClusterMetricsPath, metrics.GetHandler(false))
	}

	if config.Toggles.EnableHTTPDebugAPI {
		// ACN CLI debug handlerss
		n.router.Handle(api.NPMMgrPath, n.npmCacheHandler(npmEncoder)).Methods(http.MethodGet)
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

func (n *NPMRestServer) npmCacheHandler(npmEncoder npm.NetworkPolicyManagerEncoder) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := cache.Encode(w, npmEncoder)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	})
}
