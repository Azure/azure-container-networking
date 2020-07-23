// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
)

// used to request an IPConfig from the CNS state
func (service *HTTPRestService) requestIPConfigHandler(w http.ResponseWriter, r *http.Request) {
	var (
		err             error
		ipconfigRequest cns.GetIPConfigRequest
		ipState         ipConfigurationStatus
		returnCode      int
		returnMessage   string
	)

	err = service.Listener.Decode(w, r, &ipconfigRequest)
	logger.Request(service.Name, &ipconfigRequest, err)
	if err != nil {
		return
	}

	// retrieve ipconfig from nc
	if ipState, err = requestIPConfigHelper(service, ipconfigRequest); err != nil {
		returnCode = UnexpectedError
		returnMessage = fmt.Sprintf("AllocateIPConfig failed: %v", err)
	}

	resp := cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}

	reserveResp := &cns.GetIPConfigResponse{
		Response: resp,
	}
	reserveResp.IPConfiguration.IPSubnet = ipState.IPSubnet

	err = service.Listener.Encode(w, &reserveResp)
	logger.Response(service.Name, reserveResp, resp.ReturnCode, ReturnCodeToString(resp.ReturnCode), err)
}

func (service *HTTPRestService) releaseIPConfigHandler(w http.ResponseWriter, r *http.Request) {
	var (
		podInfo    cns.KubernetesPodInfo
		req        cns.GetIPConfigRequest
		statusCode int
	)

	statusCode = UnexpectedError

	err := service.Listener.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)
	if err != nil {
		return
	}

	defer func() {
		resp := cns.Response{}

		if err != nil {
			resp.ReturnCode = statusCode
			resp.Message = err.Error()
		}

		err = service.Listener.Encode(w, &resp)
		logger.Response(service.Name, resp, resp.ReturnCode, ReturnCodeToString(resp.ReturnCode), err)
	}()

	if service.state.OrchestratorType != cns.KubernetesCRD {
		err = fmt.Errorf("ReleaseIPConfig API supported only for kubernetes orchestrator")
		return
	}

	// retrieve podinfo  from orchestrator context
	if err = json.Unmarshal(req.OrchestratorContext, &podInfo); err != nil {
		return
	}

	if err = service.ReleaseIPConfig(podInfo); err != nil {
		statusCode = NotFound
		return
	}
	return
}

func (service *HTTPRestService) GetAllocatedIPConfigs() []ipConfigurationStatus {
	service.RLock()
	defer service.RUnlock()
	return filterIPConfigMap(service.PodIPConfigState, func(ipconfig ipConfigurationStatus) bool {
		return ipconfig.State == cns.Allocated
	})
}

func (service *HTTPRestService) GetAvailableIPConfigs() []ipConfigurationStatus {
	service.RLock()
	defer service.RUnlock()
	return filterIPConfigMap(service.PodIPConfigState, func(ipconfig ipConfigurationStatus) bool {
		return ipconfig.State == cns.Available
	})
}

func filterIPConfigMap(toBeAdded map[string]ipConfigurationStatus, f func(ipConfigurationStatus) bool) []ipConfigurationStatus {
	vsf := make([]ipConfigurationStatus, 0)
	for _, v := range toBeAdded {
		if f(v) {
			vsf = append(vsf, v)
		}
	}
	return vsf
}

//SetIPConfigAsAllocated takes a lock of the service, and sets the ipconfig in the CNS state as allocated, does not take a lock
func (service *HTTPRestService) setIPConfigAsAllocated(ipconfig ipConfigurationStatus, podInfo cns.KubernetesPodInfo, marshalledOrchestratorContext json.RawMessage) ipConfigurationStatus {
	ipconfig.State = cns.Allocated
	ipconfig.OrchestratorContext = marshalledOrchestratorContext
	service.PodIPIDByOrchestratorContext[podInfo.GetOrchestratorContextKey()] = ipconfig.ID
	service.PodIPConfigState[ipconfig.ID] = ipconfig
	return service.PodIPConfigState[ipconfig.ID]
}

//SetIPConfigAsAllocated and sets the ipconfig in the CNS state as allocated, does not take a lock
func (service *HTTPRestService) setIPConfigAsAvailable(ipconfig ipConfigurationStatus, podInfo cns.KubernetesPodInfo) ipConfigurationStatus {
	ipconfig.State = cns.Available
	ipconfig.OrchestratorContext = nil
	service.PodIPConfigState[ipconfig.ID] = ipconfig
	delete(service.PodIPIDByOrchestratorContext, podInfo.GetOrchestratorContextKey())
	return service.PodIPConfigState[ipconfig.ID]
}

