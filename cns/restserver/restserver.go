// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"fmt"
	"net/http"
	"time"

	"net"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/dockerclient"
	"github.com/Azure/azure-container-networking/cns/imdsclient"
	"github.com/Azure/azure-container-networking/cns/ipamclient"
	"github.com/Azure/azure-container-networking/cns/routes"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/store"
)

const (
	// Key against which CNS state is persisted.
	storeKey = "ContainerNetworkService"
)

// httpRestService represents http listener for CNS - Container Networking Service.
type httpRestService struct {
	*cns.Service
	dockerClient *dockerclient.DockerClient
	imdsClient   *imdsclient.ImdsClient
	ipamClient   *ipamclient.IpamClient
	routingTable *routes.RoutingTable
	store        store.KeyValueStore
	state        httpRestServiceState
}

// httpRestServiceState contains the state we would like to persist.
type httpRestServiceState struct {
	Location    string
	NetworkType string
	Initialized bool
	TimeStamp   time.Time
}

// HTTPService describes the min API interface that every service should have.
type HTTPService interface {
	common.ServiceAPI
}

// NewHTTPRestService creates a new HTTP Service object.
func NewHTTPRestService(config *common.ServiceConfig) (HTTPService, error) {
	service, err := cns.NewService(config.Name, config.Version, config.Store)
	if err != nil {
		return nil, err
	}

	imdsClient := &imdsclient.ImdsClient{}
	routingTable := &routes.RoutingTable{}
	dc, err := dockerclient.NewDefaultDockerClient(imdsClient)
	if err != nil {
		return nil, err
	}

	ic, err := ipamclient.NewIpamClient("")
	if err != nil {
		return nil, err
	}

	return &httpRestService{
		Service:      service,
		store:        service.Service.Store,
		dockerClient: dc,
		imdsClient:   imdsClient,
		ipamClient:   ic,
		routingTable: routingTable,
	}, nil
}

// Start starts the CNS listener.
func (service *httpRestService) Start(config *common.ServiceConfig) error {

	err := service.Initialize(config)
	if err != nil {
		log.Printf("[Azure CNS]  Failed to initialize base service, err:%v.", err)
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

	// handlers for v0.1
	listener.AddHandler(cns.V1Prefix+cns.SetEnvironmentPath, service.setEnvironment)
	listener.AddHandler(cns.V1Prefix+cns.CreateNetworkPath, service.createNetwork)
	listener.AddHandler(cns.V1Prefix+cns.DeleteNetworkPath, service.deleteNetwork)
	listener.AddHandler(cns.V1Prefix+cns.ReserveIPAddressPath, service.reserveIPAddress)
	listener.AddHandler(cns.V1Prefix+cns.ReleaseIPAddressPath, service.releaseIPAddress)
	listener.AddHandler(cns.V1Prefix+cns.GetHostLocalIPPath, service.getHostLocalIP)
	listener.AddHandler(cns.V1Prefix+cns.GetIPAddressUtilizationPath, service.getIPAddressUtilization)
	listener.AddHandler(cns.V1Prefix+cns.GetUnhealthyIPAddressesPath, service.getUnhealthyIPAddresses)

	log.Printf("[Azure CNS]  Listening.")
	return nil
}

// Stop stops the CNS.
func (service *httpRestService) Stop() {
	service.Uninitialize()
	log.Printf("[Azure CNS]  Service stopped.")
}

// Handles requests to set the environment type.
func (service *httpRestService) setEnvironment(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Azure CNS] setEnvironment")

	var req cns.SetEnvironmentRequest
	err := service.Listener.Decode(w, r, &req)
	log.Request(service.Name, &req, err)

	if err != nil {
		return
	}

	switch r.Method {
	case "POST":
		log.Printf("[Azure CNS]  POST received for SetEnvironment.")
		service.state.Location = req.Location
		service.state.NetworkType = req.NetworkType
		service.state.Initialized = true
		service.saveState()
	default:
	}

	resp := &cns.Response{ReturnCode: 0}
	err = service.Listener.Encode(w, &resp)

	log.Response(service.Name, resp, err)
}

