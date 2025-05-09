// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/filter"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/store"
	"github.com/pkg/errors"
	"golang.org/x/exp/maps"
)

var (
	ErrStoreEmpty             = errors.New("empty endpoint state store")
	ErrParsePodIPFailed       = errors.New("failed to parse pod's ip")
	ErrNoNCs                  = errors.New("no NCs found in the CNS internal state")
	ErrOptManageEndpointState = errors.New("CNS is not set to manage the endpoint state")
	ErrEndpointStateNotFound  = errors.New("endpoint state could not be found in the statefile")
	ErrGetAllNCResponseEmpty  = errors.New("failed to get NC responses from statefile")
)

const (
	ContainerIDLength  = 8
	InfraInterfaceName = "eth0"
)

// requestIPConfigHandlerHelper validates the request, assign IPs and return the IPConfigs
func (service *HTTPRestService) requestIPConfigHandlerHelper(ctx context.Context, ipconfigsRequest cns.IPConfigsRequest) (*cns.IPConfigsResponse, error) {
	// For SWIFT v2 scenario, the validator function will also modify the ipconfigsRequest.
	podInfo, returnCode, returnMessage := service.validateIPConfigsRequest(ctx, ipconfigsRequest)
	if returnCode != types.Success {
		return &cns.IPConfigsResponse{
			Response: cns.Response{
				ReturnCode: returnCode,
				Message:    returnMessage,
			},
		}, errors.New("failed to validate ip config request")
	}

	var podIPInfoResult []cns.PodIpInfo
	if ipconfigsRequest.BackendInterfaceExist {
		for _, bNICMacAddress := range ipconfigsRequest.BackendInterfaceMacAddresses {
			PnPID, err := service.getPNPIDFromMacAddress(ctx, bNICMacAddress)
			if err != nil {
				return &cns.IPConfigsResponse{
					Response: cns.Response{
						ReturnCode: types.FailedToAllocateIPConfig,
						Message:    fmt.Sprintf("BackendNIC allocation failed: %v, config request is %v", err, ipconfigsRequest),
					},
					PodIPInfo: []cns.PodIpInfo{},
				}, err
			}
			podBackendInfo := cns.PodIpInfo{
				MacAddress: bNICMacAddress,
				NICType:    cns.BackendNIC,
				PnPID:      PnPID,
			}
			podIPInfoResult = append(podIPInfoResult, podBackendInfo)
		}
	}

	// record a pod requesting an IP
	service.podsPendingIPAssignment.Push(podInfo.Key())
	podIPInfo, err := requestIPConfigsHelper(service, ipconfigsRequest) //nolint:contextcheck // appease linter for revert PR
	if err != nil {
		return &cns.IPConfigsResponse{
			Response: cns.Response{
				ReturnCode: types.FailedToAllocateIPConfig,
				Message:    fmt.Sprintf("AllocateIPConfig failed: %v, IP config request is %v", err, ipconfigsRequest),
			},
			PodIPInfo: podIPInfo,
		}, err
	}

	// record a pod assigned an IP
	defer func() {
		// observe IP assignment wait time
		if since := service.podsPendingIPAssignment.Pop(podInfo.Key()); since > 0 {
			ipAssignmentLatency.Observe(since.Seconds())
		}
	}()

	// Check if http rest service managed endpoint state is set
	if service.Options[common.OptManageEndpointState] == true {
		err = service.updateEndpointState(ipconfigsRequest, podInfo, podIPInfo)
		if err != nil {
			return &cns.IPConfigsResponse{
				Response: cns.Response{
					ReturnCode: types.UnexpectedError,
					Message:    fmt.Sprintf("Update endpoint state failed: %v ", err),
				},
				PodIPInfo: podIPInfo,
			}, err
		}
	}

	podIPInfoResult = append(podIPInfoResult, podIPInfo...)
	return &cns.IPConfigsResponse{
		Response: cns.Response{
			ReturnCode: types.Success,
		},
		PodIPInfo: podIPInfoResult,
	}, nil
}

// requestIPConfigHandlerHelperStandalone validates the request, assign IPs and return the IPConfigs
func (service *HTTPRestService) requestIPConfigHandlerHelperStandalone(ctx context.Context, ipconfigsRequest cns.IPConfigsRequest) (*cns.IPConfigsResponse, error) {
	// For SWIFT v2 scenario, the validator function will also modify the ipconfigsRequest.
	podInfo, returnCode, returnMessage := service.validateIPConfigsRequest(ctx, ipconfigsRequest)
	if returnCode != types.Success {
		return &cns.IPConfigsResponse{
			Response: cns.Response{
				ReturnCode: returnCode,
				Message:    returnMessage,
			},
		}, errors.New("failed to validate ip config request or unmarshal orchestratorContext")
	}

	orchestratorContext, err := podInfo.OrchestratorContext()
	if err != nil {
		return &cns.IPConfigsResponse{}, fmt.Errorf("error getting orchestrator context from PodInfo %w", err)
	}
	cnsRequest := cns.GetNetworkContainerRequest{OrchestratorContext: orchestratorContext}

	// IMPORTANT: although SwiftV2 reuses the concept of NCs, NMAgent doesn't program NCs for SwiftV2, but
	// instead programs NICs. When getting SwiftV2 NCs, we want the NIC type and MAC address of the NCs.
	// TODO: we need another way to verify and sync NMAgent's NIC programming status. pending new NMAgent API or NIC programming status to be passed in the SwiftV2 create NC request.
	resp := service.getAllNetworkContainerResponses(cnsRequest) //nolint:contextcheck // not passed in any methods, appease linter
	// return err if returned list has no NCs
	if len(resp) == 0 {
		return &cns.IPConfigsResponse{
			Response: cns.Response{
				ReturnCode: types.FailedToAllocateIPConfig,
				Message:    fmt.Sprintf("AllocateIPConfig failed due to not getting NC Response from statefile, IP config request is %+v", ipconfigsRequest),
			},
		}, ErrGetAllNCResponseEmpty
	}

	// assign NICType and MAC Address for SwiftV2. we assume that there won't be any SwiftV1 NCs here
	podIPInfoList := make([]cns.PodIpInfo, 0, len(resp))
	for i := range resp {
		podIPInfo := cns.PodIpInfo{
			PodIPConfig:                     resp[i].IPConfiguration.IPSubnet,
			MacAddress:                      resp[i].NetworkInterfaceInfo.MACAddress,
			NICType:                         resp[i].NetworkInterfaceInfo.NICType,
			NetworkContainerPrimaryIPConfig: resp[i].IPConfiguration,
		}
		podIPInfoList = append(podIPInfoList, podIPInfo)
	}

	ipConfigsResp := &cns.IPConfigsResponse{
		Response: cns.Response{
			ReturnCode: types.Success,
		},
		PodIPInfo: podIPInfoList,
	}

	err = service.updatePodInfoWithInterfaces(ctx, ipConfigsResp)
	if err != nil {
		return &cns.IPConfigsResponse{
			Response: cns.Response{
				ReturnCode: types.FailedToAllocateIPConfig,
				Message:    fmt.Sprintf("AllocateIPConfig failed while updating pod with interfaces: %v, IP config request is %+v", err, ipconfigsRequest),
			},
		}, err
	}
	return ipConfigsResp, nil
}

func (service *HTTPRestService) updatePodInfoWithInterfaces(ctx context.Context, ipconfigResponse *cns.IPConfigsResponse) error {
	// fetching primary host interface to use below for updating IPConfigsResponse
	hostPrimaryInterface, err := service.getPrimaryHostInterface(ctx)
	if err != nil {
		return err
	}
	for i := range ipconfigResponse.PodIPInfo {
		ipconfigResponse.PodIPInfo[i].HostPrimaryIPInfo = cns.HostIPInfo{
			Gateway:   hostPrimaryInterface.Gateway,
			PrimaryIP: hostPrimaryInterface.PrimaryIP,
			Subnet:    hostPrimaryInterface.Subnet,
		}
	}
	return nil
}

