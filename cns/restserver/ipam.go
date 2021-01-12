// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
)

// used to request an IPConfig from the CNS state
func (service *HTTPRestService) requestIPConfigHandler(w http.ResponseWriter, r *http.Request) {
	var (
		err             error
		ipconfigRequest cns.IPConfigRequest
		podIpInfo       cns.PodIpInfo
		returnCode      int
		returnMessage   string
	)

	err = service.Listener.Decode(w, r, &ipconfigRequest)
	logger.Request(service.Name, &ipconfigRequest, err)
	if err != nil {
		return
	}

	// retrieve ipconfig from nc
	_, returnCode, returnMessage = service.validateIpConfigRequest(ipconfigRequest)
	if returnCode == Success {
		if podIpInfo, err = requestIPConfigHelper(service, ipconfigRequest); err != nil {
			returnCode = FailedToAllocateIpConfig
			returnMessage = fmt.Sprintf("AllocateIPConfig failed: %v", err)
		}
	}

	resp := cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}

	reserveResp := &cns.IPConfigResponse{
		Response: resp,
	}
	reserveResp.PodIpInfo = podIpInfo

	err = service.Listener.Encode(w, &reserveResp)
	logger.Response(service.Name, reserveResp, resp.ReturnCode, ReturnCodeToString(resp.ReturnCode), err)
}

func (service *HTTPRestService) releaseIPConfigHandler(w http.ResponseWriter, r *http.Request) {
	var (
		req           cns.IPConfigRequest
		statusCode    int
		returnMessage string
		err           error
	)

	statusCode = UnexpectedError

	defer func() {
		resp := cns.Response{}

		if err != nil {
			resp.ReturnCode = statusCode
			resp.Message = returnMessage
		}

		err = service.Listener.Encode(w, &resp)
		logger.Response(service.Name, resp, resp.ReturnCode, ReturnCodeToString(resp.ReturnCode), err)
	}()

	err = service.Listener.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)
	if err != nil {
		returnMessage = err.Error()
		return
	}

	podInfo, statusCode, returnMessage := service.validateIpConfigRequest(req)

	if err = service.releaseIPConfig(podInfo); err != nil {
		statusCode = NotFound
		returnMessage = err.Error()
		return
	}
	return
}

// MarkIPAsPendingRelease will mark IPs to pending release state.
func (service *HTTPRestService) MarkIPAsPendingRelease(numberToMark int) (map[string]cns.IPConfigurationStatus, error) {
	allReleasedIPs := make(map[string]cns.IPConfigurationStatus)
	// Ensure PendingProgramming IPs will be release before Available ones.
	ipStateTypes := [2]string{cns.PendingProgramming, cns.Available}

	service.Lock()
	defer service.Unlock()
	for _, ipStateType := range ipStateTypes {
		pendingReleaseIPs := service.markSpecificIPTypeAsPending(numberToMark, ipStateType)
		for uuid, pependingReleaseIP := range pendingReleaseIPs {
			allReleasedIPs[uuid] = pependingReleaseIP
		}
		numberToMark -= len(pendingReleaseIPs)
		if numberToMark == 0 {
			return allReleasedIPs, nil
		}
	}
	return nil, fmt.Errorf("Failed to mark %d IP's as pending, only marked %d IP's", numberToMark, len(allReleasedIPs))
}

func (service *HTTPRestService) markSpecificIPTypeAsPending(numberToMark int, ipStateType string) map[string]cns.IPConfigurationStatus {
	pendingReleaseIPs := make(map[string]cns.IPConfigurationStatus)
	for uuid, mutableIPConfig := range service.PodIPConfigState {
		if mutableIPConfig.State == ipStateType {
			mutableIPConfig.State = cns.PendingRelease
			service.PodIPConfigState[uuid] = mutableIPConfig
			pendingReleaseIPs[uuid] = mutableIPConfig
			if len(pendingReleaseIPs) == numberToMark {
				return pendingReleaseIPs
			}
		}
	}
	return pendingReleaseIPs
}

