package healthserver

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

func Start(log *zap.Logger, addr string) {
	e := echo.New()

	e.GET("/healthz", healthz)
	e.GET("/metrics", echo.WrapHandler(promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{
		ErrorHandling: promhttp.HTTPErrorOnError,
	})))
	if err := e.Start(addr); err != nil {
		log.Error("failed to run healthserver", zap.Error(err))
	}
}

func healthz(c echo.Context) error {
	return c.NoContent(http.StatusOK) //nolint:wrapcheck // ignore
}