// RequestIPConfigHandler requests an IPConfig from the CNS state
func (service *HTTPRestService) RequestIPConfigHandler(w http.ResponseWriter, r *http.Request) {
	opName := "requestIPConfigHandler"
	defer service.publishIPStateMetrics()
	var ipconfigRequest cns.IPConfigRequest
	err := common.Decode(w, r, &ipconfigRequest)
	logger.Request(opName, ipconfigRequest, err)
	if err != nil {
		return
	}

	// This method can only return EXACTLY 1 IP. If we have more than one NC then we expect to need to return one IP per NC
	if len(service.state.ContainerStatus) != 1 {
		// we send a response back saying that this API won't be able to return the amount of IPs needed to fulfill the request
		reserveResp := &cns.IPConfigResponse{
			Response: cns.Response{
				ReturnCode: types.InvalidRequest,
				Message:    fmt.Sprintf("Expected 1 NC when calling this API but found %d NCs", len(service.state.ContainerStatus)),
			},
		}
		w.Header().Set(cnsReturnCode, reserveResp.Response.ReturnCode.String())
		err = common.Encode(w, &reserveResp)
		logger.ResponseEx(opName, ipconfigRequest, reserveResp, reserveResp.Response.ReturnCode, err)
		return
	}

	// doesn't fill in DesiredIPAddresses if it is empty in the original request
	ipconfigsRequest := cns.IPConfigsRequest{
		PodInterfaceID:      ipconfigRequest.PodInterfaceID,
		InfraContainerID:    ipconfigRequest.InfraContainerID,
		OrchestratorContext: ipconfigRequest.OrchestratorContext,
		Ifname:              ipconfigRequest.Ifname,
	}
	if ipconfigRequest.DesiredIPAddress != "" {
		ipconfigsRequest.DesiredIPAddresses = []string{
			ipconfigRequest.DesiredIPAddress,
		}
	}

	ipConfigsResp, errResp := service.requestIPConfigHandlerHelper(r.Context(), ipconfigsRequest) //nolint:contextcheck // appease linter
	if errResp != nil {
		// As this API is expected to return IPConfigResponse, generate it from the IPConfigsResponse returned above
		reserveResp := &cns.IPConfigResponse{
			Response: ipConfigsResp.Response,
		}
		w.Header().Set(cnsReturnCode, reserveResp.Response.ReturnCode.String())
		err = common.Encode(w, &reserveResp)
		logger.ResponseEx(opName, ipconfigsRequest, reserveResp, reserveResp.Response.ReturnCode, err)
		return
	}

	// Checks to make sure we return exactly 1 IP
	// If IPAM assigned more than 1 IP then we need to raise an error since this API can only return one IP and IPAM may have assigned more than one
	if len(ipConfigsResp.PodIPInfo) != 1 {
		// we send a response back saying that this API won't be able to return the amount of IPs needed to fulfill the request
		reserveResp := &cns.IPConfigResponse{
			Response: cns.Response{
				ReturnCode: types.UnexpectedError,
				Message:    fmt.Sprintf("request returned incorrect number of IPs. Expected 1 and returned %d", len(ipConfigsResp.PodIPInfo)),
			},
		}
		w.Header().Set(cnsReturnCode, reserveResp.Response.ReturnCode.String())
		err = common.Encode(w, &reserveResp)
		logger.ResponseEx(opName, ipconfigRequest, reserveResp, reserveResp.Response.ReturnCode, err)
		return
	}
	// As this API is expected to return IPConfigResponse, generate it from the IPConfigsResponse returned above.
	reserveResp := &cns.IPConfigResponse{
		Response:  ipConfigsResp.Response,
		PodIpInfo: ipConfigsResp.PodIPInfo[0],
	}
	w.Header().Set(cnsReturnCode, reserveResp.Response.ReturnCode.String())
	err = common.Encode(w, &reserveResp)
	logger.ResponseEx(opName, ipconfigsRequest, reserveResp, reserveResp.Response.ReturnCode, err)
}

// RequestIPConfigsHandler requests multiple IPConfigs from the CNS state
func (service *HTTPRestService) RequestIPConfigsHandler(w http.ResponseWriter, r *http.Request) {
	opName := "requestIPConfigsHandler"
	defer service.publishIPStateMetrics()
	var ipconfigsRequest cns.IPConfigsRequest
	err := common.Decode(w, r, &ipconfigsRequest)
	logger.Request(opName, ipconfigsRequest, err)
	if err != nil {
		return
	}
	var ipConfigsResp *cns.IPConfigsResponse

	// Check if IPConfigsHandlerMiddleware is set
	if service.IPConfigsHandlerMiddleware != nil {
		// Wrap the default datapath handlers with the middleware depending on middleware type
		var wrappedHandler cns.IPConfigsHandlerFunc
		switch service.IPConfigsHandlerMiddleware.Type() {
		case cns.K8sSWIFTV2:
			wrappedHandler = service.IPConfigsHandlerMiddleware.IPConfigsRequestHandlerWrapper(service.requestIPConfigHandlerHelper, service.ReleaseIPConfigHandlerHelper)
		// this middleware is used for standalone swiftv2 secenario where a different helper is invoked as the PodInfo is read from cns state
		case cns.StandaloneSWIFTV2:
			wrappedHandler = service.IPConfigsHandlerMiddleware.IPConfigsRequestHandlerWrapper(service.requestIPConfigHandlerHelperStandalone, nil)
		}

		ipConfigsResp, err = wrappedHandler(r.Context(), ipconfigsRequest)
	} else {
		ipConfigsResp, err = service.requestIPConfigHandlerHelper(r.Context(), ipconfigsRequest) // nolint:contextcheck // appease linter
	}

	if err != nil {
		w.Header().Set(cnsReturnCode, ipConfigsResp.Response.ReturnCode.String())
		err = common.Encode(w, &ipConfigsResp)
		logger.ResponseEx(opName, ipconfigsRequest, ipConfigsResp, ipConfigsResp.Response.ReturnCode, err)
		return
	}

	w.Header().Set(cnsReturnCode, ipConfigsResp.Response.ReturnCode.String())
	err = common.Encode(w, &ipConfigsResp)
	logger.ResponseEx(opName, ipconfigsRequest, ipConfigsResp, ipConfigsResp.Response.ReturnCode, err)
}

func (service *HTTPRestService) updateEndpointState(ipconfigsRequest cns.IPConfigsRequest, podInfo cns.PodInfo, podIPInfo []cns.PodIpInfo) error {
	if service.EndpointStateStore == nil {
		return ErrStoreEmpty
	}
	service.Lock()
	defer service.Unlock()
	logger.Printf("[updateEndpointState] Updating endpoint state for infra container %s", ipconfigsRequest.InfraContainerID)
	for i := range podIPInfo {
		if endpointInfo, ok := service.EndpointState[ipconfigsRequest.InfraContainerID]; ok {
			logger.Warnf("[updateEndpointState] Found existing endpoint state for infra container %s", ipconfigsRequest.InfraContainerID)
			ip := net.ParseIP(podIPInfo[i].PodIPConfig.IPAddress)
			if ip == nil {
				logger.Errorf("failed to parse pod ip address %s", podIPInfo[i].PodIPConfig.IPAddress)
				return ErrParsePodIPFailed
			}
			if ip.To4() == nil { // is an ipv6 address
				ipconfig := net.IPNet{IP: ip, Mask: net.CIDRMask(int(podIPInfo[i].PodIPConfig.PrefixLength), 128)} // nolint
				for _, ipconf := range endpointInfo.IfnameToIPMap[ipconfigsRequest.Ifname].IPv6 {
					if ipconf.IP.Equal(ipconfig.IP) {
						logger.Printf("[updateEndpointState] Found existing ipv6 ipconfig for infra container %s", ipconfigsRequest.InfraContainerID)
						return nil
					}
				}
				endpointInfo.IfnameToIPMap[ipconfigsRequest.Ifname].IPv6 = append(endpointInfo.IfnameToIPMap[ipconfigsRequest.Ifname].IPv6, ipconfig)
			} else {
				ipconfig := net.IPNet{IP: ip, Mask: net.CIDRMask(int(podIPInfo[i].PodIPConfig.PrefixLength), 32)} // nolint
				for _, ipconf := range endpointInfo.IfnameToIPMap[ipconfigsRequest.Ifname].IPv4 {
					if ipconf.IP.Equal(ipconfig.IP) {
						logger.Printf("[updateEndpointState] Found existing ipv4 ipconfig for infra container %s", ipconfigsRequest.InfraContainerID)
						return nil
					}
				}
				endpointInfo.IfnameToIPMap[ipconfigsRequest.Ifname].IPv4 = append(endpointInfo.IfnameToIPMap[ipconfigsRequest.Ifname].IPv4, ipconfig)
			}
			service.EndpointState[ipconfigsRequest.InfraContainerID] = endpointInfo
		} else {
			endpointInfo := &EndpointInfo{PodName: podInfo.Name(), PodNamespace: podInfo.Namespace(), IfnameToIPMap: make(map[string]*IPInfo)}
			ip := net.ParseIP(podIPInfo[i].PodIPConfig.IPAddress)
			if ip == nil {
				logger.Errorf("failed to parse pod ip address %s", podIPInfo[i].PodIPConfig.IPAddress)
				return ErrParsePodIPFailed
			}
			ipInfo := &IPInfo{}
			if ip.To4() == nil { // is an ipv6 address
				ipconfig := net.IPNet{IP: ip, Mask: net.CIDRMask(int(podIPInfo[i].PodIPConfig.PrefixLength), 128)} // nolint
				ipInfo.IPv6 = append(ipInfo.IPv6, ipconfig)
			} else {
				ipconfig := net.IPNet{IP: ip, Mask: net.CIDRMask(int(podIPInfo[i].PodIPConfig.PrefixLength), 32)} // nolint
				ipInfo.IPv4 = append(ipInfo.IPv4, ipconfig)
			}
			endpointInfo.IfnameToIPMap[ipconfigsRequest.Ifname] = ipInfo
			service.EndpointState[ipconfigsRequest.InfraContainerID] = endpointInfo
		}

		err := service.EndpointStateStore.Write(EndpointStoreKey, service.EndpointState)
		if err != nil {
			return fmt.Errorf("failed to write endpoint state to store: %w", err)
		}
	}
	return nil
}