// Handles CreateNetwork requests.
func (service *httpRestService) createNetwork(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Azure CNS] createNetwork")

	var err error
	returnCode := 0
	returnMessage := ""

	if service.state.Initialized {
		var req cns.CreateNetworkRequest
		err = service.Listener.Decode(w, r, &req)
		log.Request(service.Name, &req, err)

		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. Unable to decode input request.")
			returnCode = InvalidParameter
		} else {
			switch r.Method {
			case "POST":
				dc := service.dockerClient
				rt := service.routingTable
				err = dc.NetworkExists(req.NetworkName)

				// Network does not exist.
				if err != nil {
					switch service.state.NetworkType {
					case "Underlay":
						switch service.state.Location {
						case "Azure":
							log.Printf("[Azure CNS] Goign to create network with name %v.", req.NetworkName)

							err = rt.GetRoutingTable()
							if err != nil {
								// We should not fail the call to create network for this.
								// This is because restoring routes is a fallback mechanism in case
								// network driver is not behaving as expected.
								// The responsibility to restore routes is with network driver.
								log.Printf("[Azure CNS] Unable to get routing table from node, %+v.", err.Error())
							}

							err = dc.CreateNetwork(req.NetworkName)
							if err != nil {
								returnMessage = fmt.Sprintf("[Azure CNS] Error. CreateNetwork failed %v.", err.Error())
								returnCode = UnexpectedError
							}

							err = rt.RestoreRoutingTable()
							if err != nil {
								log.Printf("[Azure CNS] Unable to restore routing table on node, %+v.", err.Error())
							}

						case "StandAlone":
							returnMessage = fmt.Sprintf("[Azure CNS] Error. Underlay network is not supported in StandAlone environment. %v.", err.Error())
							returnCode = UnsupportedEnvironment
						}
					case "Overlay":
						returnMessage = fmt.Sprintf("[Azure CNS] Error. Overlay support not yet available. %v.", err.Error())
						returnCode = UnsupportedEnvironment
					}
				} else {
					returnMessage = fmt.Sprintf("[Azure CNS] Received a request to create an already existing network %v", req.NetworkName)
					log.Printf(returnMessage)
				}

			default:
				returnMessage = "[Azure CNS] Error. CreateNetwork did not receive a POST."
				returnCode = InvalidParameter
			}
		}

	} else {
		returnMessage = fmt.Sprintf("[Azure CNS] Error. CNS is not yet initialized with environment.")
		returnCode = UnsupportedEnvironment
	}

	resp := &cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}

	err = service.Listener.Encode(w, &resp)

	log.Response(service.Name, resp, err)
}

// Handles DeleteNetwork requests.
func (service *httpRestService) deleteNetwork(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Azure CNS] deleteNetwork")

	var req cns.DeleteNetworkRequest
	returnCode := 0
	returnMessage := ""
	err := service.Listener.Decode(w, r, &req)
	log.Request(service.Name, &req, err)

	if err != nil {
		return
	}

	switch r.Method {
	case "POST":
		dc := service.dockerClient
		err := dc.NetworkExists(req.NetworkName)

		// Network does exist
		if err == nil {
			log.Printf("[Azure CNS] Goign to delete network with name %v.", req.NetworkName)
			err := dc.DeleteNetwork(req.NetworkName)
			if err != nil {
				returnMessage = fmt.Sprintf("[Azure CNS] Error. DeleteNetwork failed %v.", err.Error())
				returnCode = UnexpectedError
			}
		} else {
			if err == fmt.Errorf("Network not found") {
				log.Printf("[Azure CNS] Received a request to delete network that does not exist: %v.", req.NetworkName)
			} else {
				returnCode = UnexpectedError
				returnMessage = err.Error()
			}
		}

	default:
		returnMessage = "[Azure CNS] Error. DeleteNetwork did not receive a POST."
		returnCode = InvalidParameter
	}

	resp := &cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}

	err = service.Listener.Encode(w, &resp)

	log.Response(service.Name, resp, err)
}

