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

// used to request an IPConfig from the CNS state
func (service *HTTPRestService) requestIPConfigHandler(w http.ResponseWriter, r *http.Request) {
	var ipconfigRequest cns.IPConfigRequest
	err := service.Listener.Decode(w, r, &ipconfigRequest)
	operationName := "requestIPConfigHandler"
	logger.Request(service.Name+operationName, ipconfigRequest, err)
	if err != nil {
		return
	}

	// retrieve ipconfig from nc
	podInfo, returnCode, returnMessage := service.validateIPConfigRequest(ipconfigRequest)
	if returnCode != types.Success {
		reserveResp := &cns.IPConfigResponse{
			Response: cns.Response{
				ReturnCode: returnCode,
				Message:    returnMessage,
			},
		}
		w.Header().Set(cnsReturnCode, reserveResp.Response.ReturnCode.String())
		err = service.Listener.Encode(w, &reserveResp)
		logger.ResponseEx(service.Name+operationName, ipconfigRequest, reserveResp, reserveResp.Response.ReturnCode, err)
		return
	}

	// record a pod requesting an IP
	service.podsPendingIPAssignment.Push(podInfo.Key())

	podIPInfo, err := requestIPConfigHelper(service, ipconfigRequest)
	if err != nil {
		reserveResp := &cns.IPConfigResponse{
			Response: cns.Response{
				ReturnCode: types.FailedToAllocateIPConfig,
				Message:    fmt.Sprintf("AllocateIPConfig failed: %v, IP config request is %s", err, ipconfigRequest),
			},
			PodIpInfo: podIPInfo,
		}
		w.Header().Set(cnsReturnCode, reserveResp.Response.ReturnCode.String())
		err = service.Listener.Encode(w, &reserveResp)
		logger.ResponseEx(service.Name+operationName, ipconfigRequest, reserveResp, reserveResp.Response.ReturnCode, err)
		return
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
		err = service.updateEndpointState(ipconfigRequest, podInfo, podIPInfo)
		if err != nil {
			reserveResp := &cns.IPConfigResponse{
				Response: cns.Response{
					ReturnCode: types.UnexpectedError,
					Message:    fmt.Sprintf("Update endpoint state failed: %v ", err),
				},
				PodIpInfo: podIPInfo,
			}
			w.Header().Set(cnsReturnCode, reserveResp.Response.ReturnCode.String())
			err = service.Listener.Encode(w, &reserveResp)
			logger.ResponseEx(service.Name+operationName, ipconfigRequest, reserveResp, reserveResp.Response.ReturnCode, err)
			return
		}
	}

	reserveResp := &cns.IPConfigResponse{
		Response: cns.Response{
			ReturnCode: types.Success,
		},
		PodIpInfo: podIPInfo,
	}
	w.Header().Set(cnsReturnCode, reserveResp.Response.ReturnCode.String())
	err = service.Listener.Encode(w, &reserveResp)
	logger.ResponseEx(service.Name+operationName, ipconfigRequest, reserveResp, reserveResp.Response.ReturnCode, err)
}

func (service *HTTPRestService) updateEndpointState(ipconfigRequest cns.IPConfigRequest, podInfo cns.PodInfo, podIPInfo cns.PodIpInfo) error {
	if service.EndpointStateStore == nil {
		return fmt.Errorf("nil endpoint state store")
	}
	service.Lock()
	defer service.Unlock()
	if service.EndpointState == nil { // if endpoint state is empty then read from store
		err := service.EndpointStateStore.Read("endpoints", service.EndpointState)
		if err != nil {
			return fmt.Errorf("failed to read endpoint state from store: %w", err)
		}
	}

	if endpointInfo, ok := service.EndpointState[ipconfigRequest.InfraContainerID]; ok {
		_, ipnet, err := net.ParseCIDR(podIPInfo.PodIPConfig.IPAddress + "/" + fmt.Sprint(podIPInfo.PodIPConfig.PrefixLength))
		if err != nil {
			return fmt.Errorf("failed to parse pod ip address: %w", err)
		}
		endpointInfo.IfnameToIPMap[ipconfigRequest.Ifname] = ipnet
		service.EndpointState[ipconfigRequest.InfraContainerID] = endpointInfo

	} else {
		endpointInfo := &EndpointInfo{PodName: podInfo.Name(), PodNamespace: podInfo.Namespace(), IfnameToIPMap: make(map[string]*net.IPNet)}
		_, ipnet, err := net.ParseCIDR(podIPInfo.PodIPConfig.IPAddress + "/" + fmt.Sprint(podIPInfo.PodIPConfig.PrefixLength))
		if err != nil {
			return fmt.Errorf("failed to parse pod ip address: %w", err)
		}
		endpointInfo.IfnameToIPMap[ipconfigRequest.Ifname] = ipnet
		service.EndpointState[ipconfigRequest.InfraContainerID] = endpointInfo
	}

	err := service.EndpointStateStore.Write("endpoints", service.EndpointState)
	if err != nil {
		return fmt.Errorf("failed to write endpoint state to store: %w", err)
	}
	return nil
}