// ReleaseIPConfigHandlerHelper validates the request and removes the endpoint associated with the pod
func (service *HTTPRestService) ReleaseIPConfigHandlerHelper(ctx context.Context, ipconfigsRequest cns.IPConfigsRequest) (*cns.IPConfigsResponse, error) {
	podInfo, returnCode, returnMessage := service.validateIPConfigsRequest(ctx, ipconfigsRequest)
	if returnCode != types.Success {
		return &cns.IPConfigsResponse{
			Response: cns.Response{
				ReturnCode: returnCode,
				Message:    returnMessage,
			},
		}, fmt.Errorf("failed to validate ip config request") //nolint:goerr113 // return error
	}
	// Check if http rest service managed endpoint state is set
	if service.Options[common.OptManageEndpointState] == true {
		if err := service.removeEndpointState(podInfo); err != nil {
			resp := &cns.IPConfigsResponse{
				Response: cns.Response{
					ReturnCode: types.UnexpectedError,
					Message:    err.Error(),
				},
			}
			return resp, fmt.Errorf("ReleaseIPConfigHandlerHelper remove endpoint state failed : %v, release IP config info %+v", resp.Response.Message, ipconfigsRequest) //nolint:goerr113 // return error
		}
	}

	if err := service.releaseIPConfigs(podInfo); err != nil {
		return &cns.IPConfigsResponse{
			Response: cns.Response{
				ReturnCode: types.UnexpectedError,
				Message:    err.Error(),
			},
		}, fmt.Errorf("ReleaseIPConfigHandlerHelper releaseIPConfigs failed : %v, release IP config info %+v", returnMessage, ipconfigsRequest) //nolint:goerr113 // return error
	}

	return &cns.IPConfigsResponse{
		Response: cns.Response{
			ReturnCode: types.Success,
			Message:    "",
		},
	}, nil
}

// ReleaseIPConfigHandler frees the IP assigned to a pod from CNS
func (service *HTTPRestService) ReleaseIPConfigHandler(w http.ResponseWriter, r *http.Request) {
	opName := "releaseIPConfigHandler"
	defer service.publishIPStateMetrics()
	var ipconfigRequest cns.IPConfigRequest
	err := common.Decode(w, r, &ipconfigRequest)
	logger.Request(opName, ipconfigRequest, err)
	if err != nil {
		resp := cns.Response{
			ReturnCode: types.UnexpectedError,
			Message:    err.Error(),
		}
		logger.Errorf("releaseIPConfigHandler decode failed becase %v, release IP config info %s", resp.Message, ipconfigRequest)
		w.Header().Set(cnsReturnCode, resp.ReturnCode.String())
		err = common.Encode(w, &resp)
		logger.ResponseEx(opName, ipconfigRequest, resp, resp.ReturnCode, err)
		return
	}

	// check to make sure there is only one NC
	if len(service.state.ContainerStatus) != 1 {
		reserveResp := &cns.IPConfigResponse{
			Response: cns.Response{
				ReturnCode: types.InvalidRequest,
				Message:    fmt.Sprintf("Expected 1 NC when calling this API but found %d NCs", len(service.state.ContainerStatus)),
			},
		}
		w.Header().Set(cnsReturnCode, reserveResp.Response.ReturnCode.String())
		err = common.Encode(w, &reserveResp)
		logger.ResponseEx(opName, ipconfigRequest, reserveResp, reserveResp.Response.ReturnCode, err)
		return
	}

	ipconfigsRequest := cns.IPConfigsRequest{
		DesiredIPAddresses: []string{
			ipconfigRequest.DesiredIPAddress,
		},
		PodInterfaceID:      ipconfigRequest.PodInterfaceID,
		InfraContainerID:    ipconfigRequest.InfraContainerID,
		OrchestratorContext: ipconfigRequest.OrchestratorContext,
		Ifname:              ipconfigRequest.Ifname,
	}

	resp, err := service.ReleaseIPConfigHandlerHelper(r.Context(), ipconfigsRequest)
	if err != nil {
		w.Header().Set(cnsReturnCode, resp.Response.ReturnCode.String())
		err = common.Encode(w, &resp)
		logger.ResponseEx(opName, ipconfigRequest, resp, resp.Response.ReturnCode, err)
	}

	w.Header().Set(cnsReturnCode, resp.Response.ReturnCode.String())
	err = common.Encode(w, &resp)
	logger.ResponseEx(opName, ipconfigRequest, resp, resp.Response.ReturnCode, err)
}

// ReleaseIPConfigsHandler frees multiple IPConfigs from the CNS state
func (service *HTTPRestService) ReleaseIPConfigsHandler(w http.ResponseWriter, r *http.Request) {
	opName := "releaseIPConfigsHandler"
	defer service.publishIPStateMetrics()
	var ipconfigsRequest cns.IPConfigsRequest
	err := common.Decode(w, r, &ipconfigsRequest)
	logger.Request("releaseIPConfigsHandler", ipconfigsRequest, err)
	if err != nil {
		resp := cns.Response{
			ReturnCode: types.UnexpectedError,
			Message:    err.Error(),
		}
		logger.Errorf("releaseIPConfigsHandler decode failed because %v, release IP config info %+v", resp.Message, ipconfigsRequest)
		w.Header().Set(cnsReturnCode, resp.ReturnCode.String())
		err = common.Encode(w, &resp)
		logger.ResponseEx(opName, ipconfigsRequest, resp, resp.ReturnCode, err)
		return
	}

	resp, err := service.ReleaseIPConfigHandlerHelper(r.Context(), ipconfigsRequest)
	if err != nil {
		w.Header().Set(cnsReturnCode, resp.Response.ReturnCode.String())
		err = common.Encode(w, &resp)
		logger.ResponseEx(opName, ipconfigsRequest, resp, resp.Response.ReturnCode, err)
	}

	w.Header().Set(cnsReturnCode, resp.Response.ReturnCode.String())
	err = common.Encode(w, &resp)
	logger.ResponseEx(opName, ipconfigsRequest, resp, resp.Response.ReturnCode, err)
}

