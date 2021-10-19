// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"regexp"
	"runtime"
	"strings"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/hnsclient"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/nmagentclient"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/cns/wireserver"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/platform"
)

// This file contains implementation of all HTTP APIs which are exposed to external clients.
// TODO: break it even further per module (network, nc, etc) like it is done for ipam

// Handles requests to set the environment type.
func (service *HTTPRestService) setEnvironment(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] setEnvironment")

	var req cns.SetEnvironmentRequest
	err := service.Listener.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)

	if err != nil {
		return
	}

	switch r.Method {
	case "POST":
		logger.Printf("[Azure CNS]  POST received for SetEnvironment.")
		service.state.Location = req.Location
		service.state.NetworkType = req.NetworkType
		service.state.Initialized = true
		service.saveState()
	default:
	}

	resp := &cns.Response{ReturnCode: 0}
	err = service.Listener.Encode(w, &resp)

	logger.Response(service.Name, resp, resp.ReturnCode, err)
}

// Handles CreateNetwork requests.
func (service *HTTPRestService) createNetwork(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] createNetwork")

	var err error
	var returnCode types.ResponseCode
	returnMessage := ""

	if service.state.Initialized {
		var req cns.CreateNetworkRequest
		err = service.Listener.Decode(w, r, &req)
		logger.Request(service.Name, &req, err)

		if err != nil {
			//nolint:goconst // ignore const string
			returnMessage = "[Azure CNS] Error. Unable to decode input request."
			returnCode = types.InvalidParameter
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
							logger.Printf("[Azure CNS] Creating network with name %v.", req.NetworkName)

							err = rt.GetRoutingTable()
							if err != nil {
								// We should not fail the call to create network for this.
								// This is because restoring routes is a fallback mechanism in case
								// network driver is not behaving as expected.
								// The responsibility to restore routes is with network driver.
								logger.Printf("[Azure CNS] Unable to get routing table from node, %+v.", err.Error())
							}

							var nicInfo *wireserver.InterfaceInfo
							nicInfo, err = service.getPrimaryHostInterface(context.TODO())
							if err != nil {
								returnMessage = fmt.Sprintf("[Azure CNS] Error. GetPrimaryInterfaceInfoFromHost failed %v.", err.Error())
								returnCode = types.UnexpectedError
								break
							}

							err = dc.CreateNetwork(req.NetworkName, nicInfo, req.Options)
							if err != nil {
								returnMessage = fmt.Sprintf("[Azure CNS] Error. CreateNetwork failed %v.", err.Error())
								returnCode = types.UnexpectedError
							}

							err = rt.RestoreRoutingTable()
							if err != nil {
								logger.Printf("[Azure CNS] Unable to restore routing table on node, %+v.", err.Error())
							}

							networkInfo := &networkInfo{
								NetworkName: req.NetworkName,
								NicInfo:     nicInfo,
								Options:     req.Options,
							}

							service.state.Networks[req.NetworkName] = networkInfo

						case "StandAlone":
							returnMessage = fmt.Sprintf("[Azure CNS] Error. Underlay network is not supported in StandAlone environment. %v.", err.Error())
							returnCode = types.UnsupportedEnvironment
						}
					case "Overlay":
						returnMessage = fmt.Sprintf("[Azure CNS] Error. Overlay support not yet available. %v.", err.Error())
						returnCode = types.UnsupportedEnvironment
					}
				} else {
					returnMessage = fmt.Sprintf("[Azure CNS] Received a request to create an already existing network %v", req.NetworkName)
					logger.Printf(returnMessage)
				}

			default:
				returnMessage = "[Azure CNS] Error. CreateNetwork did not receive a POST."
				returnCode = types.InvalidParameter
			}
		}

	} else {
		returnMessage = "[Azure CNS] Error. CNS is not yet initialized with environment."
		returnCode = types.UnsupportedEnvironment
	}

	resp := &cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}

	err = service.Listener.Encode(w, &resp)

	if returnCode == 0 {
		service.saveState()
	}

	logger.Response(service.Name, resp, resp.ReturnCode, err)
}

// Handles DeleteNetwork requests.
func (service *HTTPRestService) deleteNetwork(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] deleteNetwork")

	var req cns.DeleteNetworkRequest
	var returnCode types.ResponseCode
	returnMessage := ""
	err := service.Listener.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)

	if err != nil {
		return
	}

	switch r.Method {
	case "POST":
		dc := service.dockerClient
		err := dc.NetworkExists(req.NetworkName)

		// Network does exist
		if err == nil {
			logger.Printf("[Azure CNS] Deleting network with name %v.", req.NetworkName)
			err := dc.DeleteNetwork(req.NetworkName)
			if err != nil {
				returnMessage = fmt.Sprintf("[Azure CNS] Error. DeleteNetwork failed %v.", err.Error())
				returnCode = types.UnexpectedError
			}
		} else {
			if err == fmt.Errorf("Network not found") {
				logger.Printf("[Azure CNS] Received a request to delete network that does not exist: %v.", req.NetworkName)
			} else {
				returnCode = types.UnexpectedError
				returnMessage = err.Error()
			}
		}

	default:
		returnMessage = "[Azure CNS] Error. DeleteNetwork did not receive a POST."
		returnCode = types.InvalidParameter
	}

	resp := &cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}

	err = service.Listener.Encode(w, &resp)

	if returnCode == 0 {
		service.removeNetworkInfo(req.NetworkName)
		service.saveState()
	}

	logger.Response(service.Name, resp, resp.ReturnCode, err)
}