// Handles ip reservation requests.
func (service *httpRestService) reserveIPAddress(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Azure CNS] reserveIPAddress")

	var req cns.ReserveIPAddressRequest
	returnMessage := ""
	returnCode := 0
	addr := ""
	address := ""
	err := service.Listener.Decode(w, r, &req)

	log.Request(service.Name, &req, err)

	if err != nil {
		return
	}

	if req.ReservationID == "" {
		returnCode = ReservationNotFound
		returnMessage = fmt.Sprintf("[Azure CNS] Error. ReservationId is empty")
	}

	switch r.Method {
	case "POST":
		ic := service.ipamClient

		ifInfo, err := service.imdsClient.GetPrimaryInterfaceInfoFromMemory()
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetPrimaryIfaceInfo failed %v", err.Error())
			returnCode = UnexpectedError
			break
		}

		asID, err := ic.GetAddressSpace()
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetAddressSpace failed %v", err.Error())
			returnCode = UnexpectedError
			break
		}

		poolID, err := ic.GetPoolID(asID, ifInfo.Subnet)
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetPoolID failed %v", err.Error())
			returnCode = UnexpectedError
			break
		}

		addr, err = ic.ReserveIPAddress(poolID, req.ReservationID)
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] ReserveIpAddress failed with %+v", err.Error())
			returnCode = AddressUnavailable
			break
		}

		addressIP, _, err := net.ParseCIDR(addr)
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] ParseCIDR failed with %+v", err.Error())
			returnCode = UnexpectedError
			break
		}
		address = addressIP.String()

	default:
		returnMessage = "[Azure CNS] Error. ReserveIP did not receive a POST."
		returnCode = InvalidParameter

	}

	resp := cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}
	reserveResp := &cns.ReserveIPAddressResponse{Response: resp, IPAddress: address}
	err = service.Listener.Encode(w, &reserveResp)

	log.Response(service.Name, reserveResp, err)
}

// Handles release ip reservation requests.
func (service *httpRestService) releaseIPAddress(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Azure CNS] releaseIPAddress")

	var req cns.ReleaseIPAddressRequest
	returnMessage := ""
	returnCode := 0

	err := service.Listener.Decode(w, r, &req)
	log.Request(service.Name, &req, err)

	if err != nil {
		return
	}

	if req.ReservationID == "" {
		returnCode = ReservationNotFound
		returnMessage = fmt.Sprintf("[Azure CNS] Error. ReservationId is empty")
	}

	switch r.Method {
	case "POST":
		ic := service.ipamClient

		ifInfo, err := service.imdsClient.GetPrimaryInterfaceInfoFromMemory()
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetPrimaryIfaceInfo failed %v", err.Error())
			returnCode = UnexpectedError
			break
		}

		asID, err := ic.GetAddressSpace()
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetAddressSpace failed %v", err.Error())
			returnCode = UnexpectedError
			break
		}

		poolID, err := ic.GetPoolID(asID, ifInfo.Subnet)
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetPoolID failed %v", err.Error())
			returnCode = UnexpectedError
			break
		}

		err = ic.ReleaseIPAddress(poolID, req.ReservationID)
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] ReleaseIpAddress failed with %+v", err.Error())
			returnCode = ReservationNotFound
		}

	default:
		returnMessage = "[Azure CNS] Error. ReleaseIP did not receive a POST."
		returnCode = InvalidParameter
	}

	resp := cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}

	err = service.Listener.Encode(w, &resp)

	log.Response(service.Name, resp, err)
}