func (service *HTTPRestService) removeEndpointState(podInfo cns.PodInfo) error {
	if service.EndpointStateStore == nil {
		return ErrStoreEmpty
	}
	service.Lock()
	defer service.Unlock()
	logger.Printf("[removeEndpointState] Removing endpoint state for infra container %s", podInfo.InfraContainerID())
	if _, ok := service.EndpointState[podInfo.InfraContainerID()]; ok {
		delete(service.EndpointState, podInfo.InfraContainerID())
		err := service.EndpointStateStore.Write(EndpointStoreKey, service.EndpointState)
		if err != nil {
			return fmt.Errorf("failed to write endpoint state to store: %w", err)
		}
	} else { // will not fail if no endpoint state for infra container id is found
		logger.Printf("[removeEndpointState] No endpoint state found for infra container %s", podInfo.InfraContainerID())
	}
	return nil
}

// MarkIPAsPendingRelease will set the IPs which are in PendingProgramming or Available to PendingRelease state
// It will try to update [totalIpsToRelease]  number of ips.
func (service *HTTPRestService) MarkIPAsPendingRelease(totalIpsToRelease int) (map[string]cns.IPConfigurationStatus, error) {
	defer service.publishIPStateMetrics()
	pendingReleasedIps := make(map[string]cns.IPConfigurationStatus)
	service.Lock()
	defer service.Unlock()

	for uuid, existingIpConfig := range service.PodIPConfigState {
		if existingIpConfig.GetState() == types.PendingProgramming {
			updatedIPConfig, err := service.updateIPConfigState(uuid, types.PendingRelease, existingIpConfig.PodInfo)
			if err != nil {
				return nil, err
			}

			pendingReleasedIps[uuid] = updatedIPConfig
			if len(pendingReleasedIps) == totalIpsToRelease {
				return pendingReleasedIps, nil
			}
		}
	}

	// if not all expected IPs are set to PendingRelease, then check the Available IPs
	for uuid, existingIpConfig := range service.PodIPConfigState {
		if existingIpConfig.GetState() == types.Available {
			updatedIPConfig, err := service.updateIPConfigState(uuid, types.PendingRelease, existingIpConfig.PodInfo)
			if err != nil {
				return nil, err
			}

			pendingReleasedIps[uuid] = updatedIPConfig

			if len(pendingReleasedIps) == totalIpsToRelease {
				return pendingReleasedIps, nil
			}
		}
	}

	logger.Printf("[MarkIPAsPendingRelease] Set total ips to PendingRelease %d, expected %d", len(pendingReleasedIps), totalIpsToRelease)
	return pendingReleasedIps, nil
}

// MarkIPAsPendingRelease will attempt to set [n] number of ips to PendingRelease state.
// It will start with any IPs in PendingProgramming state and then move on to any IPs in Allocated state
// until it has reached the target release quantity.
// If it is unable to set the expected number of IPs to PendingRelease, it will revert the changed IPs
// and return an error.
// MarkNIPsPendingRelease is no-op if [n] is not a positive integer.
func (service *HTTPRestService) MarkNIPsPendingRelease(n int) (map[string]cns.IPConfigurationStatus, error) {
	defer service.publishIPStateMetrics()
	service.Lock()
	defer service.Unlock()
	// try to release from PendingProgramming
	pendingProgrammingIPs := make(map[string]cns.IPConfigurationStatus)
	for uuid, ipConfig := range service.PodIPConfigState { //nolint:gocritic // intentional value copy
		if n <= 0 {
			break
		}
		if ipConfig.GetState() == types.PendingProgramming {
			updatedIPConfig, err := service.updateIPConfigState(uuid, types.PendingRelease, ipConfig.PodInfo)
			if err != nil {
				return nil, err
			}

			pendingProgrammingIPs[uuid] = updatedIPConfig
			n--
		}
	}

	// try to release from Available
	availableIPs := make(map[string]cns.IPConfigurationStatus)
	for uuid, ipConfig := range service.PodIPConfigState { //nolint:gocritic // intentional value copy
		if n <= 0 {
			break
		}
		if ipConfig.GetState() == types.Available {
			updatedIPConfig, err := service.updateIPConfigState(uuid, types.PendingRelease, ipConfig.PodInfo)
			if err != nil {
				return nil, err
			}

			availableIPs[uuid] = updatedIPConfig
			n--
		}
	}

	// if we can release the requested quantity, return the IPs
	if n <= 0 {
		maps.Copy(pendingProgrammingIPs, availableIPs)
		return pendingProgrammingIPs, nil
	}

	// else revert changes
	for uuid, ipConfig := range pendingProgrammingIPs { //nolint:gocritic // intentional value copy
		_, _ = service.updateIPConfigState(uuid, types.PendingProgramming, ipConfig.PodInfo)
	}
	for uuid, ipConfig := range availableIPs { //nolint:gocritic // intentional value copy
		_, _ = service.updateIPConfigState(uuid, types.Available, ipConfig.PodInfo)
	}
	return nil, errors.New("unable to release requested number of IPs")
}

// TODO: Add a change so that we should only update the current state if it is different than the new state
func (service *HTTPRestService) updateIPConfigState(ipID string, updatedState types.IPState, podInfo cns.PodInfo) (cns.IPConfigurationStatus, error) {
	if ipConfig, found := service.PodIPConfigState[ipID]; found {
		logger.Printf("[updateIPConfigState] Changing IpId [%s] state to [%s], podInfo [%+v]. Current config [%+v]", ipID, updatedState, podInfo, ipConfig)
		ipConfig.SetState(updatedState)
		ipConfig.PodInfo = podInfo
		service.PodIPConfigState[ipID] = ipConfig
		return ipConfig, nil
	}

	//nolint:goerr113 //legacy
	return cns.IPConfigurationStatus{}, fmt.Errorf("[updateIPConfigState] Failed to update state %s for the IPConfig. ID %s not found PodIPConfigState", updatedState, ipID)
}

// MarkIpsAsAvailableUntransacted will update pending programming IPs to available if NMAgent side's programmed nc version keep up with nc version.
// Note: this func is an untransacted API as the caller will take a Service lock
func (service *HTTPRestService) MarkIpsAsAvailableUntransacted(ncID string, newHostNCVersion int) {
	// Check whether it exist in service state and get the related nc info
	if ncInfo, exist := service.state.ContainerStatus[ncID]; !exist {
		logger.Errorf("Can't find NC with ID %s in service state, stop updating its pending programming IP status", ncID)
	} else {
		previousHostNCVersion, err := strconv.Atoi(ncInfo.HostVersion)
		if err != nil {
			logger.Printf("[MarkIpsAsAvailableUntransacted] Get int value from ncInfo.HostVersion %s failed: %v, can't proceed", ncInfo.HostVersion, err)
			return
		}
		// We only need to handle the situation when dnc nc version is larger than programmed nc version
		if previousHostNCVersion < newHostNCVersion {
			for uuid, secondaryIPConfigs := range ncInfo.CreateNetworkContainerRequest.SecondaryIPConfigs {
				if ipConfigStatus, exist := service.PodIPConfigState[uuid]; !exist {
					logger.Errorf("IP %s with uuid as %s exist in service state Secondary IP list but can't find in PodIPConfigState", ipConfigStatus.IPAddress, uuid)
				} else if ipConfigStatus.GetState() == types.PendingProgramming && secondaryIPConfigs.NCVersion <= newHostNCVersion {
					_, err := service.updateIPConfigState(uuid, types.Available, nil)
					if err != nil {
						logger.Errorf("Error updating IPConfig [%+v] state to Available, err: %+v", ipConfigStatus, err)
					}

					// Following 2 sentence assign new host version to secondary ip config.
					secondaryIPConfigs.NCVersion = newHostNCVersion
					ncInfo.CreateNetworkContainerRequest.SecondaryIPConfigs[uuid] = secondaryIPConfigs
					logger.Printf("Change ip %s with uuid %s from pending programming to %s, current secondary ip configs is %+v", ipConfigStatus.IPAddress, uuid, types.Available,
						ncInfo.CreateNetworkContainerRequest.SecondaryIPConfigs[uuid])
				}
			}
		}
	}
}

func (service *HTTPRestService) GetPodIPConfigState() map[string]cns.IPConfigurationStatus {
	service.RLock()
	defer service.RUnlock()
	podIPConfigState := make(map[string]cns.IPConfigurationStatus, len(service.PodIPConfigState))
	for k, v := range service.PodIPConfigState {
		podIPConfigState[k] = v
	}
	return podIPConfigState
}

