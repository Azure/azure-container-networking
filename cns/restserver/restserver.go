package restserver

import (
	"context"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/dockerclient"
	"github.com/Azure/azure-container-networking/cns/ipamclient"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/networkcontainers"
	"github.com/Azure/azure-container-networking/cns/nmagent"
	"github.com/Azure/azure-container-networking/cns/routes"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/cns/types/bounded"
	"github.com/Azure/azure-container-networking/cns/wireserver"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/store"
	"github.com/pkg/errors"
)

// This file contains the initialization of RestServer.
// all HTTP APIs - api.go and/or ipam.go
// APIs for internal consumption - internalapi.go
// All helper/utility functions - util.go
// Constants - const.go

var (
	// Named Lock for accessing different states in httpRestServiceState
	namedLock = acn.InitNamedLock()
	// map of NC to their respective NMA getVersion URLs
	ncVersionURLs sync.Map
)

type interfaceGetter interface {
	GetInterfaces(ctx context.Context) (*wireserver.GetInterfacesResult, error)
}

type nmagentClient interface {
	GetNCVersionList(ctx context.Context) (*nmagent.NetworkContainerListResponse, error)
}

// HTTPRestService represents http listener for CNS - Container Networking Service.
type HTTPRestService struct {
	*cns.Service
	dockerClient             *dockerclient.Client
	wscli                    interfaceGetter
	ipamClient               *ipamclient.IpamClient
	nmagentClient            nmagentClient
	networkContainer         *networkcontainers.NetworkContainers
	PodIPIDByPodInterfaceKey map[string]string                    // PodInterfaceId is key and value is Pod IP (SecondaryIP) uuid.
	PodIPConfigState         map[string]cns.IPConfigurationStatus // Secondary IP ID(uuid) is key
	IPAMPoolMonitor          cns.IPAMPoolMonitor
	routingTable             *routes.RoutingTable
	store                    store.KeyValueStore
	state                    *httpRestServiceState
	podsPendingIPAssignment  *bounded.TimedSet
	sync.RWMutex
	dncPartitionKey string
}

type GetHTTPServiceDataResponse struct {
	HTTPRestServiceData HTTPRestServiceData
	Response            Response
}

// HTTPRestServiceData represents in-memory CNS data in the debug API paths.
type HTTPRestServiceData struct {
	PodIPIDByPodInterfaceKey map[string]string                    // PodInterfaceId is key and value is Pod IP uuid.
	PodIPConfigState         map[string]cns.IPConfigurationStatus // secondaryipid(uuid) is key
	IPAMPoolMonitor          cns.IpamPoolMonitorStateSnapshot
}

type Response struct {
	ReturnCode types.ResponseCode
	Message    string
}

// containerstatus is used to save status of an existing container
type containerstatus struct {
	ID                            string
	VMVersion                     string
	HostVersion                   string
	CreateNetworkContainerRequest cns.CreateNetworkContainerRequest
	VfpUpdateComplete             bool // True when VFP programming is completed for the NC
}

// httpRestServiceState contains the state we would like to persist.
type httpRestServiceState struct {
	Location                         string
	NetworkType                      string
	OrchestratorType                 string
	NodeID                           string
	Initialized                      bool
	ContainerIDByOrchestratorContext map[string]string          // OrchestratorContext is key and value is NetworkContainerID.
	ContainerStatus                  map[string]containerstatus // NetworkContainerID is key.
	Networks                         map[string]*networkInfo
	TimeStamp                        time.Time
	joinedNetworks                   map[string]struct{}
	primaryInterface                 *wireserver.InterfaceInfo
}

type networkInfo struct {
	NetworkName string
	NicInfo     *wireserver.InterfaceInfo
	Options     map[string]interface{}
}