func (service *HTTPRestService) releaseIPConfigHandler(w http.ResponseWriter, r *http.Request) {
	var req cns.IPConfigRequest
	err := service.Listener.Decode(w, r, &req)
	logger.Request(service.Name+"releaseIPConfigHandler", req, err)
	if err != nil {
		resp := cns.Response{
			ReturnCode: types.UnexpectedError,
			Message:    err.Error(),
		}
		logger.Errorf("releaseIPConfigHandler decode failed becase %v, release IP config info %s", resp.Message, req)
		w.Header().Set(cnsReturnCode, resp.ReturnCode.String())
		err = service.Listener.Encode(w, &resp)
		logger.ResponseEx(service.Name, req, resp, resp.ReturnCode, err)
		return
	}

	podInfo, returnCode, message := service.validateIPConfigRequest(req)

	if err = service.releaseIPConfig(podInfo); err != nil {
		returnCode = types.UnexpectedError
		message = err.Error()
		logger.Errorf("releaseIPConfigHandler releaseIPConfig failed because %v, release IP config info %s", message, req)
	}
	resp := cns.Response{
		ReturnCode: returnCode,
		Message:    message,
	}
	w.Header().Set(cnsReturnCode, resp.ReturnCode.String())
	err = service.Listener.Encode(w, &resp)
	logger.ResponseEx(service.Name, req, resp, resp.ReturnCode, err)
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

	service.PodIPIDByPodInterfaceKey[podInfo.Key()] = ipconfig.ID
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

	ipID := service.PodIPIDByPodInterfaceKey[podInfo.Key()]
	if ipID != "" {
		if ipconfig, isExist := service.PodIPConfigState[ipID]; isExist {
			logger.Printf("[releaseIPConfig] Releasing IP %+v for pod %+v", ipconfig.IPAddress, podInfo)
			_, err := service.unassignIPConfig(ipconfig, podInfo)
			if err != nil {
				return fmt.Errorf("[releaseIPConfig] failed to mark IPConfig [%+v] as Available. err: %v", ipconfig, err)
			}
			logger.Printf("[releaseIPConfig] Released IP %+v for pod %+v", ipconfig.IPAddress, podInfo)
		} else {
			logger.Errorf("[releaseIPConfig] Failed to get release ipconfig %+v and pod info is %+v. Pod to IPID exists, but IPID to IPConfig doesn't exist, CNS State potentially corrupt",
				ipconfig.IPAddress, podInfo)
			return fmt.Errorf("[releaseIPConfig] releaseIPConfig failed. IPconfig %+v and pod info is %+v. Pod to IPID exists, but IPID to IPConfig doesn't exist, CNS State potentially corrupt",
				ipconfig.IPAddress, podInfo)
		}
	} else {
		logger.Errorf("[releaseIPConfig] SetIPConfigAsAvailable ignoring request to release, no allocation found for pod [%+v]", podInfo)
		return nil
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

func (service *HTTPRestService) GetExistingIPConfig(podInfo cns.PodInfo) (cns.PodIpInfo, bool, error) {
	var (
		podIpInfo cns.PodIpInfo
		isExist   bool
	)

	service.RLock()
	defer service.RUnlock()

	ipID := service.PodIPIDByPodInterfaceKey[podInfo.Key()]
	if ipID != "" {
		if ipState, isExist := service.PodIPConfigState[ipID]; isExist {
			err := service.populateIPConfigInfoUntransacted(ipState, &podIpInfo)
			return podIpInfo, isExist, err
		}

		logger.Errorf("Failed to get existing ipconfig. Pod to IPID exists, but IPID to IPConfig doesn't exist, CNS State potentially corrupt")
		return podIpInfo, isExist, fmt.Errorf("Failed to get existing ipconfig. Pod to IPID exists, but IPID to IPConfig doesn't exist, CNS State potentially corrupt")
	}

	return podIpInfo, isExist, nil
}

func (service *HTTPRestService) AssignDesiredIPConfig(podInfo cns.PodInfo, desiredIPAddress string) (cns.PodIpInfo, error) {
	var podIpInfo cns.PodIpInfo
	service.Lock()
	defer service.Unlock()

	for _, ipConfig := range service.PodIPConfigState {
		if ipConfig.IPAddress == desiredIPAddress {
			switch ipConfig.GetState() { //nolint:exhaustive // ignoring PendingRelease case intentionally
			case types.Assigned:
				// This IP has already been assigned, if it is assigned to same pod, then return the same
				// IPconfiguration
				if ipConfig.PodInfo.Key() == podInfo.Key() {
					logger.Printf("[AssignDesiredIPConfig]: IP Config [%+v] is already assigned to this Pod [%+v]", ipConfig, podInfo)
				} else {
					return podIpInfo, errors.Errorf("[AssignDesiredIPConfig] Desired IP is already assigned %+v, requested for pod %+v", ipConfig, podInfo)
				}
			case types.Available, types.PendingProgramming:
				// This race can happen during restart, where CNS state is lost and thus we have lost the NC programmed version
				// As part of reconcile, we mark IPs as Assigned which are already assigned to Pods (listed from APIServer)
				if err := service.assignIPConfig(ipConfig, podInfo); err != nil {
					return podIpInfo, err
				}
			default:
				return podIpInfo, errors.Errorf("[AllocateDesiredIPConfig] Desired IP is not available %+v", ipConfig)
			}
			err := service.populateIPConfigInfoUntransacted(ipConfig, &podIpInfo)
			return podIpInfo, err
		}
	}
	return podIpInfo, fmt.Errorf("Requested IP not found in pool")
}

func (service *HTTPRestService) AssignAnyAvailableIPConfig(podInfo cns.PodInfo) (cns.PodIpInfo, error) {
	service.Lock()
	defer service.Unlock()

	for _, ipState := range service.PodIPConfigState {
		if ipState.GetState() == types.Available {
			if err := service.assignIPConfig(ipState, podInfo); err != nil {
				return cns.PodIpInfo{}, err
			}

			podIPInfo := cns.PodIpInfo{}
			if err := service.populateIPConfigInfoUntransacted(ipState, &podIPInfo); err != nil {
				return cns.PodIpInfo{}, err
			}

			return podIPInfo, nil
		}
	}
	//nolint:goerr113
	return cns.PodIpInfo{}, fmt.Errorf("no IPs available, waiting on Azure CNS to allocate more")
}

// If IPConfig is already assigned to pod, it returns that else it returns one of the available ipconfigs.
func requestIPConfigHelper(service *HTTPRestService, req cns.IPConfigRequest) (cns.PodIpInfo, error) {
	// check if ipconfig already assigned tothis pod and return if exists or error
	// if error, ipstate is nil, if exists, ipstate is not nil and error is nil
	podInfo, err := cns.NewPodInfoFromIPConfigRequest(req)
	if err != nil {
		return cns.PodIpInfo{}, errors.Wrapf(err, "failed to parse IPConfigRequest %v", req)
	}

	if podIPInfo, isExist, err := service.GetExistingIPConfig(podInfo); err != nil || isExist {
		return podIPInfo, err
	}

	// return desired IPConfig
	if req.DesiredIPAddress != "" {
		return service.AssignDesiredIPConfig(podInfo, req.DesiredIPAddress)
	}

	// return any free IPConfig
	return service.AssignAnyAvailableIPConfig(podInfo)
}