func (service *HTTPRestService) HandleDebugPodContext(w http.ResponseWriter, r *http.Request) { //nolint
	opName := "handleDebugPodContext"
	service.RLock()
	defer service.RUnlock()
	resp := cns.GetPodContextResponse{
		PodContext: service.PodIPIDByPodInterfaceKey,
	}
	err := common.Encode(w, &resp)
	logger.Response(opName, resp, resp.Response.ReturnCode, err)
}

func (service *HTTPRestService) HandleDebugRestData(w http.ResponseWriter, r *http.Request) { //nolint
	opName := "handleDebugRestData"
	service.RLock()
	defer service.RUnlock()
	resp := GetHTTPServiceDataResponse{
		HTTPRestServiceData: HTTPRestServiceData{
			PodIPIDByPodInterfaceKey: service.PodIPIDByPodInterfaceKey,
			PodIPConfigState:         service.PodIPConfigState,
		},
	}
	err := common.Encode(w, &resp)
	logger.Response(opName, resp, resp.Response.ReturnCode, err)
}

func (service *HTTPRestService) HandleDebugIPAddresses(w http.ResponseWriter, r *http.Request) {
	opName := "handleDebugIPAddresses"
	var req cns.GetIPAddressesRequest
	if err := common.Decode(w, r, &req); err != nil {
		resp := cns.GetIPAddressStatusResponse{
			Response: cns.Response{
				ReturnCode: types.UnexpectedError,
				Message:    err.Error(),
			},
		}
		err = common.Encode(w, &resp)
		logger.ResponseEx(opName, req, resp, resp.Response.ReturnCode, err)
		return
	}
	// Get all IPConfigs matching a state and return in the response
	resp := cns.GetIPAddressStatusResponse{
		IPConfigurationStatus: filter.MatchAnyIPConfigState(service.PodIPConfigState, filter.PredicatesForStates(req.IPConfigStateFilter...)...),
	}
	err := common.Encode(w, &resp)
	logger.ResponseEx(opName, req, resp, resp.Response.ReturnCode, err)
}

// GetAssignedIPConfigs returns a filtered list of IPs which are in
// Assigned State.
func (service *HTTPRestService) GetAssignedIPConfigs() []cns.IPConfigurationStatus {
	service.RLock()
	defer service.RUnlock()
	return filter.MatchAnyIPConfigState(service.PodIPConfigState, filter.StateAssigned)
}

// GetAvailableIPConfigs returns a filtered list of IPs which are in
// Available State.
func (service *HTTPRestService) GetAvailableIPConfigs() []cns.IPConfigurationStatus {
	service.RLock()
	defer service.RUnlock()
	return filter.MatchAnyIPConfigState(service.PodIPConfigState, filter.StateAvailable)
}

// GetPendingProgramIPConfigs returns a filtered list of IPs which are in
// PendingProgramming State.
func (service *HTTPRestService) GetPendingProgramIPConfigs() []cns.IPConfigurationStatus {
	service.RLock()
	defer service.RUnlock()
	return filter.MatchAnyIPConfigState(service.PodIPConfigState, filter.StatePendingProgramming)
}

// GetPendingReleaseIPConfigs returns a filtered list of IPs which are in
// PendingRelease State.
func (service *HTTPRestService) GetPendingReleaseIPConfigs() []cns.IPConfigurationStatus {
	service.RLock()
	defer service.RUnlock()
	return filter.MatchAnyIPConfigState(service.PodIPConfigState, filter.StatePendingRelease)
}

// assignIPConfig assigns the the ipconfig to the passed Pod, sets the state as Assigned, does not take a lock.
func (service *HTTPRestService) assignIPConfig(ipconfig cns.IPConfigurationStatus, podInfo cns.PodInfo) error { //nolint:gocritic // ignore hugeparam
	ipconfig, err := service.updateIPConfigState(ipconfig.ID, types.Assigned, podInfo)
	if err != nil {
		return err
	}

	if service.PodIPIDByPodInterfaceKey[podInfo.Key()] == nil {
		logger.Printf("IP config %v initialized", podInfo.Key())
		service.PodIPIDByPodInterfaceKey[podInfo.Key()] = make([]string, 0)
	}

	service.PodIPIDByPodInterfaceKey[podInfo.Key()] = append(service.PodIPIDByPodInterfaceKey[podInfo.Key()], ipconfig.ID)
	return nil
}

// unassignIPConfig unassigns the ipconfig from the passed Pod, sets the state as Available, does not take a lock.
func (service *HTTPRestService) unassignIPConfig(ipconfig cns.IPConfigurationStatus, podInfo cns.PodInfo) (cns.IPConfigurationStatus, error) { //nolint:gocritic // ignore hugeparam
	ipconfig, err := service.updateIPConfigState(ipconfig.ID, types.Available, nil)
	if err != nil {
		return cns.IPConfigurationStatus{}, err
	}

	delete(service.PodIPIDByPodInterfaceKey, podInfo.Key())
	logger.Printf("[setIPConfigAsAvailable] Deleted outdated pod info %s from PodIPIDByOrchestratorContext since IP %s with ID %s will be released and set as Available",
		podInfo.Key(), ipconfig.IPAddress, ipconfig.ID)
	return ipconfig, nil
}

// Todo - CNI should also pass the IPAddress which needs to be released to validate if that is the right IP allcoated
// in the first place.
func (service *HTTPRestService) releaseIPConfigs(podInfo cns.PodInfo) error {
	service.Lock()
	defer service.Unlock()
	ipsToBeReleased := make([]cns.IPConfigurationStatus, 0)
	logger.Printf("[releaseIPConfigs] Releasing pod with key %s", podInfo.Key())
	for i, ipID := range service.PodIPIDByPodInterfaceKey[podInfo.Key()] {
		if ipID != "" {
			if ipconfig, isExist := service.PodIPConfigState[ipID]; isExist {
				ipsToBeReleased = append(ipsToBeReleased, ipconfig)
			} else {
				//nolint:goerr113 // return error
				return fmt.Errorf("[releaseIPConfigs] Failed to get ipconfig %+v and pod info is %+v. Pod to IPID exists, but IPID to IPConfig doesn't exist, CNS State potentially corrupt",
					ipconfig.IPAddress, podInfo)
			}
		} else {
			logger.Errorf("[releaseIPConfigs] releaseIPConfigs could not find ipID at index %d for pod [%+v]", i, podInfo)
		}
	}

	failedToReleaseIP := false
	for _, ip := range ipsToBeReleased { //nolint:gocritic // ignore copy
		logger.Printf("[releaseIPConfigs] Releasing IP %s for pod %+v", ip.IPAddress, podInfo)
		if _, err := service.unassignIPConfig(ip, podInfo); err != nil {
			logger.Errorf("[releaseIPConfigs] Failed to release IP %s for pod %+v error: %+v", ip.IPAddress, podInfo, err)
			failedToReleaseIP = true
			break
		}

		logger.Printf("[releaseIPConfigs] Released IP %s for pod %+v", ip.IPAddress, podInfo)
	}

	if failedToReleaseIP {
		// reassigns all of the released IPs if we aren't able to release all of them
		for _, ip := range ipsToBeReleased { //nolint:gocritic // ignore copy
			if err := service.assignIPConfig(ip, podInfo); err != nil {
				logger.Errorf("[releaseIPConfigs] failed to mark IPConfig [%+v] back to Assigned. err: %v", ip, err)
			}
		}
		//nolint:goerr113 // return error
		return fmt.Errorf("[releaseIPConfigs] Failed to release one or more IPs. Not releasing any IPs for pod %+v", podInfo)
	}

	logger.Printf("[releaseIPConfigs] Successfully released all IPs for pod %+v", podInfo)
	return nil
}

// MarkExistingIPsAsPendingRelease is called when CNS is starting up and there are existing ipconfigs in the CRD that are marked as pending.
func (service *HTTPRestService) MarkExistingIPsAsPendingRelease(pendingIPIDs []string) error {
	service.Lock()
	defer service.Unlock()

	for _, id := range pendingIPIDs {
		if ipconfig, exists := service.PodIPConfigState[id]; exists {
			if ipconfig.GetState() == types.Assigned {
				return errors.Errorf("Failed to mark IP [%v] as pending, currently assigned", id)
			}

			logger.Printf("[MarkExistingIPsAsPending]: Marking IP [%+v] to PendingRelease", ipconfig)
			ipconfig.SetState(types.PendingRelease)
			service.PodIPConfigState[id] = ipconfig
		} else {
			logger.Errorf("Inconsistent state, ipconfig with ID [%v] marked as pending release, but does not exist in state", id)
		}
	}
	return nil
}

