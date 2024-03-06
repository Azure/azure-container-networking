package v2

import (
	"net/http"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type Server struct {
	*restserver.HTTPRestService
}

func New(s *restserver.HTTPRestService) *Server {
	return &Server{s}
}

func (s Server) Start(log *zap.Logger, addr string) {
	e := echo.New()
	e.HideBanner = true
	e.GET(cns.RequestIPConfig, echo.WrapHandler(restserver.NewHandlerFuncWithHistogram(s.RequestIPConfigHandler, restserver.HttpRequestLatency)))
	e.GET(cns.RequestIPConfigs, echo.WrapHandler(restserver.NewHandlerFuncWithHistogram(s.RequestIPConfigsHandler, restserver.HttpRequestLatency)))
	e.GET(cns.ReleaseIPConfig, echo.WrapHandler(restserver.NewHandlerFuncWithHistogram(s.ReleaseIPConfigHandler, restserver.HttpRequestLatency)))
	e.GET(cns.ReleaseIPConfigs, echo.WrapHandler(restserver.NewHandlerFuncWithHistogram(s.ReleaseIPConfigsHandler, restserver.HttpRequestLatency)))
	e.GET(cns.PathDebugIPAddresses, echo.WrapHandler(http.HandlerFunc(s.HandleDebugIPAddresses)))
	e.GET(cns.PathDebugPodContext, echo.WrapHandler(http.HandlerFunc(s.HandleDebugPodContext)))
	e.GET(cns.PathDebugRestData, echo.WrapHandler(http.HandlerFunc(s.HandleDebugRestData)))
	e.GET(cns.EndpointAPI, echo.WrapHandler(http.HandlerFunc(s.EndpointHandlerAPI)))
	e.GET(cns.NMAgentSupportedAPIs, echo.WrapHandler(http.HandlerFunc(s.NmAgentSupportedApisHandler)))
	e.GET(cns.GetNetworkContainerByOrchestratorContext, echo.WrapHandler(http.HandlerFunc(s.GetNetworkContainerByOrchestratorContext)))
	e.GET(cns.GetAllNetworkContainers, echo.WrapHandler(http.HandlerFunc(s.GetAllNetworkContainers)))
	e.GET(cns.CreateHostNCApipaEndpointPath, echo.WrapHandler(http.HandlerFunc(s.CreateHostNCApipaEndpoint)))
	e.GET(cns.DeleteHostNCApipaEndpointPath, echo.WrapHandler(http.HandlerFunc(s.DeleteHostNCApipaEndpoint)))
	e.GET(cns.PublishNetworkContainer, echo.WrapHandler(http.HandlerFunc(s.PublishNetworkContainer)))
	e.GET(cns.UnpublishNetworkContainer, echo.WrapHandler(http.HandlerFunc(s.UnpublishNetworkContainer)))
	e.GET(cns.CreateOrUpdateNetworkContainer, echo.WrapHandler(http.HandlerFunc(s.CreateOrUpdateNetworkContainer)))
	e.GET(cns.SetOrchestratorType, echo.WrapHandler(http.HandlerFunc(s.SetOrchestratorType)))
	e.GET(cns.DeleteNetworkContainer, echo.WrapHandler(http.HandlerFunc(s.DeleteNetworkContainer)))
	e.GET(cns.NumberOfCPUCores, echo.WrapHandler(http.HandlerFunc(s.GetNumberOfCPUCores)))
	e.GET(cns.NetworkContainersURLPath, echo.WrapHandler(http.HandlerFunc(s.GetOrRefreshNetworkContainers)))
	e.GET(cns.GetHomeAz, echo.WrapHandler(http.HandlerFunc(s.GetHomeAz)))

	// for handlers 2.0
	e.GET(cns.V2Prefix+cns.NMAgentSupportedAPIs, echo.WrapHandler(http.HandlerFunc(s.NmAgentSupportedApisHandler)))
	e.GET(cns.V2Prefix+cns.GetNetworkContainerByOrchestratorContext, echo.WrapHandler(http.HandlerFunc(s.GetNetworkContainerByOrchestratorContext)))
	e.GET(cns.V2Prefix+cns.GetAllNetworkContainers, echo.WrapHandler(http.HandlerFunc(s.GetAllNetworkContainers)))
	e.GET(cns.V2Prefix+cns.CreateHostNCApipaEndpointPath, echo.WrapHandler(http.HandlerFunc(s.CreateHostNCApipaEndpoint)))
	e.GET(cns.V2Prefix+cns.DeleteHostNCApipaEndpointPath, echo.WrapHandler(http.HandlerFunc(s.DeleteHostNCApipaEndpoint)))
	e.GET(cns.V2Prefix+cns.EndpointAPI, echo.WrapHandler(http.HandlerFunc(s.EndpointHandlerAPI)))
	e.GET(cns.V2Prefix+cns.CreateOrUpdateNetworkContainer, echo.WrapHandler(http.HandlerFunc(s.CreateOrUpdateNetworkContainer)))
	e.GET(cns.V2Prefix+cns.SetOrchestratorType, echo.WrapHandler(http.HandlerFunc(s.SetOrchestratorType)))
	e.GET(cns.V2Prefix+cns.DeleteNetworkContainer, echo.WrapHandler(http.HandlerFunc(s.DeleteNetworkContainer)))
	e.GET(cns.V2Prefix+cns.NumberOfCPUCores, echo.WrapHandler(http.HandlerFunc(s.GetNumberOfCPUCores)))
	e.GET(cns.V2Prefix+cns.GetHomeAz, echo.WrapHandler(http.HandlerFunc(s.GetHomeAz)))

	if err := e.Start(addr); err != nil {
		log.Error("failed to run server", zap.Error(err))
	}
}