// Retrieves the host local ip address. Containers can talk to host using this IP address.
func (service *httpRestService) getHostLocalIP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Azure CNS] getHostLocalIP")
	log.Request(service.Name, "getHostLocalIP", nil)

	var found bool
	var errmsg string
	hostLocalIP := "0.0.0.0"

	if service.state.Initialized {
		switch r.Method {
		case "GET":
			switch service.state.NetworkType {
			case "Underlay":
				if service.imdsClient != nil {
					piface, err := service.imdsClient.GetPrimaryInterfaceInfoFromMemory()
					if err == nil {
						hostLocalIP = piface.PrimaryIP
						found = true
					} else {
						log.Printf("[Azure-CNS] Received error from GetPrimaryInterfaceInfoFromMemory. err: %v", err.Error())
					}
				}

			case "Overlay":
				errmsg = "[Azure-CNS] Overlay is not yet supported."
			}

		default:
			errmsg = "[Azure-CNS] GetHostLocalIP API expects a GET."
		}
	}

	returnCode := 0
	if !found {
		returnCode = NotFound
		if errmsg == "" {
			errmsg = "[Azure-CNS] Unable to get host local ip. Check if environment is initialized.."
		}
	}

	resp := cns.Response{ReturnCode: returnCode, Message: errmsg}
	hostLocalIPResponse := &cns.HostLocalIPAddressResponse{
		Response:  resp,
		IPAddress: hostLocalIP,
	}

	err := service.Listener.Encode(w, &hostLocalIPResponse)

	log.Response(service.Name, hostLocalIPResponse, err)
}

// Handles ip address utilization requests.
func (service *httpRestService) getIPAddressUtilization(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Azure CNS] getIPAddressUtilization")
	log.Request(service.Name, "getIPAddressUtilization", nil)

	returnMessage := ""
	returnCode := 0
	capacity := 0
	available := 0
	var unhealthyAddrs []string

	switch r.Method {
	case "GET":
		ic := service.ipamClient

		ifInfo, err := service.imdsClient.GetPrimaryInterfaceInfoFromMemory()
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetPrimaryIfaceInfo failed %v", err.Error())
			returnCode = UnexpectedError
			break
		}

		asID, err := ic.GetAddressSpace()
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetAddressSpace failed %v", err.Error())
			returnCode = UnexpectedError
			break
		}

		poolID, err := ic.GetPoolID(asID, ifInfo.Subnet)
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetPoolID failed %v", err.Error())
			returnCode = UnexpectedError
			break
		}

		capacity, available, unhealthyAddrs, err = ic.GetIPAddressUtilization(poolID)
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetIPUtilization failed %v", err.Error())
			returnCode = UnexpectedError
			break
		}
		log.Printf("[Azure CNS] Capacity %v Available %v UnhealthyAddrs %v", capacity, available, unhealthyAddrs)

	default:
		returnMessage = "[Azure CNS] Error. GetIPUtilization did not receive a GET."
		returnCode = InvalidParameter
	}

	resp := cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}

	utilResponse := &cns.IPAddressesUtilizationResponse{
		Response:  resp,
		Available: available,
		Reserved:  capacity - available,
		Unhealthy: len(unhealthyAddrs),
	}

	err := service.Listener.Encode(w, &utilResponse)

	log.Response(service.Name, utilResponse, err)
}

// Handles retrieval of ip addresses that are available to be reserved from ipam driver.
func (service *httpRestService) getAvailableIPAddresses(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Azure CNS] getAvailableIPAddresses")
	log.Request(service.Name, "getAvailableIPAddresses", nil)

	switch r.Method {
	case "GET":
	default:
	}

	resp := cns.Response{ReturnCode: 0}
	ipResp := &cns.GetIPAddressesResponse{Response: resp}
	err := service.Listener.Encode(w, &ipResp)

	log.Response(service.Name, ipResp, err)
}

// Handles retrieval of reserved ip addresses from ipam driver.
func (service *httpRestService) getReservedIPAddresses(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Azure CNS] getReservedIPAddresses")
	log.Request(service.Name, "getReservedIPAddresses", nil)

	switch r.Method {
	case "GET":
	default:
	}

	resp := cns.Response{ReturnCode: 0}
	ipResp := &cns.GetIPAddressesResponse{Response: resp}
	err := service.Listener.Encode(w, &ipResp)

	log.Response(service.Name, ipResp, err)
}