// NewHTTPRestService creates a new HTTP Service object.
func NewHTTPRestService(config *common.ServiceConfig, wscli interfaceGetter, nmagentClient nmagentClient) (cns.HTTPService, error) {
	service, err := cns.NewService(config.Name, config.Version, config.ChannelMode, config.Store)
	if err != nil {
		return nil, err
	}

	routingTable := &routes.RoutingTable{}
	nc := &networkcontainers.NetworkContainers{}
	dc, err := dockerclient.NewDefaultClient(wscli)
	if err != nil {
		return nil, err
	}

	ic, err := ipamclient.NewIpamClient("")
	if err != nil {
		return nil, err
	}

	res, err := wscli.GetInterfaces(context.TODO()) // TODO(rbtr): thread context through this client
	if err != nil {
		return nil, errors.Wrap(err, "failed to get interfaces from IMDS")
	}
	primaryInterface, err := wireserver.GetPrimaryInterfaceFromResult(res)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get primary interface from IMDS response")
	}

	serviceState := &httpRestServiceState{
		Networks:         make(map[string]*networkInfo),
		joinedNetworks:   make(map[string]struct{}),
		primaryInterface: primaryInterface,
	}

	podIPIDByPodInterfaceKey := make(map[string]string)
	podIPConfigState := make(map[string]cns.IPConfigurationStatus)

	return &HTTPRestService{
		Service:                  service,
		store:                    service.Service.Store,
		dockerClient:             dc,
		wscli:                    wscli,
		ipamClient:               ic,
		nmagentClient:            nmagentClient,
		networkContainer:         nc,
		PodIPIDByPodInterfaceKey: podIPIDByPodInterfaceKey,
		PodIPConfigState:         podIPConfigState,
		routingTable:             routingTable,
		state:                    serviceState,
		podsPendingIPAssignment:  bounded.NewTimedSet(250), // nolint:gomnd // maxpods
	}, nil
}

