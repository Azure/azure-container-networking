package restserver

import (
	"context"
	"net"
	"net/http"
	"net/http/pprof"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/dockerclient"
	"github.com/Azure/azure-container-networking/cns/imds"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/networkcontainers"
	"github.com/Azure/azure-container-networking/cns/nodesubnet"
	"github.com/Azure/azure-container-networking/cns/routes"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/cns/types/bounded"
	"github.com/Azure/azure-container-networking/cns/wireserver"
	acn "github.com/Azure/azure-container-networking/common"
	nma "github.com/Azure/azure-container-networking/nmagent"
	"github.com/Azure/azure-container-networking/store"
	"github.com/pkg/errors"
)

// This file contains the initialization of RestServer.
// all HTTP APIs - api.go and/or ipam.go
// APIs for internal consumption - internalapi.go
// All helper/utility functions - util.go
// Constants - const.go

// Named Lock for accessing different states in httpRestServiceState
var namedLock = acn.InitNamedLock()

type interfaceGetter interface {
	GetInterfaces(ctx context.Context) (*wireserver.GetInterfacesResult, error)
}

type nmagentClient interface {
	SupportedAPIs(context.Context) ([]string, error)
	GetNCVersionList(context.Context) (nma.NCVersionList, error)
	GetHomeAz(context.Context) (nma.AzResponse, error)
	GetInterfaceIPInfo(ctx context.Context) (nma.Interfaces, error)
}

type wireserverProxy interface {
	JoinNetwork(ctx context.Context, vnetID string) (*http.Response, error)
	PublishNC(ctx context.Context, ncParams cns.NetworkContainerParameters, payload []byte) (*http.Response, error)
	UnpublishNC(ctx context.Context, ncParams cns.NetworkContainerParameters, payload []byte) (*http.Response, error)
}

type imdsClient interface {
	GetVMUniqueID(ctx context.Context) (string, error)
	GetNetworkInterfaces(ctx context.Context) ([]imds.NetworkInterface, error)
	GetIMDSVersions(ctx context.Context) (*imds.APIVersionsResponse, error)
}

type iptablesClient interface {
	ChainExists(table string, chain string) (bool, error)
	NewChain(table string, chain string) error
	Append(table string, chain string, rulespec ...string) error
	Exists(table string, chain string, rulespec ...string) (bool, error)
	Insert(table string, chain string, pos int, rulespec ...string) error
	List(table string, chain string) ([]string, error)
	ClearChain(table string, chain string) error
	Delete(table, chain string, rulespec ...string) error
}
type iptablesLegacyClient interface {
	Delete(table, chain string, rulespec ...string) error
}

type iptablesGetter interface {
	GetIPTables() (iptablesClient, error)
	GetIPTablesLegacy() (iptablesLegacyClient, error)
}

// HTTPRestService represents http listener for CNS - Container Networking Service.
type HTTPRestService struct {
	*cns.Service
	dockerClient             *dockerclient.Client
	wscli                    interfaceGetter
	iptables                 iptablesGetter
	nma                      nmagentClient
	wsproxy                  wireserverProxy
	homeAzMonitor            *HomeAzMonitor
	networkContainer         *networkcontainers.NetworkContainers
	PodIPIDByPodInterfaceKey map[string][]string                  // PodInterfaceId is key and value is slice of Pod IP (SecondaryIP) uuids.
	PodIPConfigState         map[string]cns.IPConfigurationStatus // Secondary IP ID(uuid) is key
	routingTable             *routes.RoutingTable
	store                    store.KeyValueStore
	state                    *httpRestServiceState
	podsPendingIPAssignment  *bounded.TimedSet
	sync.RWMutex
	dncPartitionKey            string
	EndpointState              map[string]*EndpointInfo // key : container id
	EndpointStateStore         store.KeyValueStore
	cniConflistGenerator       CNIConflistGenerator
	generateCNIConflistOnce    sync.Once
	IPConfigsHandlerMiddleware cns.IPConfigsHandlerMiddleware
	PnpIDByMacAddress          map[string]string
	imdsClient                 imdsClient
	nodesubnetIPFetcher        *nodesubnet.IPFetcher
}

