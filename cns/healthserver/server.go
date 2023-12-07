package healthserver

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

func Start(log *zap.Logger, addr string, readyz, healthz http.Handler) {
	e := echo.New()
	e.HideBanner = true
	e.GET("/healthz", echo.WrapHandler(http.StripPrefix("/healthz", healthz)))
	e.GET("/readyz", echo.WrapHandler(http.StripPrefix("/readyz", readyz)))
	e.GET("/metrics", echo.WrapHandler(promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{
		ErrorHandling: promhttp.HTTPErrorOnError,
	})))
	if err := e.Start(addr); err != nil {
		log.Error("failed to run healthserver", zap.Error(err))
	}
}