// MarkIpsAsAvailableUntransacted will update pending programming IPs to available if NMAgent side's programmed nc version keep up with nc version.
// Note: this func is an untransacted API as the caller will take a Service lock
func (service *HTTPRestService) MarkIpsAsAvailableUntransacted(ncID string, newHostNCVersion int) {
	// Check whether it exist in service state and get the related nc info
	if ncInfo, exist := service.state.ContainerStatus[ncID]; !exist {
		logger.Errorf("Can't find NC with ID %s in service state, stop updating its pending programming IP status", ncID)
	} else {
		previousHostNCVersion := ncInfo.HostVersion
		// We only need to handle the situation when dnc nc version is larger than programmed nc version
		if previousHostNCVersion < ncInfo.CreateNetworkContainerRequest.Version && previousHostNCVersion < strconv.Itoa(newHostNCVersion) {
			for uuid, secondaryIPConfigs := range ncInfo.CreateNetworkContainerRequest.SecondaryIPConfigs {
				if ipConfigStatus, exist := service.PodIPConfigState[uuid]; !exist {
					logger.Errorf("IP %s with uuid as %s exist in service state Secondary IP list but can't find in PodIPConfigState", ipConfigStatus.IPAddress, uuid)
				} else if ipConfigStatus.State == cns.PendingProgramming && secondaryIPConfigs.NCVersion <= newHostNCVersion {
					ipConfigStatus.State = cns.Available
					service.PodIPConfigState[uuid] = ipConfigStatus
					// Following 2 sentence assign new host version to secondary ip config.
					secondaryIPConfigs.NCVersion = newHostNCVersion
					ncInfo.CreateNetworkContainerRequest.SecondaryIPConfigs[uuid] = secondaryIPConfigs
					logger.Printf("Change ip %s with uuid %s from pending programming to %s, current secondary ip configs is %v", ipConfigStatus.IPAddress, uuid, cns.Available,
						ncInfo.CreateNetworkContainerRequest.SecondaryIPConfigs[uuid])
				}
			}
		}
	}
}

func (service *HTTPRestService) GetPodIPConfigState() map[string]cns.IPConfigurationStatus {
	service.RLock()
	defer service.RUnlock()
	return service.PodIPConfigState
}

// GetPendingProgramIPConfigs returns list of IPs which are in pending program status
func (service *HTTPRestService) GetPendingProgramIPConfigs() []cns.IPConfigurationStatus {
	service.RLock()
	defer service.RUnlock()
	return filterIPConfigMap(service.PodIPConfigState, func(ipconfig cns.IPConfigurationStatus) bool {
		return ipconfig.State == cns.PendingProgramming
	})
}

func (service *HTTPRestService) getIPAddressesHandler(w http.ResponseWriter, r *http.Request) {
	var (
		req           cns.GetIPAddressesRequest
		resp          cns.GetIPAddressStateResponse
		statusCode    int
		returnMessage string
		err           error
	)

	statusCode = UnexpectedError

	defer func() {
		if err != nil {
			resp.Response.ReturnCode = statusCode
			resp.Response.Message = returnMessage
		}

		err = service.Listener.Encode(w, &resp)
		logger.Response(service.Name, resp, resp.Response.ReturnCode, ReturnCodeToString(resp.Response.ReturnCode), err)
	}()

	err = service.Listener.Decode(w, r, &req)
	if err != nil {
		returnMessage = err.Error()
		return
	}

	filterFunc := func(ipconfig cns.IPConfigurationStatus, states []string) bool {
		for _, state := range states {
			if ipconfig.State == state {
				return true
			}
		}
		return false
	}

	// Get all IPConfigs matching a state, and append to a slice of IPAddressState
	resp.IPAddresses = filterIPConfigsMatchingState(service.PodIPConfigState, req.IPConfigStateFilter, filterFunc)

	return
}

// filter the ipconfigs in CNS matching a state (Available, Allocated, etc.) and return in a slice
func filterIPConfigsMatchingState(toBeAdded map[string]cns.IPConfigurationStatus, states []string, f func(cns.IPConfigurationStatus, []string) bool) []cns.IPAddressState {
	vsf := make([]cns.IPAddressState, 0)
	for _, v := range toBeAdded {
		if f(v, states) {
			ip := cns.IPAddressState{
				IPAddress: v.IPAddress,
				State:     v.State,
			}
			vsf = append(vsf, ip)
		}
	}
	return vsf
}

// filter ipconfigs based on predicate
func filterIPConfigs(toBeAdded map[string]cns.IPConfigurationStatus, f func(cns.IPConfigurationStatus) bool) []cns.IPConfigurationStatus {
	vsf := make([]cns.IPConfigurationStatus, 0)
	for _, v := range toBeAdded {
		if f(v) {
			vsf = append(vsf, v)
		}
	}
	return vsf
}