// Returns the current IP configs for a pod if they exist
func (service *HTTPRestService) GetExistingIPConfig(podInfo cns.PodInfo) ([]cns.PodIpInfo, bool, error) {
	service.RLock()
	defer service.RUnlock()

	numIPConfigs := len(service.PodIPIDByPodInterfaceKey[podInfo.Key()])
	podIPInfo := make([]cns.PodIpInfo, numIPConfigs)
	ipConfigExists := false

	for i, ipID := range service.PodIPIDByPodInterfaceKey[podInfo.Key()] {
		if ipID != "" {
			if ipState, isExist := service.PodIPConfigState[ipID]; isExist {
				if err := service.populateIPConfigInfoUntransacted(ipState, &podIPInfo[i]); err != nil {
					return podIPInfo, isExist, err
				}
				ipConfigExists = true
			} else {
				errMsg := fmt.Sprintf("Failed to get existing ipconfig for pod %+v. Pod to IPID exists, but IPID to IPConfig doesn't exist, CNS State potentially corrupt", podInfo)
				logger.Errorf(errMsg)
				return podIPInfo, false, errors.New(errMsg)
			}
		}
	}

	logger.Printf("[GetExistingIPConfig] IPConfigExists [%t] for pod [%+v]", ipConfigExists, podInfo.Key())
	return podIPInfo, ipConfigExists, nil
}

// Assigns a pod with all IPs desired
func (service *HTTPRestService) AssignDesiredIPConfigs(podInfo cns.PodInfo, desiredIPAddresses []string) ([]cns.PodIpInfo, error) {
	service.Lock()
	defer service.Unlock()

	// Gets the number of NCs which will determine the number of IPs given to a pod
	numOfNCs := len(service.state.ContainerStatus)
	// checks to make sure we have NCs before trying to get IPs
	if numOfNCs == 0 {
		return nil, ErrNoNCs
	}
	// Sets the number of desired IPs equal to the number of desired IPs passed in
	numDesiredIPAddresses := len(desiredIPAddresses)
	// Creates a slice of PodIpInfo with the size as number of NCs to hold the result for assigned IP configs
	podIPInfo := make([]cns.PodIpInfo, numDesiredIPAddresses)
	// creating a map for the loop to check to see if the IP in the pool is one of the desired IPs
	desiredIPMap := make(map[string]struct{})
	// slice to keep track of IP configs to assign
	ipConfigsToAssign := make([]cns.IPConfigurationStatus, 0)

	for _, desiredIP := range desiredIPAddresses {
		desiredIPMap[desiredIP] = struct{}{}
	}

	numIPConfigsAssigned := 0
	for _, ipConfig := range service.PodIPConfigState { //nolint:gocritic // ignore copy
		_, found := desiredIPMap[ipConfig.IPAddress]
		// keep searching until the all the desired IPs are found
		if !found {
			continue
		}

		switch ipConfig.GetState() { //nolint:exhaustive // ignoring PendingRelease case intentionally
		case types.Assigned:
			// This IP has already been assigned, if it is assigned to same pod add the IP to podIPInfo
			if ipConfig.PodInfo.Key() == podInfo.Key() {
				logger.Printf("[AssignDesiredIPConfigs]: IP Config [%+v] is already assigned to this Pod [%+v]", ipConfig, podInfo)
				if err := service.populateIPConfigInfoUntransacted(ipConfig, &podIPInfo[numIPConfigsAssigned]); err != nil {
					//nolint:goerr113 // return error
					return []cns.PodIpInfo{}, fmt.Errorf("[AssignDesiredIPConfigs] Failed to assign IP %+v requested for pod %+v since the IP is already assigned to %+v", ipConfig, podInfo, ipConfig.PodInfo)
				}
				numIPConfigsAssigned++
			} else {
				//nolint:goerr113 // return error
				return []cns.PodIpInfo{}, fmt.Errorf("[AssignDesiredIPConfigs] Desired IP is already assigned %+v, requested for pod %+v", ipConfig, podInfo)
			}
		case types.Available, types.PendingProgramming:
			// This race can happen during restart, where CNS state is lost and thus we have lost the NC programmed version
			// As part of reconcile, we mark IPs as Assigned which are already assigned to Pods (listed from APIServer)
			ipConfigsToAssign = append(ipConfigsToAssign, ipConfig)
		default:
			logger.Errorf("[AssignDesiredIPConfigs] Desired IP is not available %+v", ipConfig)
			//nolint:goerr113 // return error
			return podIPInfo, fmt.Errorf("IP not available")
		}

		// checks if found all of the desired IPs either as an available IP or already assigned to the pod
		if len(ipConfigsToAssign)+numIPConfigsAssigned == numDesiredIPAddresses {
			break
		}
	}

	// if we did not find all of the desired IPs return an error
	if len(ipConfigsToAssign)+numIPConfigsAssigned != numDesiredIPAddresses {
		//nolint:goerr113 // return error
		return podIPInfo, fmt.Errorf("not enough desired IPs found in pool")
	}

	failedToAssignIP := false
	// assigns all IPs that were found as available to the pod
	for i := range ipConfigsToAssign {
		if err := service.assignIPConfig(ipConfigsToAssign[i], podInfo); err != nil {
			logger.Errorf(err.Error())
			failedToAssignIP = true
			break
		}
		if err := service.populateIPConfigInfoUntransacted(ipConfigsToAssign[i], &podIPInfo[numIPConfigsAssigned]); err != nil {
			logger.Errorf(err.Error())
			failedToAssignIP = true
			break
		}
		// adds to the newly assigned IP to the counter
		numIPConfigsAssigned++
	}

	// if we were able to get at least one IP but not all of the desired IPs
	if failedToAssignIP {
		logger.Printf("[AssignDesiredIPConfigs] Failed to retrieve all desired IPs. Releasing all IPs that were found")
		for i := range ipConfigsToAssign {
			_, err := service.unassignIPConfig(ipConfigsToAssign[i], podInfo)
			if err != nil {
				logger.Errorf("[AssignDesiredIPConfigs] failed to mark IPConfig [%+v] back to Available. err: %v", ipConfigsToAssign[i], err)
			}
		}
		//nolint:goerr113 // return error
		return podIPInfo, fmt.Errorf("not all requested ips %v were found/available in the pool", desiredIPAddresses)
	}

	logger.Printf("[AssignDesiredIPConfigs] Successfully assigned all desired IPs for pod %+v", podInfo)
	return podIPInfo, nil
}

