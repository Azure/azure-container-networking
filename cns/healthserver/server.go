package healthserver

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

func Start(log *zap.Logger, addr string) {
	e := echo.New()
	e.HideBanner = true
	e.GET("/healthz", echo.WrapHandler(http.StripPrefix("/healthz", &healthz.Handler{})))
	e.GET("/metrics", echo.WrapHandler(promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{
		ErrorHandling: promhttp.HTTPErrorOnError,
	})))
	if err := e.Start(addr); err != nil {
		log.Error("failed to run healthserver", zap.Error(err))
	}
}