// Handles CreateHnsNetwork requests.
func (service *HTTPRestService) createHnsNetwork(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] createHnsNetwork")

	var err error
	var returnCode types.ResponseCode
	returnMessage := ""

	var req cns.CreateHnsNetworkRequest
	err = service.Listener.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)

	if err != nil {
		//nolint:goconst
		returnMessage = "[Azure CNS] Error. Unable to decode input request."
		returnCode = types.InvalidParameter
	} else {
		switch r.Method {
		case "POST":
			if err := hnsclient.CreateHnsNetwork(req); err == nil {
				// Save the newly created HnsNetwork name. CNS deleteHnsNetwork API
				// will only allow deleting these networks.
				networkInfo := &networkInfo{
					NetworkName: req.NetworkName,
				}
				service.setNetworkInfo(req.NetworkName, networkInfo)
				returnMessage = fmt.Sprintf("[Azure CNS] Successfully created HNS network: %s", req.NetworkName)
			} else {
				returnMessage = fmt.Sprintf("[Azure CNS] CreateHnsNetwork failed with error %v", err.Error())
				returnCode = types.UnexpectedError
			}
		default:
			returnMessage = "[Azure CNS] Error. CreateHnsNetwork did not receive a POST."
			returnCode = types.InvalidParameter
		}
	}

	resp := &cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}

	err = service.Listener.Encode(w, &resp)

	if returnCode == 0 {
		service.saveState()
	}

	logger.Response(service.Name, resp, resp.ReturnCode, err)
}

// Handles deleteHnsNetwork requests.
func (service *HTTPRestService) deleteHnsNetwork(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] deleteHnsNetwork")

	var err error
	var req cns.DeleteHnsNetworkRequest
	var returnCode types.ResponseCode
	returnMessage := ""

	err = service.Listener.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)

	if err != nil {
		//nolint:goconst
		returnMessage = "[Azure CNS] Error. Unable to decode input request."
		returnCode = types.InvalidParameter
	} else {
		switch r.Method {
		case "POST":
			networkInfo, found := service.getNetworkInfo(req.NetworkName)
			if found && networkInfo.NetworkName == req.NetworkName {
				if err = hnsclient.DeleteHnsNetwork(req.NetworkName); err == nil {
					returnMessage = fmt.Sprintf("[Azure CNS] Successfully deleted HNS network: %s", req.NetworkName)
				} else {
					returnMessage = fmt.Sprintf("[Azure CNS] DeleteHnsNetwork failed with error %v", err.Error())
					returnCode = types.UnexpectedError
				}
			} else {
				returnMessage = fmt.Sprintf("[Azure CNS] Network %s not found", req.NetworkName)
				returnCode = types.InvalidParameter
			}
		default:
			returnMessage = "[Azure CNS] Error. DeleteHnsNetwork did not receive a POST."
			returnCode = types.InvalidParameter
		}
	}

	resp := &cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}

	err = service.Listener.Encode(w, &resp)

	if returnCode == 0 {
		service.removeNetworkInfo(req.NetworkName)
		service.saveState()
	}

	logger.Response(service.Name, resp, resp.ReturnCode, err)
}

// Handles ip reservation requests.
func (service *HTTPRestService) reserveIPAddress(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] reserveIPAddress")

	var req cns.ReserveIPAddressRequest
	var returnCode types.ResponseCode
	returnMessage := ""
	addr := ""
	address := ""
	err := service.Listener.Decode(w, r, &req)

	logger.Request(service.Name, &req, err)

	if err != nil {
		return
	}

	if req.ReservationID == "" {
		returnCode = types.ReservationNotFound
		returnMessage = "[Azure CNS] Error. ReservationId is empty"
	}

	switch r.Method {
	case "POST":
		ic := service.ipamClient

		var ifInfo *wireserver.InterfaceInfo
		ifInfo, err = service.getPrimaryHostInterface(context.TODO())
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetPrimaryIfaceInfo failed %v", err.Error())
			returnCode = types.UnexpectedError
			break
		}

		asID, err := ic.GetAddressSpace()
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetAddressSpace failed %v", err.Error())
			returnCode = types.UnexpectedError
			break
		}

		poolID, err := ic.GetPoolID(asID, ifInfo.Subnet)
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetPoolID failed %v", err.Error())
			returnCode = types.UnexpectedError
			break
		}

		addr, err = ic.ReserveIPAddress(poolID, req.ReservationID)
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] ReserveIpAddress failed with %+v", err.Error())
			returnCode = types.AddressUnavailable
			break
		}

		addressIP, _, err := net.ParseCIDR(addr)
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] ParseCIDR failed with %+v", err.Error())
			returnCode = types.UnexpectedError
			break
		}
		address = addressIP.String()

	default:
		returnMessage = "[Azure CNS] Error. ReserveIP did not receive a POST."
		returnCode = types.InvalidParameter

	}

	resp := cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}

	reserveResp := &cns.ReserveIPAddressResponse{Response: resp, IPAddress: address}
	err = service.Listener.Encode(w, &reserveResp)
	logger.Response(service.Name, reserveResp, resp.ReturnCode, err)
}