////SetIPConfigAsAllocated takes a lock of the service, and sets the ipconfig in the CNS stateas Available
// Todo - CNI should also pass the IPAddress which needs to be released to validate if that is the right IP allcoated
// in the first place.
func (service *HTTPRestService) ReleaseIPConfig(podInfo cns.KubernetesPodInfo) error {
	service.Lock()
	defer service.Unlock()

	ipID := service.PodIPIDByOrchestratorContext[podInfo.GetOrchestratorContextKey()]
	if ipID != "" {
		if ipconfig, isExist := service.PodIPConfigState[ipID]; isExist {
			service.setIPConfigAsAvailable(ipconfig, podInfo)
		} else {
			logger.Errorf("Failed to get release ipconfig. Pod to IPID exists, but IPID to IPConfig doesn't exist, CNS State potentially corrupt")
			return fmt.Errorf("ReleaseIPConfig failed. Pod to IPID exists, but IPID to IPConfig doesn't exist, CNS State potentially corrupt")
		}
	} else {
		logger.Printf("SetIPConfigAsAvailable failed to release, no allocation found for pod")
		return nil
	}
	return nil
}

func (service *HTTPRestService) GetExistingIPConfig(podInfo cns.KubernetesPodInfo) (ipConfigurationStatus, bool, error) {
	var (
		ipState ipConfigurationStatus
		isExist bool
	)

	service.RLock()
	defer service.RUnlock()

	ipID := service.PodIPIDByOrchestratorContext[podInfo.GetOrchestratorContextKey()]
	if ipID != "" {
		if ipState, isExist = service.PodIPConfigState[ipID]; isExist {
			return ipState, isExist, nil
		}
		logger.Errorf("Failed to get existing ipconfig. Pod to IPID exists, but IPID to IPConfig doesn't exist, CNS State potentially corrupt")
		return ipState, isExist, fmt.Errorf("Failed to get existing ipconfig. Pod to IPID exists, but IPID to IPConfig doesn't exist, CNS State potentially corrupt")
	}

	return ipState, isExist, nil
}

func (service *HTTPRestService) AllocateDesiredIPConfig(podInfo cns.KubernetesPodInfo, desiredIPAddress string, orchestratorContext json.RawMessage) (ipConfigurationStatus, error) {
	var ipState ipConfigurationStatus

	service.Lock()
	defer service.Unlock()

	for _, ipState := range service.PodIPConfigState {
		if ipState.IPSubnet.IPAddress == desiredIPAddress {
			if ipState.State == cns.Available {
				return service.setIPConfigAsAllocated(ipState, podInfo, orchestratorContext), nil
			}
			return ipState, fmt.Errorf("Desired IP has already been allocated")
		}
	}
	return ipState, fmt.Errorf("Requested IP not found in pool")
}

func (service *HTTPRestService) AllocateAnyAvailableIPConfig(podInfo cns.KubernetesPodInfo, orchestratorContext json.RawMessage) (ipConfigurationStatus, error) {
	var ipState ipConfigurationStatus

	service.Lock()
	defer service.Unlock()

	for _, ipState = range service.PodIPConfigState {
		if ipState.State == cns.Available {
			return service.setIPConfigAsAllocated(ipState, podInfo, orchestratorContext), nil
		}
	}
	return ipState, fmt.Errorf("No more free IP's available, trigger batch")
}

// If IPConfig is already allocated for pod, it returns that else it returns one of the available ipconfigs.
func requestIPConfigHelper(service *HTTPRestService, req cns.GetIPConfigRequest) (ipConfigurationStatus, error) {
	var (
		podInfo cns.KubernetesPodInfo
		ipState ipConfigurationStatus
		isExist bool
		err     error
	)

	// todo - change it to
	if service.state.OrchestratorType != cns.KubernetesCRD {
		return ipState, fmt.Errorf("AllocateIPconfig API supported only for kubernetes orchestrator")
	}

	// retrieve podinfo  from orchestrator context
	if err := json.Unmarshal(req.OrchestratorContext, &podInfo); err != nil {
		return ipState, err
	}

	// check if ipconfig already allocated for this pod and return if exists or error
	// if error, ipstate is nil, if exists, ipstate is not nil and error is nil
	if ipState, isExist, err = service.GetExistingIPConfig(podInfo); err != nil || isExist {
		return ipState, err
	}

	// return desired IPConfig
	if req.DesiredIPConfig.IPAddress != "" {
		return service.AllocateDesiredIPConfig(podInfo, req.DesiredIPConfig.IPAddress, req.OrchestratorContext)
	}

	// return any free IPConfig
	return service.AllocateAnyAvailableIPConfig(podInfo, req.OrchestratorContext)
}