type CNIConflistGenerator interface {
	Generate() error
	Close() error
}

type NoOpConflistGenerator struct{}

func (*NoOpConflistGenerator) Generate() error {
	return nil
}

func (*NoOpConflistGenerator) Close() error {
	return nil
}

type EndpointInfo struct {
	PodName       string
	PodNamespace  string
	IfnameToIPMap map[string]*IPInfo // key : interface name, value : IPInfo
}

type IPInfo struct {
	IPv4               []net.IPNet
	IPv6               []net.IPNet `json:",omitempty"`
	HnsEndpointID      string      `json:",omitempty"`
	HnsNetworkID       string      `json:",omitempty"`
	HostVethName       string      `json:",omitempty"`
	MacAddress         string      `json:",omitempty"`
	NetworkContainerID string      `json:",omitempty"`
	NICType            cns.NICType
}

type GetHTTPServiceDataResponse struct {
	HTTPRestServiceData HTTPRestServiceData `json:"HTTPRestServiceData"`
	Response            Response            `json:"Response"`
}

// HTTPRestServiceData represents in-memory CNS data in the debug API paths.
// TODO: add json tags for this struct as per linter suggestion, ignored for now as part of revert-PR
type HTTPRestServiceData struct { //nolint:musttag // not tagging struct for revert-PR
	PodIPIDByPodInterfaceKey map[string][]string                  // PodInterfaceId is key and value is slice of Pod IP uuids.
	PodIPConfigState         map[string]cns.IPConfigurationStatus // secondaryipid(uuid) is key
}

type Response struct {
	ReturnCode types.ResponseCode
	Message    string
}

// GetEndpointResponse describes response from the The GetEndpoint API.
type GetEndpointResponse struct {
	Response     Response     `json:"response"`
	EndpointInfo EndpointInfo `json:"endpointInfo"`
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
	ContainerIDByOrchestratorContext map[string]*ncList         // OrchestratorContext is the key and value is a list of NetworkContainerIDs separated by comma
	ContainerStatus                  map[string]containerstatus // NetworkContainerID is key.
	Networks                         map[string]*networkInfo
	TimeStamp                        time.Time
	joinedNetworks                   map[string]struct{}
	primaryInterface                 *wireserver.InterfaceInfo
	PnpIDByMacAddress                map[string]string
}

type networkInfo struct {
	NetworkName string
	NicInfo     *wireserver.InterfaceInfo
	Options     map[string]interface{}
}