// Handles release ip reservation requests.
func (service *HTTPRestService) releaseIPAddress(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] releaseIPAddress")

	var req cns.ReleaseIPAddressRequest
	var returnCode types.ResponseCode
	returnMessage := ""

	err := service.Listener.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)

	if err != nil {
		return
	}

	if req.ReservationID == "" {
		returnCode = types.ReservationNotFound
		returnMessage = "[Azure CNS] Error. ReservationId is empty"
	}

	switch r.Method {
	case "POST":
		ic := service.ipamClient

		var ifInfo *wireserver.InterfaceInfo
		ifInfo, err = service.getPrimaryHostInterface(context.TODO())
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetPrimaryIfaceInfo failed %v", err.Error())
			returnCode = types.UnexpectedError
			break
		}

		asID, err := ic.GetAddressSpace()
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetAddressSpace failed %v", err.Error())
			returnCode = types.UnexpectedError
			break
		}

		poolID, err := ic.GetPoolID(asID, ifInfo.Subnet)
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetPoolID failed %v", err.Error())
			returnCode = types.UnexpectedError
			break
		}

		err = ic.ReleaseIPAddress(poolID, req.ReservationID)
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] ReleaseIpAddress failed with %+v", err.Error())
			returnCode = types.ReservationNotFound
		}

	default:
		returnMessage = "[Azure CNS] Error. ReleaseIP did not receive a POST."
		returnCode = types.InvalidParameter
	}

	resp := cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}

	err = service.Listener.Encode(w, &resp)
	logger.Response(service.Name, resp, resp.ReturnCode, err)
}

// Retrieves the host local ip address. Containers can talk to host using this IP address.
func (service *HTTPRestService) getHostLocalIP(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] getHostLocalIP")
	logger.Request(service.Name, "getHostLocalIP", nil)

	var found bool
	var errmsg string
	hostLocalIP := "0.0.0.0"

	if service.state.Initialized {
		switch r.Method {
		case "GET":
			switch service.state.NetworkType {
			case "Underlay":
				if service.wscli != nil {
					piface, err := service.getPrimaryHostInterface(context.TODO())
					if err == nil {
						hostLocalIP = piface.PrimaryIP
						found = true
					} else {
						logger.Printf("[Azure-CNS] Received error from GetPrimaryInterfaceInfoFromMemory. err: %v", err.Error())
					}
				}

			case "Overlay":
				errmsg = "[Azure-CNS] Overlay is not yet supported."
			}

		default:
			errmsg = "[Azure-CNS] GetHostLocalIP API expects a GET."
		}
	}

	var returnCode types.ResponseCode
	if !found {
		returnCode = types.NotFound
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

	logger.Response(service.Name, hostLocalIPResponse, resp.ReturnCode, err)
}

// Handles ip address utilization requests.
func (service *HTTPRestService) getIPAddressUtilization(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] getIPAddressUtilization")
	logger.Request(service.Name, "getIPAddressUtilization", nil)

	var returnCode types.ResponseCode
	returnMessage := ""
	capacity := 0
	available := 0
	var unhealthyAddrs []string

	switch r.Method {
	case "GET":
		ic := service.ipamClient

		ifInfo, err := service.getPrimaryHostInterface(context.TODO())
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetPrimaryIfaceInfo failed %v", err.Error())
			returnCode = types.UnexpectedError
			break
		}

		asID, err := ic.GetAddressSpace()
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetAddressSpace failed %v", err.Error())
			returnCode = types.UnexpectedError
			break
		}

		poolID, err := ic.GetPoolID(asID, ifInfo.Subnet)
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetPoolID failed %v", err.Error())
			returnCode = types.UnexpectedError
			break
		}

		capacity, available, unhealthyAddrs, err = ic.GetIPAddressUtilization(poolID)
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetIPUtilization failed %v", err.Error())
			returnCode = types.UnexpectedError
			break
		}
		logger.Printf("[Azure CNS] Capacity %v Available %v UnhealthyAddrs %v", capacity, available, unhealthyAddrs)

	default:
		returnMessage = "[Azure CNS] Error. GetIPUtilization did not receive a GET."
		returnCode = types.InvalidParameter
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
	logger.Response(service.Name, utilResponse, resp.ReturnCode, err)
}

// Handles retrieval of ip addresses that are available to be reserved from ipam driver.
func (service *HTTPRestService) getAvailableIPAddresses(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] getAvailableIPAddresses")
	logger.Request(service.Name, "getAvailableIPAddresses", nil)

	switch r.Method {
	case "GET":
	default:
	}

	resp := cns.Response{ReturnCode: 0}
	ipResp := &cns.GetIPAddressesResponse{Response: resp}
	err := service.Listener.Encode(w, &ipResp)

	logger.Response(service.Name, ipResp, resp.ReturnCode, err)
}

// Handles retrieval of reserved ip addresses from ipam driver.
func (service *HTTPRestService) getReservedIPAddresses(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] getReservedIPAddresses")
	logger.Request(service.Name, "getReservedIPAddresses", nil)

	switch r.Method {
	case "GET":
	default:
	}

	resp := cns.Response{ReturnCode: 0}
	ipResp := &cns.GetIPAddressesResponse{Response: resp}
	err := service.Listener.Encode(w, &ipResp)

	logger.Response(service.Name, ipResp, resp.ReturnCode, err)
}

