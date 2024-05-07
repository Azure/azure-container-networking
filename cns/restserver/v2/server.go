package v2

import (
	"context"
	"net/http"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
)

type Server struct {
	*restserver.HTTPRestService
}

func New(s *restserver.HTTPRestService) *Server {
	return &Server{s}
}

func (s Server) Start(ctx context.Context, addr string) error {
	e := echo.New()
	e.HideBanner = true
	e.POST(cns.RequestIPConfig, echo.WrapHandler(restserver.NewHandlerFuncWithHistogram(s.RequestIPConfigHandler, restserver.HTTPRequestLatency)))
	e.POST(cns.RequestIPConfigs, echo.WrapHandler(restserver.NewHandlerFuncWithHistogram(s.RequestIPConfigsHandler, restserver.HTTPRequestLatency)))
	e.POST(cns.ReleaseIPConfig, echo.WrapHandler(restserver.NewHandlerFuncWithHistogram(s.ReleaseIPConfigHandler, restserver.HTTPRequestLatency)))
	e.POST(cns.ReleaseIPConfigs, echo.WrapHandler(restserver.NewHandlerFuncWithHistogram(s.ReleaseIPConfigsHandler, restserver.HTTPRequestLatency)))
	e.POST(cns.PathDebugIPAddresses, echo.WrapHandler(http.HandlerFunc(s.HandleDebugIPAddresses)))
	e.POST(cns.PathDebugPodContext, echo.WrapHandler(http.HandlerFunc(s.HandleDebugPodContext)))
	e.POST(cns.PathDebugRestData, echo.WrapHandler(http.HandlerFunc(s.HandleDebugRestData)))
	e.POST(cns.GetNetworkContainerByOrchestratorContext, echo.WrapHandler(http.HandlerFunc(s.GetNetworkContainerByOrchestratorContext)))
	e.POST(cns.GetAllNetworkContainers, echo.WrapHandler(http.HandlerFunc(s.GetAllNetworkContainers)))
	e.POST(cns.CreateHostNCApipaEndpointPath, echo.WrapHandler(http.HandlerFunc(s.CreateHostNCApipaEndpoint)))
	e.POST(cns.DeleteHostNCApipaEndpointPath, echo.WrapHandler(http.HandlerFunc(s.DeleteHostNCApipaEndpoint)))

	// for handlers 2.0
	e.POST(cns.V2Prefix+cns.GetNetworkContainerByOrchestratorContext, echo.WrapHandler(http.HandlerFunc(s.GetNetworkContainerByOrchestratorContext)))
	e.POST(cns.V2Prefix+cns.GetAllNetworkContainers, echo.WrapHandler(http.HandlerFunc(s.GetAllNetworkContainers)))
	e.POST(cns.V2Prefix+cns.CreateHostNCApipaEndpointPath, echo.WrapHandler(http.HandlerFunc(s.CreateHostNCApipaEndpoint)))
	e.POST(cns.V2Prefix+cns.DeleteHostNCApipaEndpointPath, echo.WrapHandler(http.HandlerFunc(s.DeleteHostNCApipaEndpoint)))

	if err := e.Start(addr); err != nil {
		logger.Errorf("failed to run echo server due to %+v", err)
		return errors.Wrap(err, "failed to start echo server")
	}

	// after context is done, shutdown local server
	<-ctx.Done()
	if err := e.Shutdown(ctx); err != nil {
		logger.Errorf("failed to shutdown echo server due to %+v", err)
		return errors.Wrap(err, "failed to shutdown echo server")
	}

	return nil
}