// Handles retrieval of ghost ip addresses from ipam driver.
func (service *httpRestService) getUnhealthyIPAddresses(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Azure CNS] getUnhealthyIPAddresses")
	log.Request(service.Name, "getUnhealthyIPAddresses", nil)

	returnMessage := ""
	returnCode := 0
	capacity := 0
	available := 0
	var unhealthyAddrs []string

	switch r.Method {
	case "GET":
		ic := service.ipamClient

		ifInfo, err := service.imdsClient.GetPrimaryInterfaceInfoFromMemory()
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetPrimaryIfaceInfo failed %v", err.Error())
			returnCode = UnexpectedError
			break
		}

		asID, err := ic.GetAddressSpace()
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetAddressSpace failed %v", err.Error())
			returnCode = UnexpectedError
			break
		}

		poolID, err := ic.GetPoolID(asID, ifInfo.Subnet)
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetPoolID failed %v", err.Error())
			returnCode = UnexpectedError
			break
		}

		capacity, available, unhealthyAddrs, err = ic.GetIPAddressUtilization(poolID)
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetIPUtilization failed %v", err.Error())
			returnCode = UnexpectedError
			break
		}
		log.Printf("[Azure CNS] Capacity %v Available %v UnhealthyAddrs %v", capacity, available, unhealthyAddrs)

	default:
		returnMessage = "[Azure CNS] Error. GetUnhealthyIP did not receive a POST."
		returnCode = InvalidParameter
	}

	resp := cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}

	ipResp := &cns.GetIPAddressesResponse{
		Response:    resp,
		IPAddresses: unhealthyAddrs,
	}

	err := service.Listener.Encode(w, &ipResp)

	log.Response(service.Name, ipResp, err)
}

// getAllIPAddresses retrieves all ip addresses from ipam driver.
func (service *httpRestService) getAllIPAddresses(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Azure CNS] getAllIPAddresses")
	log.Request(service.Name, "getAllIPAddresses", nil)

	switch r.Method {
	case "GET":
	default:
	}

	resp := cns.Response{ReturnCode: 0}
	ipResp := &cns.GetIPAddressesResponse{Response: resp}
	err := service.Listener.Encode(w, &ipResp)

	log.Response(service.Name, ipResp, err)
}

// Handles health report requests.
func (service *httpRestService) getHealthReport(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Azure CNS] getHealthReport")
	log.Request(service.Name, "getHealthReport", nil)

	switch r.Method {
	case "GET":
	default:
	}

	resp := &cns.Response{ReturnCode: 0}
	err := service.Listener.Encode(w, &resp)

	log.Response(service.Name, resp, err)
}

// saveState writes CNS state to persistent store.
func (service *httpRestService) saveState() error {
	log.Printf("[Azure CNS] saveState")

	// Skip if a store is not provided.
	if service.store == nil {
		log.Printf("[Azure CNS]  store not initialized.")
		return nil
	}

	// Update time stamp.
	service.state.TimeStamp = time.Now()
	err := service.store.Write(storeKey, &service.state)
	if err == nil {
		log.Printf("[Azure CNS]  State saved successfully.\n")
	} else {
		log.Printf("[Azure CNS]  Failed to save state., err:%v\n", err)
	}

	return err
}

// restoreState restores CNS state from persistent store.
func (service *httpRestService) restoreState() error {
	log.Printf("[Azure CNS] restoreState")

	// Skip if a store is not provided.
	if service.store == nil {
		log.Printf("[Azure CNS]  store not initialized.")
		return nil
	}

	// Read any persisted state.
	err := service.store.Read(storeKey, &service.state)
	if err != nil {
		if err == store.ErrKeyNotFound {
			// Nothing to restore.
			log.Printf("[Azure CNS]  No state to restore.\n")
			return nil
		}

		log.Printf("[Azure CNS]  Failed to restore state, err:%v\n", err)
		return err
	}

	log.Printf("[Azure CNS]  Restored state, %+v\n", service.state)
	return nil
}