// Handles retrieval of ghost ip addresses from ipam driver.
func (service *HTTPRestService) getUnhealthyIPAddresses(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] getUnhealthyIPAddresses")
	logger.Request(service.Name, "getUnhealthyIPAddresses", nil)

	var returnCode types.ResponseCode
	returnMessage := ""
	capacity := 0
	available := 0
	var unhealthyAddrs []string

	switch r.Method {
	case "GET":
		ic := service.ipamClient

		ifInfo, err := service.getPrimaryHostInterface(context.TODO())
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetPrimaryIfaceInfo failed %v", err.Error())
			returnCode = types.UnexpectedError
			break
		}

		asID, err := ic.GetAddressSpace()
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetAddressSpace failed %v", err.Error())
			returnCode = types.UnexpectedError
			break
		}

		poolID, err := ic.GetPoolID(asID, ifInfo.Subnet)
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetPoolID failed %v", err.Error())
			returnCode = types.UnexpectedError
			break
		}

		capacity, available, unhealthyAddrs, err = ic.GetIPAddressUtilization(poolID)
		if err != nil {
			returnMessage = fmt.Sprintf("[Azure CNS] Error. GetIPUtilization failed %v", err.Error())
			returnCode = types.UnexpectedError
			break
		}
		logger.Printf("[Azure CNS] Capacity %v Available %v UnhealthyAddrs %v", capacity, available, unhealthyAddrs)

	default:
		returnMessage = "[Azure CNS] Error. GetUnhealthyIP did not receive a POST."
		returnCode = types.InvalidParameter
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
	logger.Response(service.Name, ipResp, resp.ReturnCode, err)
}

// getAllIPAddresses retrieves all ip addresses from ipam driver.
func (service *HTTPRestService) getAllIPAddresses(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] getAllIPAddresses")
	logger.Request(service.Name, "getAllIPAddresses", nil)

	switch r.Method {
	case "GET":
	default:
	}

	resp := cns.Response{ReturnCode: 0}
	ipResp := &cns.GetIPAddressesResponse{Response: resp}
	err := service.Listener.Encode(w, &ipResp)

	logger.Response(service.Name, ipResp, resp.ReturnCode, err)
}

// Handles health report requests.
func (service *HTTPRestService) getHealthReport(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] getHealthReport")
	logger.Request(service.Name, "getHealthReport", nil)

	switch r.Method {
	case "GET":
	default:
	}

	resp := &cns.Response{ReturnCode: 0}
	err := service.Listener.Encode(w, &resp)

	logger.Response(service.Name, resp, resp.ReturnCode, err)
}

func (service *HTTPRestService) setOrchestratorType(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] setOrchestratorType")

	var (
		req           cns.SetOrchestratorTypeRequest
		returnMessage string
		returnCode    types.ResponseCode
		nodeID        string
	)

	err := service.Listener.Decode(w, r, &req)
	if err != nil {
		return
	}

	service.Lock()

	service.dncPartitionKey = req.DncPartitionKey
	nodeID = service.state.NodeID

	if nodeID == "" || nodeID == req.NodeID {
		switch req.OrchestratorType {
		case cns.ServiceFabric:
			fallthrough
		case cns.Kubernetes:
			fallthrough
		case cns.KubernetesCRD:
			fallthrough
		case cns.WebApps:
			fallthrough
		case cns.Batch:
			fallthrough
		case cns.DBforPostgreSQL:
			fallthrough
		case cns.AzureFirstParty:
			service.state.OrchestratorType = req.OrchestratorType
			service.state.NodeID = req.NodeID
			logger.SetContextDetails(req.OrchestratorType, req.NodeID)
			service.saveState()
		default:
			returnMessage = fmt.Sprintf("Invalid Orchestrator type %v", req.OrchestratorType)
			returnCode = types.UnsupportedOrchestratorType
		}
	} else {
		returnMessage = fmt.Sprintf("Invalid request since this node has already been registered as %s", nodeID)
		returnCode = types.InvalidRequest
	}

	service.Unlock()

	resp := cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}

	err = service.Listener.Encode(w, &resp)
	logger.Response(service.Name, resp, resp.ReturnCode, err)
}

func (service *HTTPRestService) createOrUpdateNetworkContainer(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] createOrUpdateNetworkContainer")

	var req cns.CreateNetworkContainerRequest
	err := service.Listener.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)
	if err != nil {
		return
	}

	var returnCode types.ResponseCode
	var returnMessage string
	switch r.Method {
	case "POST":
		if req.NetworkContainerType == cns.WebApps {
			// try to get the saved nc state if it exists
			existing, ok := service.getNetworkContainerDetails(req.NetworkContainerid)

			// create/update nc only if it doesn't exist or it exists and the requested version is different from the saved version
			if !ok || (ok && existing.VMVersion != req.Version) {
				nc := service.networkContainer
				if err = nc.Create(req); err != nil {
					returnMessage = fmt.Sprintf("[Azure CNS] Error. CreateOrUpdateNetworkContainer failed %v", err.Error())
					returnCode = types.UnexpectedError
					break
				}
			}
		} else if req.NetworkContainerType == cns.AzureContainerInstance {
			// try to get the saved nc state if it exists
			existing, ok := service.getNetworkContainerDetails(req.NetworkContainerid)

			// create/update nc only if it doesn't exist or it exists and the requested version is different from the saved version
			if ok && existing.VMVersion != req.Version {
				nc := service.networkContainer
				netPluginConfig := service.getNetPluginDetails()
				if err = nc.Update(req, netPluginConfig); err != nil {
					returnMessage = fmt.Sprintf("[Azure CNS] Error. CreateOrUpdateNetworkContainer failed %v", err.Error())
					returnCode = types.UnexpectedError
					break
				}
			}
		}

		returnCode, returnMessage = service.saveNetworkContainerGoalState(req)

	default:
		returnMessage = "[Azure CNS] Error. CreateOrUpdateNetworkContainer did not receive a POST."
		returnCode = types.InvalidParameter
	}

	resp := cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}

	reserveResp := &cns.CreateNetworkContainerResponse{Response: resp}
	err = service.Listener.Encode(w, &reserveResp)

	// If the NC was created successfully, log NC snapshot.
	if returnCode == types.Success {
		logNCSnapshot(req)
	}

	logger.Response(service.Name, reserveResp, resp.ReturnCode, err)
}