func (service *HTTPRestService) GetAllocatedIPConfigs() []cns.IPConfigurationStatus {
	service.RLock()
	defer service.RUnlock()
	return filterIPConfigMap(service.PodIPConfigState, func(ipconfig cns.IPConfigurationStatus) bool {
		return ipconfig.State == cns.Allocated
	})
}

func (service *HTTPRestService) GetAvailableIPConfigs() []cns.IPConfigurationStatus {
	service.RLock()
	defer service.RUnlock()
	return filterIPConfigMap(service.PodIPConfigState, func(ipconfig cns.IPConfigurationStatus) bool {
		return ipconfig.State == cns.Available
	})
}

func (service *HTTPRestService) GetPendingReleaseIPConfigs() []cns.IPConfigurationStatus {
	service.RLock()
	defer service.RUnlock()
	return filterIPConfigMap(service.PodIPConfigState, func(ipconfig cns.IPConfigurationStatus) bool {
		return ipconfig.State == cns.PendingRelease
	})
}

func filterIPConfigMap(toBeAdded map[string]cns.IPConfigurationStatus, f func(cns.IPConfigurationStatus) bool) []cns.IPConfigurationStatus {
	vsf := make([]cns.IPConfigurationStatus, 0)
	for _, v := range toBeAdded {
		if f(v) {
			vsf = append(vsf, v)
		}
	}
	return vsf
}

//SetIPConfigAsAllocated takes a lock of the service, and sets the ipconfig in the CNS state as allocated, does not take a lock
func (service *HTTPRestService) setIPConfigAsAllocated(ipconfig cns.IPConfigurationStatus, podInfo cns.KubernetesPodInfo, marshalledOrchestratorContext json.RawMessage) cns.IPConfigurationStatus {
	ipconfig.State = cns.Allocated
	ipconfig.OrchestratorContext = marshalledOrchestratorContext
	service.PodIPIDByOrchestratorContext[podInfo.GetOrchestratorContextKey()] = ipconfig.ID
	service.PodIPConfigState[ipconfig.ID] = ipconfig
	return service.PodIPConfigState[ipconfig.ID]
}

//SetIPConfigAsAllocated and sets the ipconfig in the CNS state as allocated, does not take a lock
func (service *HTTPRestService) setIPConfigAsAvailable(ipconfig cns.IPConfigurationStatus, podInfo cns.KubernetesPodInfo) cns.IPConfigurationStatus {
	ipconfig.State = cns.Available
	ipconfig.OrchestratorContext = nil
	service.PodIPConfigState[ipconfig.ID] = ipconfig
	delete(service.PodIPIDByOrchestratorContext, podInfo.GetOrchestratorContextKey())
	return service.PodIPConfigState[ipconfig.ID]
}

////SetIPConfigAsAllocated takes a lock of the service, and sets the ipconfig in the CNS stateas Available
// Todo - CNI should also pass the IPAddress which needs to be released to validate if that is the right IP allcoated
// in the first place.
func (service *HTTPRestService) releaseIPConfig(podInfo cns.KubernetesPodInfo) error {
	service.Lock()
	defer service.Unlock()

	ipID := service.PodIPIDByOrchestratorContext[podInfo.GetOrchestratorContextKey()]
	if ipID != "" {
		if ipconfig, isExist := service.PodIPConfigState[ipID]; isExist {
			service.setIPConfigAsAvailable(ipconfig, podInfo)
			logger.Printf("Released IP %+v for pod %+v", ipconfig.IPAddress, podInfo)

		} else {
			logger.Errorf("Failed to get release ipconfig. Pod to IPID exists, but IPID to IPConfig doesn't exist, CNS State potentially corrupt")
			return fmt.Errorf("releaseIPConfig failed. Pod to IPID exists, but IPID to IPConfig doesn't exist, CNS State potentially corrupt")
		}
	} else {
		logger.Errorf("SetIPConfigAsAvailable failed to release, no allocation found for pod")
		return nil
	}
	return nil
}

// called when CNS is starting up and there are existing ipconfigs in the CRD that are marked as pending
func (service *HTTPRestService) MarkExistingIPsAsPending(pendingIPIDs []string) error {
	service.Lock()
	defer service.Unlock()

	for _, id := range pendingIPIDs {
		if ipconfig, exists := service.PodIPConfigState[id]; exists {
			if ipconfig.State == cns.Allocated {
				return fmt.Errorf("Failed to mark IP [%v] as pending, currently allocated", id)
			}

			ipconfig.State = cns.PendingRelease
			service.PodIPConfigState[id] = ipconfig
		} else {
			logger.Errorf("Inconsistent state, ipconfig with ID [%v] marked as pending release, but does not exist in state", id)
		}
	}
	return nil
}