// Assigns an available IP from each NC on the NNC. If there is one NC then we expect to only have one IP return
// In the case of dualstack we would expect to have one IPv6 from one NC and one IPv4 from a second NC
func (service *HTTPRestService) AssignAvailableIPConfigs(podInfo cns.PodInfo) ([]cns.PodIpInfo, error) {
	// Gets the number of NCs which will determine the number of IPs given to a pod
	numOfNCs := len(service.state.ContainerStatus)
	// if there are no NCs on the NNC there will be no IPs in the pool so return error
	if numOfNCs == 0 {
		return nil, ErrNoNCs
	}

	//  Map used to get the number of IPFamilies across all NCs
	ncIPFamilies := map[cns.IPFamily]struct{}{}
	// Gets the IPFamilies from all NCs and store them in a map. This will be used to determine the number of IPs to return
	for ncID := range service.state.ContainerStatus {
		if len(ncIPFamilies) == 2 {
			break
		}

		for _, secIPConfig := range service.state.ContainerStatus[ncID].CreateNetworkContainerRequest.SecondaryIPConfigs {
			if len(ncIPFamilies) == 2 {
				break
			}

			ip := net.ParseIP(secIPConfig.IPAddress)
			if ip == nil {
				continue
			}

			if ip.To4() != nil {
				ncIPFamilies[cns.IPv4] = struct{}{}
			} else {
				ncIPFamilies[cns.IPv6] = struct{}{}
			}
		}
	}
	// Makes sure we have at least one IPFamily across all NCs
	numOfIPFamilies := len(ncIPFamilies)

	numberOfIPs := numOfNCs
	if numOfIPFamilies != 0 {
		numberOfIPs = numOfIPFamilies
	}

	service.Lock()
	defer service.Unlock()
	// Creates a slice of PodIpInfo with the size as number of NCs to hold the result for assigned IP configs
	podIPInfo := make([]cns.PodIpInfo, numberOfIPs)
	// This map is used to store whether or not we have found an available IP from an NC when looping through the pool
	ipsToAssign := make(map[string]cns.IPConfigurationStatus)

	// Searches for available IPs in the pool
	for _, ipState := range service.PodIPConfigState {

		// get the IPFamily of the current ipState
		var ipStateFamily cns.IPFamily
		if net.ParseIP(ipState.IPAddress).To4() != nil {
			ipStateFamily = cns.IPv4
		} else {
			ipStateFamily = cns.IPv6
		}

		key := generateAssignedIPKey(ipState.NCID, ipStateFamily)

		// check if the IP with the same family type exists already
		if _, ncIPFamilyAlreadyMarkedForAssignment := ipsToAssign[key]; ncIPFamilyAlreadyMarkedForAssignment {
			continue
		}
		// Checks if the current IP is available
		if ipState.GetState() != types.Available {
			continue
		}
		ipsToAssign[key] = ipState
		// Once numberOfIPs per container is found break out of the loop and stop searching
		if len(ipsToAssign) == numberOfIPs {
			break
		}
	}

	// Checks to make sure we found one IP for each NCxIPFamily
	if len(ipsToAssign) != numberOfIPs {
		for ncID := range service.state.ContainerStatus {
			for ipFamily := range ncIPFamilies {
				if _, found := ipsToAssign[generateAssignedIPKey(ncID, ipFamily)]; found {
					continue
				}
				return podIPInfo, errors.Errorf("not enough IPs available of type %s for %s, waiting on Azure CNS to allocate more with NC Status: %s",
					ipFamily, ncID, string(service.state.ContainerStatus[ncID].CreateNetworkContainerRequest.NCStatus))
			}
		}
	}

	failedToAssignIP := false
	numIPConfigsAssigned := 0
	// assigns all IPs in the map to the pod
	for _, ip := range ipsToAssign { //nolint:gocritic // ignore copy
		if err := service.assignIPConfig(ip, podInfo); err != nil {
			logger.Errorf(err.Error())
			failedToAssignIP = true
			break
		}

		if err := service.populateIPConfigInfoUntransacted(ip, &podIPInfo[numIPConfigsAssigned]); err != nil {
			logger.Errorf(err.Error())
			failedToAssignIP = true
			break
		}
		numIPConfigsAssigned++
	}

	// if we were able to find at least one IP but not enough
	if failedToAssignIP {
		logger.Printf("[AssignAvailableIPConfigs] failed to assign enough IPs. Releasing all IPs that were found")
		for _, ipState := range ipsToAssign { //nolint:gocritic // ignore copy
			_, err := service.unassignIPConfig(ipState, podInfo)
			if err != nil {
				logger.Errorf("[AssignAvailableIPConfigs] failed to mark IPConfig [%+v] back to Available. err: %v", ipState, err)
			}
		}
		//nolint:goerr113 // return error
		return podIPInfo, fmt.Errorf("not enough IPs available, waiting on Azure CNS to allocate more")
	}

	logger.Printf("[AssignAvailableIPConfigs] Successfully assigned IPs for pod %+v", podInfo)
	return podIPInfo, nil
}

func generateAssignedIPKey(ncID string, ipFamily cns.IPFamily) string {
	return fmt.Sprintf("%s_%s", ncID, string(ipFamily))
}

// If IPConfigs are already assigned to the pod, it returns that else it returns the available ipconfigs.
func requestIPConfigsHelper(service *HTTPRestService, req cns.IPConfigsRequest) ([]cns.PodIpInfo, error) {
	// check if ipconfigs already assigned to this pod and return if exists or error
	// if error, ipstate is nil, if exists, ipstate is not nil and error is nil
	podInfo, err := cns.NewPodInfoFromIPConfigsRequest(req)
	if err != nil {
		return []cns.PodIpInfo{}, errors.Wrapf(err, "failed to parse IPConfigsRequest %v", req)
	}

	if podIPInfo, isExist, err := service.GetExistingIPConfig(podInfo); err != nil || isExist {
		return podIPInfo, err
	}

	// if the desired IP configs are not specified, assign any free IPConfigs
	if len(req.DesiredIPAddresses) == 0 {
		return service.AssignAvailableIPConfigs(podInfo)
	}

	if err := validateDesiredIPAddresses(req.DesiredIPAddresses); err != nil {
		return []cns.PodIpInfo{}, err
	}

	return service.AssignDesiredIPConfigs(podInfo, req.DesiredIPAddresses)
}

// checks all desired IPs for a request to make sure they are all valid
func validateDesiredIPAddresses(desiredIPs []string) error {
	for _, desiredIP := range desiredIPs {
		ip := net.ParseIP(desiredIP)
		if ip.To4() == nil && ip.To16() == nil {
			//nolint:goerr113 // return error
			return fmt.Errorf("[validateDesiredIPAddresses] invalid ip %s specified as desired IP", desiredIP)
		}
	}
	return nil
}