func (service *HTTPRestService) getNetworkContainerByID(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] getNetworkContainerByID")

	var req cns.GetNetworkContainerRequest
	var returnCode types.ResponseCode
	returnMessage := ""

	err := service.Listener.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)
	if err != nil {
		return
	}

	resp := cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}

	reserveResp := &cns.GetNetworkContainerResponse{Response: resp}
	err = service.Listener.Encode(w, &reserveResp)
	logger.Response(service.Name, reserveResp, resp.ReturnCode, err)
}

func (service *HTTPRestService) getNetworkContainerByOrchestratorContext(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] getNetworkContainerByOrchestratorContext")

	var req cns.GetNetworkContainerRequest

	err := service.Listener.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)
	if err != nil {
		return
	}

	// getNetworkContainerByOrchestratorContext gets called for multitenancy and
	// setting the SDNRemoteArpMacAddress regKey is essential for the multitenancy
	// to work correctly in case of windows platform. Return if there is an error
	if err = platform.SetSdnRemoteArpMacAddress(); err != nil {
		logger.Printf("[Azure CNS] SetSdnRemoteArpMacAddress failed with error: %s", err.Error())
		return
	}

	getNetworkContainerResponse := service.getNetworkContainerResponse(req)
	returnCode := getNetworkContainerResponse.Response.ReturnCode
	err = service.Listener.Encode(w, &getNetworkContainerResponse)
	logger.Response(service.Name, getNetworkContainerResponse, returnCode, err)
}

func (service *HTTPRestService) deleteNetworkContainer(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] deleteNetworkContainer")

	var req cns.DeleteNetworkContainerRequest
	var returnCode types.ResponseCode
	returnMessage := ""

	err := service.Listener.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)
	if err != nil {
		return
	}

	if req.NetworkContainerid == "" {
		returnCode = types.NetworkContainerNotSpecified
		returnMessage = "[Azure CNS] Error. NetworkContainerid is empty"
	}

	switch r.Method {
	case "POST":
		var containerStatus containerstatus
		var ok bool

		containerStatus, ok = service.getNetworkContainerDetails(req.NetworkContainerid)

		if !ok {
			logger.Printf("Not able to retrieve network container details for this container id %v", req.NetworkContainerid)
			break
		}

		if containerStatus.CreateNetworkContainerRequest.NetworkContainerType == cns.WebApps {
			nc := service.networkContainer
			if err := nc.Delete(req.NetworkContainerid); err != nil {
				returnMessage = fmt.Sprintf("[Azure CNS] Error. DeleteNetworkContainer failed %v", err.Error())
				returnCode = types.UnexpectedError
				break
			}
		}

		service.Lock()
		defer service.Unlock()

		if service.state.ContainerStatus != nil {
			delete(service.state.ContainerStatus, req.NetworkContainerid)
		}

		if service.state.ContainerIDByOrchestratorContext != nil {
			for orchestratorContext, networkContainerID := range service.state.ContainerIDByOrchestratorContext {
				if networkContainerID == req.NetworkContainerid {
					delete(service.state.ContainerIDByOrchestratorContext, orchestratorContext)
					break
				}
			}
		}

		service.saveState()
	default:
		returnMessage = "[Azure CNS] Error. DeleteNetworkContainer did not receive a POST."
		returnCode = types.InvalidParameter
	}

	resp := cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}

	reserveResp := &cns.DeleteNetworkContainerResponse{Response: resp}
	err = service.Listener.Encode(w, &reserveResp)
	logger.Response(service.Name, reserveResp, resp.ReturnCode, err)
}

func (service *HTTPRestService) getInterfaceForContainer(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] getInterfaceForContainer")

	var req cns.GetInterfaceForContainerRequest
	var returnCode types.ResponseCode
	returnMessage := ""

	err := service.Listener.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)
	if err != nil {
		return
	}

	containerInfo := service.state.ContainerStatus
	containerDetails, ok := containerInfo[req.NetworkContainerID]
	var interfaceName string
	var ipaddress string
	var cnetSpace []cns.IPSubnet
	var dnsServers []string
	var version string

	if ok {
		savedReq := containerDetails.CreateNetworkContainerRequest
		interfaceName = savedReq.NetworkContainerid
		cnetSpace = savedReq.CnetAddressSpace
		ipaddress = savedReq.IPConfiguration.IPSubnet.IPAddress // it has to exist
		dnsServers = savedReq.IPConfiguration.DNSServers
		version = savedReq.Version
	} else {
		returnMessage = "[Azure CNS] Never received call to create this container."
		returnCode = types.UnknownContainerID
		interfaceName = ""
		ipaddress = ""
		version = ""
	}

	resp := cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}

	getInterfaceForContainerResponse := cns.GetInterfaceForContainerResponse{
		Response:                resp,
		NetworkInterface:        cns.NetworkInterface{Name: interfaceName, IPAddress: ipaddress},
		CnetAddressSpace:        cnetSpace,
		DNSServers:              dnsServers,
		NetworkContainerVersion: version,
	}

	err = service.Listener.Encode(w, &getInterfaceForContainerResponse)

	logger.Response(service.Name, getInterfaceForContainerResponse, resp.ReturnCode, err)
}