func (service *HTTPRestService) GetExistingIPConfig(podInfo cns.KubernetesPodInfo) (cns.PodIpInfo, bool, error) {
	var (
		podIpInfo cns.PodIpInfo
		isExist   bool
	)

	service.RLock()
	defer service.RUnlock()

	ipID := service.PodIPIDByOrchestratorContext[podInfo.GetOrchestratorContextKey()]
	if ipID != "" {
		if ipState, isExist := service.PodIPConfigState[ipID]; isExist {
			err := service.populateIpConfigInfoUntransacted(ipState, &podIpInfo)
			return podIpInfo, isExist, err
		}

		logger.Errorf("Failed to get existing ipconfig. Pod to IPID exists, but IPID to IPConfig doesn't exist, CNS State potentially corrupt")
		return podIpInfo, isExist, fmt.Errorf("Failed to get existing ipconfig. Pod to IPID exists, but IPID to IPConfig doesn't exist, CNS State potentially corrupt")
	}

	return podIpInfo, isExist, nil
}

func (service *HTTPRestService) AllocateDesiredIPConfig(podInfo cns.KubernetesPodInfo, desiredIPAddress string, orchestratorContext json.RawMessage) (cns.PodIpInfo, error) {
	var podIpInfo cns.PodIpInfo
	service.Lock()
	defer service.Unlock()

	found := false
	for _, ipState := range service.PodIPConfigState {
		if ipState.IPAddress == desiredIPAddress {
			if ipState.State == cns.Allocated {
				// This IP has already been allocated, if it is allocated to same pod, then return the same
				// IPconfiguration
				if bytes.Equal(orchestratorContext, ipState.OrchestratorContext) == true {
					found = true
				} else {
					var pInfo cns.KubernetesPodInfo
					json.Unmarshal(ipState.OrchestratorContext, &pInfo)
					return podIpInfo, fmt.Errorf("Desired IP is already allocated %+v to Pod: %+v, requested for pod %+v", ipState, pInfo, podInfo)
				}
			} else if ipState.State == cns.Available {
				service.setIPConfigAsAllocated(ipState, podInfo, orchestratorContext)
				found = true
			} else {
				return podIpInfo, fmt.Errorf("Desired IP is not available %+v", ipState)
			}

			if found {
				err := service.populateIpConfigInfoUntransacted(ipState, &podIpInfo)
				return podIpInfo, err
			}
		}
	}
	return podIpInfo, fmt.Errorf("Requested IP not found in pool")
}

func (service *HTTPRestService) AllocateAnyAvailableIPConfig(podInfo cns.KubernetesPodInfo, orchestratorContext json.RawMessage) (cns.PodIpInfo, error) {
	var podIpInfo cns.PodIpInfo

	service.Lock()
	defer service.Unlock()

	for _, ipState := range service.PodIPConfigState {
		if ipState.State == cns.Available {
			err := service.populateIpConfigInfoUntransacted(ipState, &podIpInfo)
			if err == nil {
				service.setIPConfigAsAllocated(ipState, podInfo, orchestratorContext)
			}
			return podIpInfo, err
		}
	}

	return podIpInfo, fmt.Errorf("No more free IP's available, waiting on Azure CNS to allocated more IP's...")
}

// If IPConfig is already allocated for pod, it returns that else it returns one of the available ipconfigs.
func requestIPConfigHelper(service *HTTPRestService, req cns.IPConfigRequest) (cns.PodIpInfo, error) {
	var (
		podInfo   cns.KubernetesPodInfo
		podIpInfo cns.PodIpInfo
		isExist   bool
		err       error
	)

	// check if ipconfig already allocated for this pod and return if exists or error
	// if error, ipstate is nil, if exists, ipstate is not nil and error is nil
	json.Unmarshal(req.OrchestratorContext, &podInfo)
	if podIpInfo, isExist, err = service.GetExistingIPConfig(podInfo); err != nil || isExist {
		return podIpInfo, err
	}

	// return desired IPConfig
	if req.DesiredIPAddress != "" {
		return service.AllocateDesiredIPConfig(podInfo, req.DesiredIPAddress, req.OrchestratorContext)
	}

	// return any free IPConfig
	return service.AllocateAnyAvailableIPConfig(podInfo, req.OrchestratorContext)
}