// EndpointHandlerAPI forwards the endpoint related APIs to GetEndpointHandler or UpdateEndpointHandler based on the http method
func (service *HTTPRestService) EndpointHandlerAPI(w http.ResponseWriter, r *http.Request) {
	opName := "endpointHandler"
	logger.Printf("[EndpointHandlerAPI] EndpointHandlerAPI received request with http Method %s", r.Method)
	service.Lock()
	defer service.Unlock()
	// Check if CNS is managing the CNI statefile
	if service.Options[common.OptManageEndpointState] == false {
		response := cns.Response{
			ReturnCode: types.UnexpectedError,
			Message:    fmt.Sprintf("[EndpointHandlerAPI] EndpointHandlerAPI failed with error: %s", ErrOptManageEndpointState),
		}
		err := common.Encode(w, &response)
		logger.Response(opName, response, response.ReturnCode, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		service.GetEndpointHandler(w, r)
	case http.MethodPatch:
		service.UpdateEndpointHandler(w, r)
	default:
		logger.Errorf("[EndpointHandlerAPI] EndpointHandler API expect http Get or Patch method")
	}
}

// GetEndpointHandler handles the incoming GetEndpoint requests with http Get method
func (service *HTTPRestService) GetEndpointHandler(w http.ResponseWriter, r *http.Request) {
	opName := "getEndpointState"
	logger.Printf("[GetEndpointState] GetEndpoint for %s", r.URL.Path)
	endpointID := strings.TrimPrefix(r.URL.Path, cns.EndpointPath)
	endpointInfo, err := service.GetEndpointHelper(endpointID)
	// Check if the request is valid
	if err != nil {
		response := GetEndpointResponse{
			Response: Response{
				ReturnCode: types.UnexpectedError,
				Message:    fmt.Sprintf("[GetEndpointState] GetEndpoint failed with error: %s", err.Error()),
			},
		}
		if errors.Is(err, ErrEndpointStateNotFound) {
			response = GetEndpointResponse{
				Response: Response{
					ReturnCode: types.NotFound,
					Message:    fmt.Sprintf("[GetEndpointState] %s", err.Error()),
				},
			}
		}
		w.Header().Set(cnsReturnCode, response.Response.ReturnCode.String())
		err = common.Encode(w, &response)
		logger.Response(opName, response, response.Response.ReturnCode, err)
		return
	}
	response := GetEndpointResponse{
		Response: Response{
			ReturnCode: types.Success,
			Message:    "[GetEndpointState] GetEndpoint retruned successfully",
		},
		EndpointInfo: *endpointInfo,
	}
	w.Header().Set(cnsReturnCode, response.Response.ReturnCode.String())
	err = common.Encode(w, &response)
	logger.Response(opName, response, response.Response.ReturnCode, err)
}

// GetEndpointHelper returns the state of the given endpointId
func (service *HTTPRestService) GetEndpointHelper(endpointID string) (*EndpointInfo, error) {
	logger.Printf("[GetEndpointState] Get endpoint state for infra container %s", endpointID)

	// Skip if a store is not provided.
	if service.EndpointStateStore == nil {
		logger.Printf("[GetEndpointState]  store not initialized.")
		return nil, ErrStoreEmpty
	}

	err := service.EndpointStateStore.Read(EndpointStoreKey, &service.EndpointState)
	if err != nil {

		if errors.Is(err, store.ErrKeyNotFound) {
			// Nothing to retrieve.
			logger.Printf("[GetEndpointState]  No endpoint state to retrieve.\n")
		} else {
			logger.Errorf("[GetEndpointState]  Failed to retrieve state, err:%v", err)
		}
		return nil, ErrEndpointStateNotFound
	}
	if endpointInfo, ok := service.EndpointState[endpointID]; ok {
		logger.Warnf("[GetEndpointState] Found existing endpoint state for container %s", endpointID)
		return endpointInfo, nil
	}
	// This part is a temprory fix if we have endpoint states belong to CNI version 1.4.X on Windows since the states don't have the containerID
	// In case there was no endpoint founded with ContainerID as the key,
	// then [First 8 character of containerid]-eth0 will be tried
	legacyEndpointID := endpointID[:ContainerIDLength] + "-" + InfraInterfaceName
	if endpointInfo, ok := service.EndpointState[legacyEndpointID]; ok {
		logger.Warnf("[GetEndpointState] Found existing endpoint state for container %s", legacyEndpointID)
		return endpointInfo, nil
	}
	return nil, ErrEndpointStateNotFound
}

// UpdateEndpointHandler handles the incoming UpdateEndpoint requests with http Patch method
func (service *HTTPRestService) UpdateEndpointHandler(w http.ResponseWriter, r *http.Request) {
	opName := "UpdateEndpointHandler"
	logger.Printf("[updateEndpoint] updateEndpoint for %s", r.URL.Path)

	var req map[string]*IPInfo
	err := common.Decode(w, r, &req)
	endpointID := strings.TrimPrefix(r.URL.Path, cns.EndpointPath)
	logger.Request(opName, &req, err)
	// Check if the request is valid
	if err != nil {
		response := cns.Response{
			ReturnCode: types.InvalidRequest,
			Message:    fmt.Sprintf("[updateEndpoint] updateEndpoint failed with error: %s", err.Error()),
		}
		w.Header().Set(cnsReturnCode, response.ReturnCode.String())
		err = common.Encode(w, &response)
		logger.Response(opName, response, response.ReturnCode, err)
		return
	}
	if err = verifyUpdateEndpointStateRequest(req); err != nil {
		response := cns.Response{
			ReturnCode: types.InvalidRequest,
			Message:    err.Error(),
		}
		w.Header().Set(cnsReturnCode, response.ReturnCode.String())
		err = common.Encode(w, &response)
		logger.Response(opName, response, response.ReturnCode, err)
		return
	}
	// Update the endpoint state
	err = service.UpdateEndpointHelper(endpointID, req)
	if err != nil {
		response := cns.Response{
			ReturnCode: types.UnexpectedError,
			Message:    fmt.Sprintf("[updateEndpoint] updateEndpoint failed with error: %s", err.Error()),
		}
		w.Header().Set(cnsReturnCode, response.ReturnCode.String())
		err = common.Encode(w, &response)
		logger.Response(opName, response, response.ReturnCode, err)
		return
	}
	response := cns.Response{
		ReturnCode: types.Success,
		Message:    "[updateEndpoint] updateEndpoint retruned successfully",
	}
	w.Header().Set(cnsReturnCode, response.ReturnCode.String())
	err = common.Encode(w, &response)
	logger.Response(opName, response, response.ReturnCode, err)
}

// UpdateEndpointHelper updates the state of the given endpointId with HNSId, VethName or other InterfaceInfo fields
func (service *HTTPRestService) UpdateEndpointHelper(endpointID string, req map[string]*IPInfo) error {
	if service.EndpointStateStore == nil {
		return ErrStoreEmpty
	}
	logger.Printf("[updateEndpoint] Updating endpoint state for infra container %s", endpointID)
	endpointInfo, endpointExist := service.EndpointState[endpointID]
	// create a new entry in case the ednpoint does not exist in the statefile.
	// this applies to the ACI scenario when the endpoint is not added to the statefile when the goalstate is sent to CNI
	if !endpointExist {
		logger.Printf("[updateEndpoint] endpoint could not be found in the statefile %s, new entry is being added", endpointID)
		endpointInfo = &EndpointInfo{PodName: "", PodNamespace: "", IfnameToIPMap: make(map[string]*IPInfo)}
		service.EndpointState[endpointID] = endpointInfo
	}
	// updating the InterfaceInfo map of endpoint states with the interfaceInfo map that is given by Stateless Azure CNI
	for ifName, interfaceInfo := range req {
		// updating the ipInfoMap
		updateIPInfoMap(endpointInfo.IfnameToIPMap, interfaceInfo, ifName, endpointID)
	}
	err := service.EndpointStateStore.Write(EndpointStoreKey, service.EndpointState)
	if err != nil {
		return fmt.Errorf("[updateEndpoint] failed to write endpoint state to store for pod %s :  %w", endpointInfo.PodName, err)
	}
	logger.Printf("[updateEndpoint] successfully write the state to the file %s", endpointID)
	return nil
}

// updateIPInfoMap updates the IfnameToIPMap of endpoint states with the interfaceInfo map that is given by Stateless Azure CNI
func updateIPInfoMap(iPInfo map[string]*IPInfo, interfaceInfo *IPInfo, ifName, endpointID string) {
	// This codition will create a map for SecodaryNIC and also also creates MAP entry for InfraNic in case that the initial goalState is using empty InterfaceName
	if _, keyExist := iPInfo[ifName]; !keyExist {
		iPInfo[ifName] = &IPInfo{}
		if val, emptyKeyExist := iPInfo[""]; emptyKeyExist && ifName == InfraInterfaceName {
			iPInfo[ifName].IPv4 = val.IPv4
			iPInfo[ifName].IPv6 = val.IPv6
			delete(iPInfo, "")
		}
	}
	logger.Printf("[updateEndpoint] Found existing endpoint state for infra container %s with  %s , [%+v]", endpointID, ifName, interfaceInfo)
	if interfaceInfo.HnsEndpointID != "" {
		iPInfo[ifName].HnsEndpointID = interfaceInfo.HnsEndpointID
		logger.Printf("[updateEndpoint] update the endpoint %s with HNSID  %s", endpointID, interfaceInfo.HnsEndpointID)
	}
	if interfaceInfo.HnsNetworkID != "" {
		iPInfo[ifName].HnsNetworkID = interfaceInfo.HnsNetworkID
		logger.Printf("[updateEndpoint] update the endpoint %s with HnsNetworkID  %s", endpointID, interfaceInfo.HnsEndpointID)
	}
	if interfaceInfo.HostVethName != "" {
		iPInfo[ifName].HostVethName = interfaceInfo.HostVethName
		logger.Printf("[updateEndpoint] update the endpoint %s with vethName  %s", endpointID, interfaceInfo.HostVethName)
	}
	if interfaceInfo.NICType != "" {
		iPInfo[ifName].NICType = interfaceInfo.NICType
		logger.Printf("[updateEndpoint] update the endpoint %s with NICType  %s", endpointID, interfaceInfo.NICType)
	}
	if interfaceInfo.MacAddress != "" {
		iPInfo[ifName].MacAddress = interfaceInfo.MacAddress
		logger.Printf("[updateEndpoint] update the endpoint %s with MacAddress  %s", endpointID, interfaceInfo.MacAddress)
	}
}

// verifyUpdateEndpointStateRequest verify the CNI request body for the UpdateENdpointState API
func verifyUpdateEndpointStateRequest(req map[string]*IPInfo) error {
	for ifName, InterfaceInfo := range req {
		if InterfaceInfo.HostVethName == "" && InterfaceInfo.HnsEndpointID == "" && InterfaceInfo.NICType == "" && InterfaceInfo.MacAddress == "" {
			return errors.New("[updateEndpoint] No NicType, MacAddress, HnsEndpointID or HostVethName has been provided")
		}
		if ifName == "" {
			return errors.New("[updateEndpoint] No Interface has been provided")
		}
	}
	return nil
}