// Init starts the CNS listener.
func (service *HTTPRestService) Init(config *common.ServiceConfig) error {
	err := service.Initialize(config)
	if err != nil {
		logger.Errorf("[Azure CNS]  Failed to initialize base service, err:%v.", err)
		return err
	}

	service.restoreState()
	err = service.restoreNetworkState()
	if err != nil {
		logger.Errorf("[Azure CNS]  Failed to restore network state, err:%v.", err)
		return err
	}

	// Add handlers.
	listener := service.Listener
	// default handlers
	listener.AddHandler(cns.SetEnvironmentPath, service.setEnvironment)
	listener.AddHandler(cns.CreateNetworkPath, service.createNetwork)
	listener.AddHandler(cns.DeleteNetworkPath, service.deleteNetwork)
	listener.AddHandler(cns.ReserveIPAddressPath, service.reserveIPAddress)
	listener.AddHandler(cns.ReleaseIPAddressPath, service.releaseIPAddress)
	listener.AddHandler(cns.GetHostLocalIPPath, service.getHostLocalIP)
	listener.AddHandler(cns.GetIPAddressUtilizationPath, service.getIPAddressUtilization)
	listener.AddHandler(cns.GetUnhealthyIPAddressesPath, service.getUnhealthyIPAddresses)
	listener.AddHandler(cns.CreateOrUpdateNetworkContainer, service.createOrUpdateNetworkContainer)
	listener.AddHandler(cns.DeleteNetworkContainer, service.deleteNetworkContainer)
	listener.AddHandler(cns.GetInterfaceForContainer, service.getInterfaceForContainer)
	listener.AddHandler(cns.SetOrchestratorType, service.setOrchestratorType)
	listener.AddHandler(cns.GetNetworkContainerByOrchestratorContext, service.getNetworkContainerByOrchestratorContext)
	listener.AddHandler(cns.AttachContainerToNetwork, service.attachNetworkContainerToNetwork)
	listener.AddHandler(cns.DetachContainerFromNetwork, service.detachNetworkContainerFromNetwork)
	listener.AddHandler(cns.CreateHnsNetworkPath, service.createHnsNetwork)
	listener.AddHandler(cns.DeleteHnsNetworkPath, service.deleteHnsNetwork)
	listener.AddHandler(cns.NumberOfCPUCoresPath, service.getNumberOfCPUCores)
	listener.AddHandler(cns.CreateHostNCApipaEndpointPath, service.createHostNCApipaEndpoint)
	listener.AddHandler(cns.DeleteHostNCApipaEndpointPath, service.deleteHostNCApipaEndpoint)
	listener.AddHandler(cns.PublishNetworkContainer, service.publishNetworkContainer)
	listener.AddHandler(cns.UnpublishNetworkContainer, service.unpublishNetworkContainer)
	listener.AddHandler(cns.RequestIPConfig, newHandlerFuncWithHistogram(service.requestIPConfigHandler, httpRequestLatency))
	listener.AddHandler(cns.ReleaseIPConfig, newHandlerFuncWithHistogram(service.releaseIPConfigHandler, httpRequestLatency))
	listener.AddHandler(cns.NmAgentSupportedApisPath, service.nmAgentSupportedApisHandler)
	listener.AddHandler(cns.PathDebugIPAddresses, service.handleDebugIPAddresses)
	listener.AddHandler(cns.PathDebugPodContext, service.handleDebugPodContext)
	listener.AddHandler(cns.PathDebugRestData, service.handleDebugRestData)

	// handlers for v0.2
	listener.AddHandler(cns.V2Prefix+cns.SetEnvironmentPath, service.setEnvironment)
	listener.AddHandler(cns.V2Prefix+cns.CreateNetworkPath, service.createNetwork)
	listener.AddHandler(cns.V2Prefix+cns.DeleteNetworkPath, service.deleteNetwork)
	listener.AddHandler(cns.V2Prefix+cns.ReserveIPAddressPath, service.reserveIPAddress)
	listener.AddHandler(cns.V2Prefix+cns.ReleaseIPAddressPath, service.releaseIPAddress)
	listener.AddHandler(cns.V2Prefix+cns.GetHostLocalIPPath, service.getHostLocalIP)
	listener.AddHandler(cns.V2Prefix+cns.GetIPAddressUtilizationPath, service.getIPAddressUtilization)
	listener.AddHandler(cns.V2Prefix+cns.GetUnhealthyIPAddressesPath, service.getUnhealthyIPAddresses)
	listener.AddHandler(cns.V2Prefix+cns.CreateOrUpdateNetworkContainer, service.createOrUpdateNetworkContainer)
	listener.AddHandler(cns.V2Prefix+cns.DeleteNetworkContainer, service.deleteNetworkContainer)
	listener.AddHandler(cns.V2Prefix+cns.GetInterfaceForContainer, service.getInterfaceForContainer)
	listener.AddHandler(cns.V2Prefix+cns.SetOrchestratorType, service.setOrchestratorType)
	listener.AddHandler(cns.V2Prefix+cns.GetNetworkContainerByOrchestratorContext, service.getNetworkContainerByOrchestratorContext)
	listener.AddHandler(cns.V2Prefix+cns.AttachContainerToNetwork, service.attachNetworkContainerToNetwork)
	listener.AddHandler(cns.V2Prefix+cns.DetachContainerFromNetwork, service.detachNetworkContainerFromNetwork)
	listener.AddHandler(cns.V2Prefix+cns.CreateHnsNetworkPath, service.createHnsNetwork)
	listener.AddHandler(cns.V2Prefix+cns.DeleteHnsNetworkPath, service.deleteHnsNetwork)
	listener.AddHandler(cns.V2Prefix+cns.NumberOfCPUCoresPath, service.getNumberOfCPUCores)
	listener.AddHandler(cns.V2Prefix+cns.CreateHostNCApipaEndpointPath, service.createHostNCApipaEndpoint)
	listener.AddHandler(cns.V2Prefix+cns.DeleteHostNCApipaEndpointPath, service.deleteHostNCApipaEndpoint)
	listener.AddHandler(cns.V2Prefix+cns.NmAgentSupportedApisPath, service.nmAgentSupportedApisHandler)

	// Initialize HTTP client to be reused in CNS
	connectionTimeout, _ := service.GetOption(acn.OptHttpConnectionTimeout).(int)
	responseHeaderTimeout, _ := service.GetOption(acn.OptHttpResponseHeaderTimeout).(int)
	acn.InitHttpClient(connectionTimeout, responseHeaderTimeout)

	logger.SetContextDetails(service.state.OrchestratorType, service.state.NodeID)
	logger.Printf("[Azure CNS]  Listening.")

	return nil
}

// Start starts the CNS listener.
func (service *HTTPRestService) Start(config *common.ServiceConfig) error {
	// Start the listener.
	// continue to listen on the normal endpoint for http traffic, this will be supported
	// for sometime until partners migrate fully to https
	if err := service.StartListener(config); err != nil {
		return err
	}

	return nil
}

// Stop stops the CNS.
func (service *HTTPRestService) Stop() {
	service.Uninitialize()
	logger.Printf("[Azure CNS]  Service stopped.")
}