func (service *HTTPRestService) attachNetworkContainerToNetwork(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] attachNetworkContainerToNetwork")

	var req cns.ConfigureContainerNetworkingRequest
	err := service.Listener.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)
	if err != nil {
		return
	}

	resp := service.attachOrDetachHelper(req, attach, r.Method)
	attachResp := &cns.AttachContainerToNetworkResponse{Response: resp}
	err = service.Listener.Encode(w, &attachResp)
	logger.Response(service.Name, attachResp, resp.ReturnCode, err)
}

func (service *HTTPRestService) detachNetworkContainerFromNetwork(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] detachNetworkContainerFromNetwork")

	var req cns.ConfigureContainerNetworkingRequest
	err := service.Listener.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)
	if err != nil {
		return
	}

	resp := service.attachOrDetachHelper(req, detach, r.Method)
	detachResp := &cns.DetachContainerFromNetworkResponse{Response: resp}
	err = service.Listener.Encode(w, &detachResp)
	logger.Response(service.Name, detachResp, resp.ReturnCode, err)
}

// Retrieves the number of logic processors on a node. It will be primarily
// used to enforce per VM delegated NIC limit by DNC.
func (service *HTTPRestService) getNumberOfCPUCores(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure-CNS] getNumberOfCPUCores")
	logger.Request(service.Name, "getNumberOfCPUCores", nil)

	var (
		num        int
		returnCode types.ResponseCode
		errMsg     string
	)

	switch r.Method {
	case "GET":
		num = runtime.NumCPU()
	default:
		errMsg = "[Azure-CNS] getNumberOfCPUCores API expects a GET."
		returnCode = types.UnsupportedVerb
	}

	resp := cns.Response{ReturnCode: returnCode, Message: errMsg}
	numOfCPUCoresResp := cns.NumOfCPUCoresResponse{
		Response:      resp,
		NumOfCPUCores: num,
	}

	err := service.Listener.Encode(w, &numOfCPUCoresResp)

	logger.Response(service.Name, numOfCPUCoresResp, resp.ReturnCode, err)
}

func getInterfaceIdFromCreateNetworkContainerURL(
	createNetworkContainerURL string) string {
	return strings.Split(strings.Split(createNetworkContainerURL, "interfaces/")[1], "/")[0]
}

func getAuthTokenFromCreateNetworkContainerURL(
	createNetworkContainerURL string) string {
	return strings.Split(strings.Split(createNetworkContainerURL, "authenticationToken/")[1], "/")[0]
}

var rgx = regexp.MustCompile("^http[s]?://(.*?)/joinedVirtualNetworks.*?$")

func extractHostFromJoinNetworkURL(url string) string {
	submatches := rgx.FindStringSubmatch(url)
	if len(submatches) != 2 {
		return ""
	}
	return submatches[1]
}

