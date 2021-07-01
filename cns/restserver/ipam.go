// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package restserver

import (
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
	operationName := "requestIPConfigHandler"
	logger.Request(service.Name+operationName, ipconfigRequest, err)
	if err != nil {
		return
	}

	// retrieve ipconfig from nc
	_, returnCode, returnMessage = service.validateIpConfigRequest(ipconfigRequest)
	if returnCode == Success {
		if podIpInfo, err = requestIPConfigHelper(service, ipconfigRequest); err != nil {
			returnCode = FailedToAllocateIpConfig
			returnMessage = fmt.Sprintf("AllocateIPConfig failed: %v, IP config request is %s", err, ipconfigRequest)
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
	logger.ResponseEx(service.Name+operationName, ipconfigRequest, reserveResp, resp.ReturnCode, ReturnCodeToString(resp.ReturnCode), err)
}

func (service *HTTPRestService) releaseIPConfigHandler(w http.ResponseWriter, r *http.Request) {
	req := cns.IPConfigRequest{}
	resp := cns.Response{}
	var err error

	defer func() {
		err = service.Listener.Encode(w, &resp)
		logger.ResponseEx(service.Name, req, resp, resp.ReturnCode, ReturnCodeToString(resp.ReturnCode), err)
	}()

	err = service.Listener.Decode(w, r, &req)
	logger.Request(service.Name+"releaseIPConfigHandler", req, err)
	if err != nil {
		resp.ReturnCode = UnexpectedError
		resp.Message = err.Error()
		logger.Errorf("releaseIPConfigHandler decode failed becase %v, release IP config info %s", resp.Message, req)
		return
	}

	var podInfo cns.PodInfo
	podInfo, resp.ReturnCode, resp.Message = service.validateIpConfigRequest(req)

	if err = service.releaseIPConfig(podInfo); err != nil {
		resp.ReturnCode = UnexpectedError
		resp.Message = err.Error()
		logger.Errorf("releaseIPConfigHandler releaseIPConfig failed because %v, release IP config info %s", resp.Message, req)
		return
	}
}

// MarkIPAsPendingRelease will set the IPs which are in PendingProgramming or Available to PendingRelease state
// It will try to update [totalIpsToRelease]  number of ips.
func (service *HTTPRestService) MarkIPAsPendingRelease(totalIpsToRelease int) (map[string]cns.IPConfigurationStatus, error) {
	pendingReleasedIps := make(map[string]cns.IPConfigurationStatus)
	service.Lock()
	defer service.Unlock()

	for uuid, existingIpConfig := range service.PodIPConfigState {
		if existingIpConfig.State == cns.PendingProgramming {
			updatedIpConfig, err := service.updateIPConfigState(uuid, cns.PendingRelease, existingIpConfig.PodInfo)
			if err != nil {
				return nil, err
			}

			pendingReleasedIps[uuid] = updatedIpConfig
			if len(pendingReleasedIps) == totalIpsToRelease {
				return pendingReleasedIps, nil
			}
		}
	}

	// if not all expected IPs are set to PendingRelease, then check the Available IPs
	for uuid, existingIpConfig := range service.PodIPConfigState {
		if existingIpConfig.State == cns.Available {
			updatedIpConfig, err := service.updateIPConfigState(uuid, cns.PendingRelease, existingIpConfig.PodInfo)
			if err != nil {
				return nil, err
			}

			pendingReleasedIps[uuid] = updatedIpConfig

			if len(pendingReleasedIps) == totalIpsToRelease {
				return pendingReleasedIps, nil
			}
		}
	}

	logger.Printf("[MarkIPAsPendingRelease] Set total ips to PendingRelease %d, expected %d", len(pendingReleasedIps), totalIpsToRelease)
	return pendingReleasedIps, nil
}

func (service *HTTPRestService) updateIPConfigState(ipId string, updatedState string, podInfo cns.PodInfo) (cns.IPConfigurationStatus, error) {
	if ipConfig, found := service.PodIPConfigState[ipId]; found {
		logger.Printf("[updateIPConfigState] Changing IpId [%s] state to [%s], podInfo [%+v]. Current config [%+v]", ipId, updatedState, podInfo, ipConfig)
		ipConfig.State = updatedState
		ipConfig.PodInfo = podInfo
		service.PodIPConfigState[ipId] = ipConfig
		return ipConfig, nil
	}

	return cns.IPConfigurationStatus{}, fmt.Errorf("[updateIPConfigState] Failed to update state %s for the IPConfig. ID %s not found PodIPConfigState", updatedState, ipId)
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
				} else if ipConfigStatus.State == cns.PendingProgramming && secondaryIPConfigs.NCVersion <= newHostNCVersion {
					_, err := service.updateIPConfigState(uuid, cns.Available, nil)
					if err != nil {
						logger.Errorf("Error updating IPConfig [%+v] state to Available, err: %+v", ipConfigStatus, err)
					}

					// Following 2 sentence assign new host version to secondary ip config.
					secondaryIPConfigs.NCVersion = newHostNCVersion
					ncInfo.CreateNetworkContainerRequest.SecondaryIPConfigs[uuid] = secondaryIPConfigs
					logger.Printf("Change ip %s with uuid %s from pending programming to %s, current secondary ip configs is %+v", ipConfigStatus.IPAddress, uuid, cns.Available,
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

func (service *HTTPRestService) getPodIPIDByOrchestratorContexthandler(w http.ResponseWriter, r *http.Request) {
	var (
		resp          cns.GetPodContextResponse
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

	resp.PodContext = service.GetPodIPIDByOrchestratorContext()

	return
}

func (service *HTTPRestService) GetPodIPIDByOrchestratorContext() map[string]string {
	service.RLock()
	defer service.RUnlock()
	return service.PodIPIDByPodInterfaceKey
}

func (service *HTTPRestService) GetHTTPRestDataHandler(w http.ResponseWriter, r *http.Request) {
	var (
		resp          GetHTTPServiceDataResponse
		returnMessage string
		err           error
	)

	defer func() {
		if err != nil {
			resp.Response.ReturnCode = UnexpectedError
			resp.Response.Message = returnMessage
		}

		err = service.Listener.Encode(w, &resp)
		logger.Response(service.Name, resp, resp.Response.ReturnCode, ReturnCodeToString(resp.Response.ReturnCode), err)
	}()

	resp.HttpRestServiceData = service.GetHTTPStruct()
	return
}

func (service *HTTPRestService) GetHTTPStruct() HttpRestServiceData {
	service.RLock()
	defer service.RUnlock()

	return HttpRestServiceData{
		PodIPIDByPodInterfaceKey: service.PodIPIDByPodInterfaceKey,
		PodIPConfigState:         service.PodIPConfigState,
		IPAMPoolMonitor:          service.IPAMPoolMonitor.GetStateSnapshot(),
	}
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
		resp          cns.GetIPAddressStatusResponse
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
		logger.ResponseEx(service.Name, req, resp, resp.Response.ReturnCode, ReturnCodeToString(resp.Response.ReturnCode), err)
	}()

	err = service.Listener.Decode(w, r, &req)
	if err != nil {
		returnMessage = err.Error()
		logger.Errorf("getIPAddressesHandler decode failed because %v, GetIPAddressesRequest is %v",
			returnMessage, req)
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
	resp.IPConfigurationStatus = filterIPConfigsMatchingState(service.PodIPConfigState, req.IPConfigStateFilter, filterFunc)

	return
}

// filter the ipconfigs in CNS matching a state (Available, Allocated, etc.) and return in a slice
func filterIPConfigsMatchingState(toBeAdded map[string]cns.IPConfigurationStatus, states []string, f func(cns.IPConfigurationStatus, []string) bool) []cns.IPConfigurationStatus {
	vsf := make([]cns.IPConfigurationStatus, 0)
	for _, v := range toBeAdded {
		if f(v, states) {
			vsf = append(vsf, v)
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
func (service *HTTPRestService) setIPConfigAsAllocated(ipconfig cns.IPConfigurationStatus, podInfo cns.PodInfo) (cns.IPConfigurationStatus, error) {
	ipconfig, err := service.updateIPConfigState(ipconfig.ID, cns.Allocated, podInfo)
	if err != nil {
		return cns.IPConfigurationStatus{}, err
	}

	service.PodIPIDByPodInterfaceKey[podInfo.Key()] = ipconfig.ID
	return ipconfig, nil
}

//SetIPConfigAsAllocated and sets the ipconfig in the CNS state as allocated, does not take a lock
func (service *HTTPRestService) setIPConfigAsAvailable(ipconfig cns.IPConfigurationStatus, podInfo cns.PodInfo) (cns.IPConfigurationStatus, error) {
	ipconfig, err := service.updateIPConfigState(ipconfig.ID, cns.Available, nil)
	if err != nil {
		return cns.IPConfigurationStatus{}, err
	}

	delete(service.PodIPIDByPodInterfaceKey, podInfo.Key())
	logger.Printf("[setIPConfigAsAvailable] Deleted outdated pod info %s from PodIPIDByOrchestratorContext since IP %s with ID %s will be released and set as Available",
		podInfo.Key(), ipconfig.IPAddress, ipconfig.ID)
	return ipconfig, nil
}

////SetIPConfigAsAllocated takes a lock of the service, and sets the ipconfig in the CNS stateas Available
// Todo - CNI should also pass the IPAddress which needs to be released to validate if that is the right IP allcoated
// in the first place.
func (service *HTTPRestService) releaseIPConfig(podInfo cns.PodInfo) error {
	service.Lock()
	defer service.Unlock()

	ipID := service.PodIPIDByPodInterfaceKey[podInfo.Key()]
	if ipID != "" {
		if ipconfig, isExist := service.PodIPConfigState[ipID]; isExist {
			logger.Printf("[releaseIPConfig] Releasing IP %+v for pod %+v", ipconfig.IPAddress, podInfo)
			_, err := service.setIPConfigAsAvailable(ipconfig, podInfo)
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

// called when CNS is starting up and there are existing ipconfigs in the CRD that are marked as pending
func (service *HTTPRestService) MarkExistingIPsAsPending(pendingIPIDs []string) error {
	service.Lock()
	defer service.Unlock()

	for _, id := range pendingIPIDs {
		if ipconfig, exists := service.PodIPConfigState[id]; exists {
			if ipconfig.State == cns.Allocated {
				return fmt.Errorf("Failed to mark IP [%v] as pending, currently allocated", id)
			}

			logger.Printf("[MarkExistingIPsAsPending]: Marking IP [%+v] to PendingRelease", ipconfig)
			ipconfig.State = cns.PendingRelease
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
			err := service.populateIpConfigInfoUntransacted(ipState, &podIpInfo)
			return podIpInfo, isExist, err
		}

		logger.Errorf("Failed to get existing ipconfig. Pod to IPID exists, but IPID to IPConfig doesn't exist, CNS State potentially corrupt")
		return podIpInfo, isExist, fmt.Errorf("Failed to get existing ipconfig. Pod to IPID exists, but IPID to IPConfig doesn't exist, CNS State potentially corrupt")
	}

	return podIpInfo, isExist, nil
}

func (service *HTTPRestService) AllocateDesiredIPConfig(podInfo cns.PodInfo, desiredIpAddress string) (cns.PodIpInfo, error) {
	var podIpInfo cns.PodIpInfo
	service.Lock()
	defer service.Unlock()

	found := false
	for _, ipConfig := range service.PodIPConfigState {
		if ipConfig.IPAddress == desiredIpAddress {
			if ipConfig.State == cns.Allocated {
				// This IP has already been allocated, if it is allocated to same pod, then return the same
				// IPconfiguration
				if ipConfig.PodInfo.Key() == podInfo.Key() {
					logger.Printf("[AllocateDesiredIPConfig]: IP Config [%+v] is already allocated to this Pod [%+v]", ipConfig, podInfo)
					found = true
				} else {
					return podIpInfo, fmt.Errorf("[AllocateDesiredIPConfig] Desired IP is already allocated %+v, requested for pod %+v", ipConfig, podInfo)
				}
			} else if ipConfig.State == cns.Available || ipConfig.State == cns.PendingProgramming {
				// This race can happen during restart, where CNS state is lost and thus we have lost the NC programmed version
				// As part of reconcile, we mark IPs as Allocated which are already allocated to PODs (listed from APIServer)
				_, err := service.setIPConfigAsAllocated(ipConfig, podInfo)
				if err != nil {
					return podIpInfo, err
				}
				found = true
			} else {
				return podIpInfo, fmt.Errorf("[AllocateDesiredIPConfig] Desired IP is not available %+v", ipConfig)
			}

			if found {
				err := service.populateIpConfigInfoUntransacted(ipConfig, &podIpInfo)
				return podIpInfo, err
			}
		}
	}
	return podIpInfo, fmt.Errorf("Requested IP not found in pool")
}

func (service *HTTPRestService) AllocateAnyAvailableIPConfig(podInfo cns.PodInfo) (cns.PodIpInfo, error) {
	var podIpInfo cns.PodIpInfo

	service.Lock()
	defer service.Unlock()

	for _, ipState := range service.PodIPConfigState {
		if ipState.State == cns.Available {
			_, err := service.setIPConfigAsAllocated(ipState, podInfo)
			if err != nil {
				return podIpInfo, err
			}

			err = service.populateIpConfigInfoUntransacted(ipState, &podIpInfo)
			if err != nil {
				return podIpInfo, err
			}

			return podIpInfo, err
		}
	}

	return podIpInfo, fmt.Errorf("No more free IP's available, waiting on Azure CNS to allocated more IP's...")
}

// If IPConfig is already allocated for pod, it returns that else it returns one of the available ipconfigs.
func requestIPConfigHelper(service *HTTPRestService, req cns.IPConfigRequest) (cns.PodIpInfo, error) {
	var (
		podIpInfo cns.PodIpInfo
		isExist   bool
	)

	// check if ipconfig already allocated for this pod and return if exists or error
	// if error, ipstate is nil, if exists, ipstate is not nil and error is nil
	podInfo, err := cns.NewPodInfoFromIPConfigRequest(req)
	if err != nil {
		return podIpInfo, err
	}

	if podIpInfo, isExist, err = service.GetExistingIPConfig(podInfo); err != nil || isExist {
		return podIpInfo, err
	}

	// return desired IPConfig
	if req.DesiredIPAddress != "" {
		return service.AllocateDesiredIPConfig(podInfo, req.DesiredIPAddress)
	}

	// return any free IPConfig
	return service.AllocateAnyAvailableIPConfig(podInfo)
}