// NewHTTPRestService creates a new HTTP Service object.
func NewHTTPRestService(config *common.ServiceConfig, wscli interfaceGetter, wsproxy wireserverProxy, iptg iptablesGetter, nmagentClient nmagentClient,
	endpointStateStore store.KeyValueStore, gen CNIConflistGenerator, homeAzMonitor *HomeAzMonitor,
	imdsClient imdsClient,
) (*HTTPRestService, error) {
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

	res, err := wscli.GetInterfaces(context.TODO()) // TODO(rbtr): thread context through this client
	if err != nil {
		return nil, errors.Wrap(err, "failed to get interfaces from IMDS")
	}
	primaryInterface, err := wireserver.GetPrimaryInterfaceFromResult(res)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get primary interface from IMDS response")
	}

	// add primaryInterfaceIP to cns config
	config.Server.PrimaryInterfaceIP = primaryInterface.PrimaryIP

	serviceState := &httpRestServiceState{
		Networks:          make(map[string]*networkInfo),
		joinedNetworks:    make(map[string]struct{}),
		primaryInterface:  primaryInterface,
		PnpIDByMacAddress: make(map[string]string),
	}

	podIPIDByPodInterfaceKey := make(map[string][]string)
	podIPConfigState := make(map[string]cns.IPConfigurationStatus)

	if gen == nil {
		gen = &NoOpConflistGenerator{}
	}

	return &HTTPRestService{
		Service:                  service,
		store:                    service.Service.Store,
		dockerClient:             dc,
		wscli:                    wscli,
		iptables:                 iptg,
		nma:                      nmagentClient,
		wsproxy:                  wsproxy,
		networkContainer:         nc,
		PodIPIDByPodInterfaceKey: podIPIDByPodInterfaceKey,
		PodIPConfigState:         podIPConfigState,
		routingTable:             routingTable,
		state:                    serviceState,
		podsPendingIPAssignment:  bounded.NewTimedSet(250), // nolint:gomnd // maxpods
		EndpointStateStore:       endpointStateStore,
		EndpointState:            make(map[string]*EndpointInfo),
		homeAzMonitor:            homeAzMonitor,
		cniConflistGenerator:     gen,
		imdsClient:               imdsClient,
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
	listener.AddHandler(cns.GetHostLocalIPPath, service.getHostLocalIP)
	listener.AddHandler(cns.CreateOrUpdateNetworkContainer, service.createOrUpdateNetworkContainer)
	listener.AddHandler(cns.DeleteNetworkContainer, service.deleteNetworkContainer)
	listener.AddHandler(cns.GetInterfaceForContainer, service.getInterfaceForContainer)
	listener.AddHandler(cns.SetOrchestratorType, service.setOrchestratorType)
	listener.AddHandler(cns.GetNetworkContainerByOrchestratorContext, service.GetNetworkContainerByOrchestratorContext)
	listener.AddHandler(cns.GetAllNetworkContainers, service.GetAllNetworkContainers)
	listener.AddHandler(cns.AttachContainerToNetwork, service.attachNetworkContainerToNetwork)
	listener.AddHandler(cns.DetachContainerFromNetwork, service.detachNetworkContainerFromNetwork)
	listener.AddHandler(cns.CreateHnsNetworkPath, service.createHnsNetwork)
	listener.AddHandler(cns.DeleteHnsNetworkPath, service.deleteHnsNetwork)
	listener.AddHandler(cns.NumberOfCPUCoresPath, service.getNumberOfCPUCores)
	listener.AddHandler(cns.CreateHostNCApipaEndpointPath, service.CreateHostNCApipaEndpoint)
	listener.AddHandler(cns.DeleteHostNCApipaEndpointPath, service.DeleteHostNCApipaEndpoint)
	listener.AddHandler(cns.PublishNetworkContainer, service.publishNetworkContainer)
	listener.AddHandler(cns.UnpublishNetworkContainer, service.unpublishNetworkContainer)
	listener.AddHandler(cns.RequestIPConfig, NewHandlerFuncWithHistogram(service.RequestIPConfigHandler, HTTPRequestLatency))
	listener.AddHandler(cns.RequestIPConfigs, NewHandlerFuncWithHistogram(service.RequestIPConfigsHandler, HTTPRequestLatency))
	listener.AddHandler(cns.ReleaseIPConfig, NewHandlerFuncWithHistogram(service.ReleaseIPConfigHandler, HTTPRequestLatency))
	listener.AddHandler(cns.ReleaseIPConfigs, NewHandlerFuncWithHistogram(service.ReleaseIPConfigsHandler, HTTPRequestLatency))
	listener.AddHandler(cns.NmAgentSupportedApisPath, service.nmAgentSupportedApisHandler)
	listener.AddHandler(cns.PathDebugIPAddresses, service.HandleDebugIPAddresses)
	listener.AddHandler(cns.PathDebugPodContext, service.HandleDebugPodContext)
	listener.AddHandler(cns.PathDebugRestData, service.HandleDebugRestData)
	listener.AddHandler(cns.NetworkContainersURLPath, service.getOrRefreshNetworkContainers)
	listener.AddHandler(cns.GetHomeAz, service.getHomeAz)
	listener.AddHandler(cns.EndpointPath, service.EndpointHandlerAPI)
	// This API is only needed for Direct channel mode.
	if config.ChannelMode == cns.Direct {
		listener.AddHandler(cns.GetVMUniqueID, service.getVMUniqueID)
		listener.AddHandler(cns.GetNCList, service.nmAgentNCListHandler)
	}

	// handlers for v0.2
	listener.AddHandler(cns.V2Prefix+cns.SetEnvironmentPath, service.setEnvironment)
	listener.AddHandler(cns.V2Prefix+cns.CreateNetworkPath, service.createNetwork)
	listener.AddHandler(cns.V2Prefix+cns.DeleteNetworkPath, service.deleteNetwork)
	listener.AddHandler(cns.V2Prefix+cns.GetHostLocalIPPath, service.getHostLocalIP)
	listener.AddHandler(cns.V2Prefix+cns.CreateOrUpdateNetworkContainer, service.createOrUpdateNetworkContainer)
	listener.AddHandler(cns.V2Prefix+cns.DeleteNetworkContainer, service.deleteNetworkContainer)
	listener.AddHandler(cns.V2Prefix+cns.GetInterfaceForContainer, service.getInterfaceForContainer)
	listener.AddHandler(cns.V2Prefix+cns.SetOrchestratorType, service.setOrchestratorType)
	listener.AddHandler(cns.V2Prefix+cns.GetNetworkContainerByOrchestratorContext, service.GetNetworkContainerByOrchestratorContext)
	listener.AddHandler(cns.V2Prefix+cns.GetAllNetworkContainers, service.GetAllNetworkContainers)
	listener.AddHandler(cns.V2Prefix+cns.AttachContainerToNetwork, service.attachNetworkContainerToNetwork)
	listener.AddHandler(cns.V2Prefix+cns.DetachContainerFromNetwork, service.detachNetworkContainerFromNetwork)
	listener.AddHandler(cns.V2Prefix+cns.CreateHnsNetworkPath, service.createHnsNetwork)
	listener.AddHandler(cns.V2Prefix+cns.DeleteHnsNetworkPath, service.deleteHnsNetwork)
	listener.AddHandler(cns.V2Prefix+cns.NumberOfCPUCoresPath, service.getNumberOfCPUCores)
	listener.AddHandler(cns.V2Prefix+cns.CreateHostNCApipaEndpointPath, service.CreateHostNCApipaEndpoint)
	listener.AddHandler(cns.V2Prefix+cns.DeleteHostNCApipaEndpointPath, service.DeleteHostNCApipaEndpoint)
	listener.AddHandler(cns.V2Prefix+cns.NmAgentSupportedApisPath, service.nmAgentSupportedApisHandler)
	listener.AddHandler(cns.V2Prefix+cns.GetHomeAz, service.getHomeAz)
	listener.AddHandler(cns.V2Prefix+cns.EndpointPath, service.EndpointHandlerAPI)
	// This API is only needed for Direct channel mode.
	if config.ChannelMode == cns.Direct {
		listener.AddHandler(cns.V2Prefix+cns.GetVMUniqueID, service.getVMUniqueID)
		listener.AddHandler(cns.V2Prefix+cns.GetNCList, service.nmAgentNCListHandler)
	}

	// Initialize HTTP client to be reused in CNS
	connectionTimeout, _ := service.GetOption(acn.OptHttpConnectionTimeout).(int)
	responseHeaderTimeout, _ := service.GetOption(acn.OptHttpResponseHeaderTimeout).(int)
	acn.InitHttpClient(connectionTimeout, responseHeaderTimeout)

	logger.SetContextDetails(service.state.OrchestratorType, service.state.NodeID)
	logger.Printf("[Azure CNS]  Listening.")

	return nil
}

func (service *HTTPRestService) RegisterPProfEndpoints() {
	if service.Listener != nil {
		mux := service.Listener.GetMux()
		mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
		mux.Handle("/debug/pprof/block", pprof.Handler("block"))
		mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
		mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
		mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
		mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}
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

// MustGenerateCNIConflistOnce will generate the CNI conflist once if the service was initialized with
// a conflist generator. If not, this is a no-op.
func (service *HTTPRestService) MustGenerateCNIConflistOnce() {
	service.generateCNIConflistOnce.Do(func() {
		if err := service.cniConflistGenerator.Generate(); err != nil {
			panic("unable to generate cni conflist with error: " + err.Error())
		}

		if err := service.cniConflistGenerator.Close(); err != nil {
			panic("unable to close the cni conflist output stream: " + err.Error())
		}
	})
}

func (service *HTTPRestService) AttachIPConfigsHandlerMiddleware(middleware cns.IPConfigsHandlerMiddleware) {
	service.IPConfigsHandlerMiddleware = middleware
}