// Publish Network Container by calling nmagent
func (service *HTTPRestService) publishNetworkContainer(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure-CNS] PublishNetworkContainer")

	var (
		err                 error
		req                 cns.PublishNetworkContainerRequest
		returnCode          types.ResponseCode
		returnMessage       string
		publishResponse     *http.Response
		publishStatusCode   int
		publishResponseBody []byte
		publishError        error
		publishErrorStr     string
		isNetworkJoined     bool
	)

	err = service.Listener.Decode(w, r, &req)

	// reqCopy creates a copy of incoming request. It doesn't copy the authentication token info
	// to avoid logging it.
	reqCopy := cns.PublishNetworkContainerRequest{
		NetworkID:                 req.NetworkID,
		NetworkContainerID:        req.NetworkContainerID,
		JoinNetworkURL:            req.JoinNetworkURL,
		CreateNetworkContainerURL: strings.Split(req.CreateNetworkContainerURL, "authenticationToken")[0],
	}

	logger.Request(service.Name, &reqCopy, err)
	if err != nil {
		return
	}

	switch r.Method {
	case "POST":
		// Join the network
		publishResponse, publishError, err = service.joinNetwork(req.NetworkID, req.JoinNetworkURL)
		if err == nil {
			isNetworkJoined = true
		} else {
			returnMessage = err.Error()
			returnCode = types.NetworkJoinFailed
		}

		if isNetworkJoined {
			// Publish Network Container
			publishResponse, publishError = nmagentclient.PublishNetworkContainer(
				req.NetworkContainerID,
				req.CreateNetworkContainerURL,
				req.CreateNetworkContainerRequestBody)
			if publishError != nil || publishResponse.StatusCode != http.StatusOK {
				returnMessage = fmt.Sprintf("Failed to publish Network Container: %s", req.NetworkContainerID)
				returnCode = types.NetworkContainerPublishFailed
				logger.Errorf("[Azure-CNS] %s", returnMessage)
			}
		}

		// Store ncGetVersionURL needed for calling NMAgent to check if vfp programming is completed for the NC
		primaryInterfaceIdentifier := getInterfaceIdFromCreateNetworkContainerURL(req.CreateNetworkContainerURL)
		authToken := getAuthTokenFromCreateNetworkContainerURL(req.CreateNetworkContainerURL)

		// we attempt to extract the wireserver IP to use from the request, otherwise default to the well-known IP.
		hostIP := extractHostFromJoinNetworkURL(req.JoinNetworkURL)
		if hostIP == "" {
			hostIP = nmagentclient.WireserverIP
		}

		ncGetVersionURL := fmt.Sprintf(nmagentclient.GetNetworkContainerVersionURLFmt,
			hostIP,
			primaryInterfaceIdentifier,
			req.NetworkContainerID,
			authToken)
		ncVersionURLs.Store(cns.SwiftPrefix+req.NetworkContainerID, ncGetVersionURL)

	default:
		returnMessage = "PublishNetworkContainer API expects a POST"
		returnCode = types.UnsupportedVerb
	}

	if publishError != nil {
		publishErrorStr = publishError.Error()
	}

	if publishResponse != nil {
		publishStatusCode = publishResponse.StatusCode

		var errParse error
		publishResponseBody, errParse = ioutil.ReadAll(publishResponse.Body)
		if errParse != nil {
			returnMessage = fmt.Sprintf("Failed to parse the publish body. Error: %v", errParse)
			returnCode = types.UnexpectedError
			logger.Errorf("[Azure-CNS] %s", returnMessage)
		}

		publishResponse.Body.Close()
	}

	response := cns.PublishNetworkContainerResponse{
		Response: cns.Response{
			ReturnCode: returnCode,
			Message:    returnMessage,
		},
		PublishErrorStr:     publishErrorStr,
		PublishStatusCode:   publishStatusCode,
		PublishResponseBody: publishResponseBody,
	}

	err = service.Listener.Encode(w, &response)
	logger.Response(service.Name, response, response.Response.ReturnCode, err)
}

// Unpublish Network Container by calling nmagent
func (service *HTTPRestService) unpublishNetworkContainer(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure-CNS] UnpublishNetworkContainer")

	var (
		err                   error
		req                   cns.UnpublishNetworkContainerRequest
		returnCode            types.ResponseCode
		returnMessage         string
		unpublishResponse     *http.Response
		unpublishStatusCode   int
		unpublishResponseBody []byte
		unpublishError        error
		unpublishErrorStr     string
		isNetworkJoined       bool
	)

	err = service.Listener.Decode(w, r, &req)

	// reqCopy creates a copy of incoming request. It doesn't copy the authentication token info
	// to avoid logging it.
	reqCopy := cns.UnpublishNetworkContainerRequest{
		NetworkID:                 req.NetworkID,
		NetworkContainerID:        req.NetworkContainerID,
		JoinNetworkURL:            req.JoinNetworkURL,
		DeleteNetworkContainerURL: strings.Split(req.DeleteNetworkContainerURL, "authenticationToken")[0],
	}

	logger.Request(service.Name, &reqCopy, err)
	if err != nil {
		return
	}

	switch r.Method {
	case "POST":
		// Join Network if not joined already
		isNetworkJoined = service.isNetworkJoined(req.NetworkID)
		if !isNetworkJoined {
			unpublishResponse, unpublishError, err = service.joinNetwork(req.NetworkID, req.JoinNetworkURL)
			if err == nil {
				isNetworkJoined = true
			} else {
				returnMessage = err.Error()
				returnCode = types.NetworkJoinFailed
			}
		}

		if isNetworkJoined {
			// Unpublish Network Container
			unpublishResponse, unpublishError = nmagentclient.UnpublishNetworkContainer(
				req.NetworkContainerID,
				req.DeleteNetworkContainerURL)
			if unpublishError != nil || unpublishResponse.StatusCode != http.StatusOK {
				returnMessage = fmt.Sprintf("Failed to unpublish Network Container: %s", req.NetworkContainerID)
				returnCode = types.NetworkContainerUnpublishFailed
				logger.Errorf("[Azure-CNS] %s", returnMessage)
			}

			if unpublishResponse != nil {
				var errParse error
				unpublishResponseBody, errParse = ioutil.ReadAll(unpublishResponse.Body)
				if errParse != nil {
					returnMessage = fmt.Sprintf("Failed to parse the unpublish body. Error: %v", errParse)
					returnCode = types.UnexpectedError
					logger.Errorf("[Azure-CNS] %s", returnMessage)
				}

				unpublishResponse.Body.Close()
			}
		}

		// Remove the NC version URL entry added during publish
		ncVersionURLs.Delete(cns.SwiftPrefix + req.NetworkContainerID)
	default:
		returnMessage = "UnpublishNetworkContainer API expects a POST"
		returnCode = types.UnsupportedVerb
	}

	if unpublishError != nil {
		unpublishErrorStr = unpublishError.Error()
	}

	if unpublishResponse != nil {
		unpublishStatusCode = unpublishResponse.StatusCode
	}

	response := cns.UnpublishNetworkContainerResponse{
		Response: cns.Response{
			ReturnCode: returnCode,
			Message:    returnMessage,
		},
		UnpublishErrorStr:     unpublishErrorStr,
		UnpublishStatusCode:   unpublishStatusCode,
		UnpublishResponseBody: unpublishResponseBody,
	}

	err = service.Listener.Encode(w, &response)
	logger.Response(service.Name, response, response.Response.ReturnCode, err)
}

