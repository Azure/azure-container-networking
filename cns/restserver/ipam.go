// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/filter"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/common"
	"github.com/pkg/errors"
)

func (service *HTTPRestService) requestIPConfigHandlerHelper(ipconfigsRequest cns.IPConfigsRequest) (*cns.IPConfigsResponse, error) {
	podInfo, returnCode, returnMessage := service.validateIPConfigsRequest(ipconfigsRequest)
	if returnCode != types.Success {
		return &cns.IPConfigsResponse{
			Response: cns.Response{
				ReturnCode: returnCode,
				Message:    returnMessage,
			},
		}, errors.New("failed to validate ip config request")
	}

	// record a pod requesting an IP
	service.podsPendingIPAssignment.Push(podInfo.Key())

	podIPInfo, err := requestIPConfigHelper(service, ipconfigsRequest)
	if err != nil {
		return &cns.IPConfigsResponse{
			Response: cns.Response{
				ReturnCode: types.FailedToAllocateIPConfig,
				Message:    fmt.Sprintf("AllocateIPConfig failed: %v, IP config request is %s", err, ipconfigsRequest),
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

	return &cns.IPConfigsResponse{
		Response: cns.Response{
			ReturnCode: types.Success,
		},
		PodIPInfo: podIPInfo,
	}, nil
}

// requestIPConfigHandler requests an IPConfig from the CNS state
func (service *HTTPRestService) requestIPConfigHandler(w http.ResponseWriter, r *http.Request) {
	var ipconfigRequest cns.IPConfigRequest
	err := service.Listener.Decode(w, r, &ipconfigRequest)
	operationName := "requestIPConfigHandler"
	logger.Request(service.Name+operationName, ipconfigRequest, err)
	if err != nil {
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

	ipConfigsResp, err := service.requestIPConfigHandlerHelper(ipconfigsRequest) //nolint:contextcheck // appease linter
	if err != nil {
		// As this API is expected to return IPConfigResponse, generate it from the IPConfigsResponse returned above
		reserveResp := &cns.IPConfigResponse{
			Response: ipConfigsResp.Response,
		}
		w.Header().Set(cnsReturnCode, reserveResp.Response.ReturnCode.String())
		err = service.Listener.Encode(w, &reserveResp)
		logger.ResponseEx(service.Name+operationName, ipconfigsRequest, reserveResp, reserveResp.Response.ReturnCode, err)
		return
	}

	// As this API is expected to return IPConfigResponse, generate it from the IPConfigsResponse returned above.
	// Only single value of PodIpInfo is expected to be returned
	reserveResp := &cns.IPConfigResponse{
		Response:  ipConfigsResp.Response,
		PodIpInfo: ipConfigsResp.PodIPInfo[0],
	}
	w.Header().Set(cnsReturnCode, reserveResp.Response.ReturnCode.String())
	err = service.Listener.Encode(w, &reserveResp)
	logger.ResponseEx(service.Name+operationName, ipconfigsRequest, reserveResp, reserveResp.Response.ReturnCode, err)
}

// requestIPConfigsHandler requests multiple IPConfigs from the CNS state
func (service *HTTPRestService) requestIPConfigsHandler(w http.ResponseWriter, r *http.Request) {
	var ipconfigsRequest cns.IPConfigsRequest
	err := service.Listener.Decode(w, r, &ipconfigsRequest)
	operationName := "requestIPConfigsHandler"
	logger.Request(service.Name+operationName, ipconfigsRequest, err)
	if err != nil {
		return
	}

	ipConfigsResp, err := service.requestIPConfigHandlerHelper(ipconfigsRequest) // nolint:contextcheck // appease linter
	if err != nil {
		w.Header().Set(cnsReturnCode, ipConfigsResp.Response.ReturnCode.String())
		err = service.Listener.Encode(w, &ipConfigsResp)
		logger.ResponseEx(service.Name+operationName, ipconfigsRequest, ipConfigsResp, ipConfigsResp.Response.ReturnCode, err)
		return
	}

	w.Header().Set(cnsReturnCode, ipConfigsResp.Response.ReturnCode.String())
	err = service.Listener.Encode(w, &ipConfigsResp)
	logger.ResponseEx(service.Name+operationName, ipconfigsRequest, ipConfigsResp, ipConfigsResp.Response.ReturnCode, err)
}

var (
	errStoreEmpty       = errors.New("empty endpoint state store")
	errParsePodIPFailed = errors.New("failed to parse pod's ip")
)

func (service *HTTPRestService) updateEndpointState(ipconfigsRequest cns.IPConfigsRequest, podInfo cns.PodInfo, podIPInfo []cns.PodIpInfo) error {
	if service.EndpointStateStore == nil {
		return errStoreEmpty
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
				return errParsePodIPFailed
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
				return errParsePodIPFailed
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

func (service *HTTPRestService) releaseIPConfigHandlerHelper(ipconfigsRequest cns.IPConfigsRequest) (*cns.Response, error) {
	podInfo, returnCode, returnMessage := service.validateIPConfigsRequest(ipconfigsRequest)
	if returnCode != types.Success {
		return &cns.Response{
			ReturnCode: returnCode,
			Message:    returnMessage,
		}, fmt.Errorf("failed to validate ip config request") //nolint:goerr113 // return error
	}
	// Check if http rest service managed endpoint state is set
	if service.Options[common.OptManageEndpointState] == true {
		if err := service.removeEndpointState(podInfo); err != nil {
			resp := &cns.Response{
				ReturnCode: types.UnexpectedError,
				Message:    err.Error(),
			}
			return resp, fmt.Errorf("releaseIPConfigHandlerHelper remove endpoint state failed because %v, release IP config info %s", resp.Message, ipconfigsRequest) //nolint:goerr113 // return error
		}
	}

	if err := service.releaseIPConfig(podInfo); err != nil {
		return &cns.Response{
			ReturnCode: types.UnexpectedError,
			Message:    err.Error(),
		}, fmt.Errorf("releaseIPConfigHandler releaseIPConfig failed because %v, release IP config info %s", returnMessage, ipconfigsRequest) //nolint:goerr113 // return error
	}

	return &cns.Response{
		ReturnCode: types.Success,
		Message:    "",
	}, nil
}

func (service *HTTPRestService) releaseIPConfigHandler(w http.ResponseWriter, r *http.Request) {
	var ipconfigRequest cns.IPConfigRequest
	err := service.Listener.Decode(w, r, &ipconfigRequest)
	logger.Request(service.Name+"releaseIPConfigHandler", ipconfigRequest, err)
	if err != nil {
		resp := cns.Response{
			ReturnCode: types.UnexpectedError,
			Message:    err.Error(),
		}
		logger.Errorf("releaseIPConfigHandler decode failed becase %v, release IP config info %s", resp.Message, ipconfigRequest)
		w.Header().Set(cnsReturnCode, resp.ReturnCode.String())
		err = service.Listener.Encode(w, &resp)
		logger.ResponseEx(service.Name, ipconfigRequest, resp, resp.ReturnCode, err)
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

	resp, err := service.releaseIPConfigHandlerHelper(ipconfigsRequest)
	if err != nil {
		w.Header().Set(cnsReturnCode, resp.ReturnCode.String())
		err = service.Listener.Encode(w, &resp)
		logger.ResponseEx(service.Name, ipconfigRequest, resp, resp.ReturnCode, err)
	}

	w.Header().Set(cnsReturnCode, resp.ReturnCode.String())
	err = service.Listener.Encode(w, &resp)
	logger.ResponseEx(service.Name, ipconfigRequest, resp, resp.ReturnCode, err)
}

func (service *HTTPRestService) releaseIPConfigsHandler(w http.ResponseWriter, r *http.Request) {
	var ipconfigsRequest cns.IPConfigsRequest
	err := service.Listener.Decode(w, r, &ipconfigsRequest)
	logger.Request(service.Name+"releaseIPConfigsHandler", ipconfigsRequest, err)
	if err != nil {
		resp := cns.Response{
			ReturnCode: types.UnexpectedError,
			Message:    err.Error(),
		}
		logger.Errorf("releaseIPConfigsHandler decode failed because %v, release IP config info %s", resp.Message, ipconfigsRequest)
		w.Header().Set(cnsReturnCode, resp.ReturnCode.String())
		err = service.Listener.Encode(w, &resp)
		logger.ResponseEx(service.Name, ipconfigsRequest, resp, resp.ReturnCode, err)
		return
	}

	resp, err := service.releaseIPConfigHandlerHelper(ipconfigsRequest)
	if err != nil {
		w.Header().Set(cnsReturnCode, resp.ReturnCode.String())
		err = service.Listener.Encode(w, &resp)
		logger.ResponseEx(service.Name, ipconfigsRequest, resp, resp.ReturnCode, err)
	}

	w.Header().Set(cnsReturnCode, resp.ReturnCode.String())
	err = service.Listener.Encode(w, &resp)
	logger.ResponseEx(service.Name, ipconfigsRequest, resp, resp.ReturnCode, err)
}

func (service *HTTPRestService) removeEndpointState(podInfo cns.PodInfo) error {
	if service.EndpointStateStore == nil {
		return errStoreEmpty
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

func (service *HTTPRestService) updateIPConfigState(ipID string, updatedState types.IPState, podInfo cns.PodInfo) (cns.IPConfigurationStatus, error) {
	if ipConfig, found := service.PodIPConfigState[ipID]; found {
		logger.Printf("[updateIPConfigState] Changing IpId [%s] state to [%s], podInfo [%+v]. Current config [%+v]", ipID, updatedState, podInfo, ipConfig)
		ipConfig.SetState(updatedState)
		ipConfig.PodInfo = podInfo
		service.PodIPConfigState[ipID] = ipConfig
		return ipConfig, nil
	}

	//nolint:goerr113
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

func (service *HTTPRestService) handleDebugPodContext(w http.ResponseWriter, r *http.Request) {
	service.RLock()
	defer service.RUnlock()
	resp := cns.GetPodContextResponse{
		PodContext: service.PodIPIDByPodInterfaceKey,
	}
	err := service.Listener.Encode(w, &resp)
	logger.Response(service.Name, resp, resp.Response.ReturnCode, err)
}

func (service *HTTPRestService) handleDebugRestData(w http.ResponseWriter, r *http.Request) {
	service.RLock()
	defer service.RUnlock()
	resp := GetHTTPServiceDataResponse{
		HTTPRestServiceData: HTTPRestServiceData{
			PodIPIDByPodInterfaceKey: service.PodIPIDByPodInterfaceKey,
			PodIPConfigState:         service.PodIPConfigState,
			IPAMPoolMonitor:          service.IPAMPoolMonitor.GetStateSnapshot(),
		},
	}
	err := service.Listener.Encode(w, &resp)
	logger.Response(service.Name, resp, resp.Response.ReturnCode, err)
}

func (service *HTTPRestService) handleDebugIPAddresses(w http.ResponseWriter, r *http.Request) {
	var req cns.GetIPAddressesRequest
	if err := service.Listener.Decode(w, r, &req); err != nil {
		resp := cns.GetIPAddressStatusResponse{
			Response: cns.Response{
				ReturnCode: types.UnexpectedError,
				Message:    err.Error(),
			},
		}
		err = service.Listener.Encode(w, &resp)
		logger.ResponseEx(service.Name, req, resp, resp.Response.ReturnCode, err)
		return
	}
	// Get all IPConfigs matching a state and return in the response
	resp := cns.GetIPAddressStatusResponse{
		IPConfigurationStatus: filter.MatchAnyIPConfigState(service.PodIPConfigState, filter.PredicatesForStates(req.IPConfigStateFilter...)...),
	}
	err := service.Listener.Encode(w, &resp)
	logger.ResponseEx(service.Name, req, resp, resp.Response.ReturnCode, err)
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
		logger.Printf("IP config initialized")
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
func (service *HTTPRestService) releaseIPConfig(podInfo cns.PodInfo) error {
	service.Lock()
	defer service.Unlock()
	ipsReleased := make([]cns.IPConfigurationStatus, len(service.PodIPIDByPodInterfaceKey[podInfo.Key()]))

	for i, ipID := range service.PodIPIDByPodInterfaceKey[podInfo.Key()] {
		if ipID != "" {
			if ipconfig, isExist := service.PodIPConfigState[ipID]; isExist {
				logger.Printf("[releaseIPConfig] Releasing IP %+v for pod %+v", ipconfig.IPAddress, podInfo)
				_, err := service.unassignIPConfig(ipconfig, podInfo)
				if err != nil {
					return fmt.Errorf("[releaseIPConfig] failed to mark IPConfig [%+v] as Available. err: %w", ipconfig, err)
				}
				ipsReleased[i] = ipconfig
				logger.Printf("[releaseIPConfig] Released IP %+v for pod %+v", ipconfig.IPAddress, podInfo)
				if i == len(ipsReleased)-1 {
					return nil
				}
			} else {
				logger.Errorf("[releaseIPConfig] Failed to get release ipconfig %+v and pod info is %+v. Pod to IPID exists, but IPID to IPConfig doesn't exist, CNS State potentially corrupt",
					ipconfig.IPAddress, podInfo)
				//nolint:goerr113 // return error
				return fmt.Errorf("[releaseIPConfig] releaseIPConfig failed. IPconfig %+v and pod info is %+v. Pod to IPID exists, but IPID to IPConfig doesn't exist, CNS State potentially corrupt",
					ipconfig.IPAddress, podInfo)
			}
		} else {
			logger.Errorf("[releaseIPConfig] SetIPConfigAsAvailable ignoring request to release, no allocation found for pod [%+v]", podInfo)
			break
		}
	}

	// if we were able to get at least one IP but not all of the desired IPs
	if len(ipsReleased) > 0 {
		logger.Printf("[releaseIPConfig] Failed to release all desired IPs. Reassigning all IPs that weren't released")
		for i := range ipsReleased {
			err := service.assignIPConfig(ipsReleased[i], podInfo)
			if err != nil {
				return fmt.Errorf("[releaseIPConfig] failed to mark IPConfig [%+v] back to Assigned. err: %w", ipsReleased[i], err)
			}
		}
	}

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

func (service *HTTPRestService) GetExistingIPConfig(podInfo cns.PodInfo) ([]cns.PodIpInfo, bool, error) {
	podIPInfo := make([]cns.PodIpInfo, len(service.PodIPIDByPodInterfaceKey[podInfo.Key()]))

	service.RLock()
	defer service.RUnlock()

	for i, ipID := range service.PodIPIDByPodInterfaceKey[podInfo.Key()] {
		if ipID != "" {
			if ipState, isExist := service.PodIPConfigState[ipID]; isExist {
				err := service.populateIPConfigInfoUntransacted(ipState, &podIPInfo[i])
				if i == len(service.PodIPIDByPodInterfaceKey[podInfo.Key()])-1 {
					return podIPInfo, isExist, err
				}
			} else {
				logger.Errorf("Failed to get existing ipconfig. Pod to IPID exists, but IPID to IPConfig doesn't exist, CNS State potentially corrupt")
				//nolint:goerr113 // return error
				return podIPInfo, false, fmt.Errorf("failed to get existing ipconfig. Pod to IPID exists, but IPID to IPConfig doesn't exist, CNS State potentially corrupt")
			}
		}
	}

	return podIPInfo, false, nil
}

// Assigns a pod with all IPs desired
func (service *HTTPRestService) AssignDesiredIPConfigs(podInfo cns.PodInfo, desiredIPAddresses []string) ([]cns.PodIpInfo, error) {
	podIPInfo := make([]cns.PodIpInfo, len(desiredIPAddresses))
	service.Lock()
	defer service.Unlock()
	// creating a map for the loop to check to see if the IP in the pool is one of the desired IPs
	IPMap := make(map[string]string)
	for _, desiredIP := range desiredIPAddresses {
		IPMap[desiredIP] = desiredIP
	}
	ncMap := make(map[string]cns.IPConfigurationStatus)
	assignedMap := make(map[string]cns.IPConfigurationStatus)

forLoop:
	for _, ipConfig := range service.PodIPConfigState { //nolint:gocritic // ignore copy
		if _, found := IPMap[ipConfig.IPAddress]; found {
			switch ipConfig.GetState() { //nolint:exhaustive // ignoring PendingRelease case intentionally
			case types.Assigned:
				// This IP has already been assigned, if it is assigned to same pod, then return the same
				// IPconfiguration
				if ipConfig.PodInfo.Key() == podInfo.Key() {
					logger.Printf("[AssignDesiredIPConfigs]: IP Config [%+v] is already assigned to this Pod [%+v]", ipConfig, podInfo)
					assignedMap[ipConfig.NCID] = ipConfig
					err := service.populateIPConfigInfoUntransacted(ipConfig, &podIPInfo[len(assignedMap)])
					if len(assignedMap) == len(desiredIPAddresses) {
						return podIPInfo, err
					}
				} else {
					logger.Errorf("[AssignDesiredIPConfigs] Desired IP is already assigned %+v, requested for pod %+v", ipConfig, podInfo)
					break forLoop
				}
			case types.Available, types.PendingProgramming:
				// This race can happen during restart, where CNS state is lost and thus we have lost the NC programmed version
				// As part of reconcile, we mark IPs as Assigned which are already assigned to Pods (listed from APIServer)
				if err := service.assignIPConfig(ipConfig, podInfo); err != nil {
					logger.Errorf(err.Error())
					break forLoop
				}
			default:
				logger.Errorf("[AssignDesiredIPConfigs] Desired IP is not available %+v", ipConfig)
				break forLoop
			}
			err := service.populateIPConfigInfoUntransacted(ipConfig, &podIPInfo[len(ncMap)])
			ncMap[ipConfig.NCID] = ipConfig
			if len(ncMap) == len(desiredIPAddresses) {
				return podIPInfo, err
			}
		}
	}

	// if we were able to get at least one IP but not all of the desired IPs
	if len(ncMap) > 0 {
		logger.Printf("[AssignDesiredIPConfigs] Failed to retrieve all desired IPs. Releasing all IPs that were found")
		for _, ipState := range ncMap {
			_, err := service.unassignIPConfig(ipState, podInfo)
			if err != nil {
				return podIPInfo, fmt.Errorf("[AssignDesiredIPConfigs] failed to mark IPConfig [%+v] back to Available. err: %w", ipState, err)
			}
		}
	}
	//nolint:goerr113 // return error
	return podIPInfo, fmt.Errorf("Not all requested ips were found/available in the pool")
}

// Assigns an IP from each NC on the NNC
func (service *HTTPRestService) AssignAvailableIPConfigs(podInfo cns.PodInfo) ([]cns.PodIpInfo, error) {
	service.Lock()
	defer service.Unlock()
	// Creates a slice of PoPodIpInfo with the size of number of NCs
	podIPInfo := make([]cns.PodIpInfo, len(service.state.ContainerStatus))
	// This map is used to store whether or not we have found an available IP from an NC when looping through the pool
	ncMap := make(map[string]cns.IPConfigurationStatus)

	for _, ipState := range service.PodIPConfigState {
		_, found := ncMap[ipState.NCID]
		// Checks if the current IP is available and if we haven't already found an IP from that NC
		if !found && ipState.GetState() == types.Available {
			if err := service.assignIPConfig(ipState, podInfo); err != nil {
				logger.Errorf(err.Error())
				break
			}

			if err := service.populateIPConfigInfoUntransacted(ipState, &podIPInfo[len(ncMap)]); err != nil {
				logger.Errorf(err.Error())
				break
			}
			ncMap[ipState.NCID] = ipState
			// return once we have found one IP per NC
			if len(ncMap) == len(service.state.ContainerStatus) {
				return podIPInfo, nil
			}
		}
	}

	// if we were able to find at least one IP but not enough
	if len(ncMap) > 0 {
		logger.Printf("[AssignAvailableIPConfigs] Failed to retrieve enough IPs. Releasing all IPs that were found")
		for _, ipState := range ncMap {
			_, err := service.unassignIPConfig(ipState, podInfo)
			if err != nil {
				return podIPInfo, fmt.Errorf("[AssignAvailableIPConfigs] failed to mark IPConfig [%+v] back to Available. err: %w", ipState, err)
			}
		}
	}
	//nolint:goerr113
	return podIPInfo, fmt.Errorf("not enough IPs available, waiting on Azure CNS to allocate more")
}

// If IPConfig is already assigned to pod, it returns that else it returns one of the available ipconfigs.
func requestIPConfigHelper(service *HTTPRestService, req cns.IPConfigsRequest) ([]cns.PodIpInfo, error) {
	// check if ipconfig already assigned tothis pod and return if exists or error
	// if error, ipstate is nil, if exists, ipstate is not nil and error is nil
	podInfo, err := cns.NewPodInfoFromIPConfigRequest(req)
	if err != nil {
		return []cns.PodIpInfo{}, errors.Wrapf(err, "failed to parse IPConfigsRequest %v", req)
	}

	if podIPInfo, isExist, err := service.GetExistingIPConfig(podInfo); err != nil || isExist {
		return podIPInfo, err
	}

	// return desired IPConfig
	if req.DesiredIPAddresses != nil && len(req.DesiredIPAddresses) != 0 {
		return service.AssignDesiredIPConfigs(podInfo, req.DesiredIPAddresses)
	}

	// return any free IPConfig
	return service.AssignAvailableIPConfigs(podInfo)
	// TODO: create a check for returning a slice of PodIpInfo that is not full (i.e. slice of size two with only 1 IP)
	// This is to ensure that we don't end up having IPs listed as assigned in IPAM that aren't actually being used by the container.
}