func (service *HTTPRestService) createHostNCApipaEndpoint(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure-CNS] createHostNCApipaEndpoint")

	var (
		err           error
		req           cns.CreateHostNCApipaEndpointRequest
		returnCode    types.ResponseCode
		returnMessage string
		endpointID    string
	)

	err = service.Listener.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)
	if err != nil {
		return
	}

	switch r.Method {
	case "POST":
		networkContainerDetails, found := service.getNetworkContainerDetails(req.NetworkContainerID)
		if found {
			if !networkContainerDetails.CreateNetworkContainerRequest.AllowNCToHostCommunication &&
				!networkContainerDetails.CreateNetworkContainerRequest.AllowHostToNCCommunication {
				returnMessage = fmt.Sprintf("HostNCApipaEndpoint creation is not supported unless " +
					"AllowNCToHostCommunication or AllowHostToNCCommunication is set to true")
				returnCode = types.InvalidRequest
			} else {
				if endpointID, err = hnsclient.CreateHostNCApipaEndpoint(
					req.NetworkContainerID,
					networkContainerDetails.CreateNetworkContainerRequest.LocalIPConfiguration,
					networkContainerDetails.CreateNetworkContainerRequest.AllowNCToHostCommunication,
					networkContainerDetails.CreateNetworkContainerRequest.AllowHostToNCCommunication,
					networkContainerDetails.CreateNetworkContainerRequest.EndpointPolicies); err != nil {
					returnMessage = fmt.Sprintf("CreateHostNCApipaEndpoint failed with error: %v", err)
					returnCode = types.UnexpectedError
				}
			}
		} else {
			returnMessage = fmt.Sprintf("CreateHostNCApipaEndpoint failed with error: Unable to find goal state for"+
				" the given Network Container: %s", req.NetworkContainerID)
			returnCode = types.UnknownContainerID
		}
	default:
		returnMessage = "createHostNCApipaEndpoint API expects a POST"
		returnCode = types.UnsupportedVerb
	}

	response := cns.CreateHostNCApipaEndpointResponse{
		Response: cns.Response{
			ReturnCode: returnCode,
			Message:    returnMessage,
		},
		EndpointID: endpointID,
	}

	err = service.Listener.Encode(w, &response)
	logger.Response(service.Name, response, response.Response.ReturnCode, err)
}

func (service *HTTPRestService) deleteHostNCApipaEndpoint(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure-CNS] deleteHostNCApipaEndpoint")

	var (
		err           error
		req           cns.DeleteHostNCApipaEndpointRequest
		returnCode    types.ResponseCode
		returnMessage string
	)

	err = service.Listener.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)
	if err != nil {
		return
	}

	switch r.Method {
	case "POST":
		if err = hnsclient.DeleteHostNCApipaEndpoint(req.NetworkContainerID); err != nil {
			returnMessage = fmt.Sprintf("Failed to delete endpoint for Network Container: %s "+
				"due to error: %v", req.NetworkContainerID, err)
			returnCode = types.UnexpectedError
		}
	default:
		returnMessage = "deleteHostNCApipaEndpoint API expects a DELETE"
		returnCode = types.UnsupportedVerb
	}

	response := cns.DeleteHostNCApipaEndpointResponse{
		Response: cns.Response{
			ReturnCode: returnCode,
			Message:    returnMessage,
		},
	}

	err = service.Listener.Encode(w, &response)
	logger.Response(service.Name, response, response.Response.ReturnCode, err)
}

// This function is used to query NMagents's supported APIs list
func (service *HTTPRestService) nmAgentSupportedApisHandler(w http.ResponseWriter, r *http.Request) {
	logger.Request(service.Name, "nmAgentSupportedApisHandler", nil)
	var (
		err, retErr   error
		req           cns.NmAgentSupportedApisRequest
		returnCode    types.ResponseCode
		returnMessage string
		supportedApis []string
	)

	err = service.Listener.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)
	if err != nil {
		return
	}

	switch r.Method {
	case http.MethodPost:
		supportedApis, retErr = nmagentclient.GetNmAgentSupportedApis(common.GetHttpClient(),
			req.GetNmAgentSupportedApisURL)
		if retErr != nil {
			returnCode = types.NmAgentSupportedApisError
			returnMessage = fmt.Sprintf("[Azure-CNS] %s", retErr.Error())
		}
		if supportedApis == nil {
			supportedApis = []string{}
		}

	default:
		returnMessage = "[Azure-CNS] NmAgentSupported API list expects a POST method."
	}

	resp := cns.Response{ReturnCode: returnCode, Message: returnMessage}
	nmAgentSupportedApisResponse := &cns.NmAgentSupportedApisResponse{
		Response:      resp,
		SupportedApis: supportedApis,
	}

	serviceErr := service.Listener.Encode(w, &nmAgentSupportedApisResponse)

	logger.Response(service.Name, nmAgentSupportedApisResponse, resp.ReturnCode, serviceErr)
}
