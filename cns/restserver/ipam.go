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

func newIPConfig(ipAddress string, prefixLength uint8) cns.IPSubnet {
	return cns.IPSubnet{
		IPAddress:    ipAddress,
		PrefixLength: prefixLength,
	}
}

func NewPodState(ipaddress string, prefixLength uint8, id, ncid, state string) cns.ContainerIPConfigState {
	ipconfig := newIPConfig(ipaddress, prefixLength)

	return cns.ContainerIPConfigState{
		IPConfig: ipconfig,
		ID:       id,
		NCID:     ncid,
		State:    state,
	}
}

func NewPodStateWithOrchestratorContext(ipaddress string, prefixLength uint8, id, ncid, state string, orchestratorContext cns.KubernetesPodInfo) (cns.ContainerIPConfigState, error) {
	ipconfig := newIPConfig(ipaddress, prefixLength)
	b, err := json.Marshal(orchestratorContext)
	return cns.ContainerIPConfigState{
		IPConfig:            ipconfig,
		ID:                  id,
		NCID:                ncid,
		State:               state,
		OrchestratorContext: b,
	}, err
}

// used to request an IPConfig from the CNS state
func (service *HTTPRestService) requestIPConfigHandler(w http.ResponseWriter, r *http.Request) {
	var (
		err             error
		ipconfigRequest cns.GetIPConfigRequest
		ipState         cns.ContainerIPConfigState
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
	reserveResp.IPConfiguration.IPSubnet = ipState.IPConfig

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

	if service.state.OrchestratorType != cns.Kubernetes {
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

func validateIPConfig(ipconfig cns.ContainerIPConfigState) error {
	if ipconfig.ID == "" {
		return fmt.Errorf("Failed to add IPConfig to state: %+v, empty ID", ipconfig)
	}
	if ipconfig.State == "" {
		return fmt.Errorf("Failed to add IPConfig to state: %+v, empty State", ipconfig)
	}
	if ipconfig.IPConfig.IPAddress == "" {
		return fmt.Errorf("Failed to add IPConfig to state: %+v, empty IPSubnet.IPAddress", ipconfig)
	}
	if ipconfig.IPConfig.PrefixLength == 0 {
		return fmt.Errorf("Failed to add IPConfig to state: %+v, empty IPSubnet.PrefixLength", ipconfig)
	}
	return nil
}

func (service *HTTPRestService) CreateOrUpdateNetworkContainerWithSecondaryIPConfigs(nc cns.CreateNetworkContainerRequest) error {
	//return service.addIPConfigsToState(nc.SecondaryIPConfigs)
	return nil
}

//AddIPConfigsToState takes a lock on the service object, and will add an array of ipconfigs to the CNS Service.
//Used to add IPConfigs to the CNS pool, specifically in the scenario of rebatching.
func (service *HTTPRestService) addIPConfigsToState(ipconfigs map[string]cns.ContainerIPConfigState) error {
	var (
		err      error
		ipconfig cns.ContainerIPConfigState
	)

	addedIPconfigs := make([]cns.ContainerIPConfigState, 0)

	service.Lock()

	defer func() {
		service.Unlock()

		if err != nil {
			if removeErr := service.removeIPConfigsFromState(addedIPconfigs); removeErr != nil {
				logger.Printf("Failed remove IPConfig after AddIpConfigs: %v", removeErr)
			}
		}
	}()

	// ensure the ipconfigs we are not attempting to overwrite existing ipconfig state
	existingIPConfigs := filterIPConfigMap(ipconfigs, func(ipconfig *cns.ContainerIPConfigState) bool {
		existingIPConfig, exists := service.PodIPConfigState[ipconfig.ID]
		if exists && existingIPConfig.State != ipconfig.State {
			return true
		}
		return false
	})
	if len(existingIPConfigs) > 0 {
		return fmt.Errorf("Failed to add IPConfigs to state, attempting to overwrite existing ipconfig states: %v", existingIPConfigs)
	}

	// add ipconfigs to state
	for _, ipconfig = range ipconfigs {
		if err = validateIPConfig(ipconfig); err != nil {
			return err
		}

		service.PodIPConfigState[ipconfig.ID] = ipconfig
		addedIPconfigs = append(addedIPconfigs, ipconfig)

		if ipconfig.State == cns.Allocated {
			var podInfo cns.KubernetesPodInfo

			if err = json.Unmarshal(ipconfig.OrchestratorContext, &podInfo); err != nil {
				return fmt.Errorf("Failed to add IPConfig to state: %+v with error: %v", ipconfig, err)
			}

			service.PodIPIDByOrchestratorContext[podInfo.GetOrchestratorContextKey()] = ipconfig.ID
		}
	}
	return err
}

func filterIPConfigMap(toBeAdded map[string]cns.ContainerIPConfigState, f func(*cns.ContainerIPConfigState) bool) []*cns.ContainerIPConfigState {
	vsf := make([]*cns.ContainerIPConfigState, 0)
	for _, v := range toBeAdded {
		if f(&v) {
			vsf = append(vsf, &v)
		}
	}
	return vsf
}

func (service *HTTPRestService) GetAllocatedIPConfigs() []*cns.ContainerIPConfigState {
	service.RLock()
	defer service.RUnlock()
	return filterIPConfigMap(service.PodIPConfigState, func(ipconfig *cns.ContainerIPConfigState) bool {
		return ipconfig.State == cns.Allocated
	})
}

func (service *HTTPRestService) GetAvailableIPConfigs() []*cns.ContainerIPConfigState {
	service.RLock()
	defer service.RUnlock()
	return filterIPConfigMap(service.PodIPConfigState, func(ipconfig *cns.ContainerIPConfigState) bool {
		return ipconfig.State == cns.Available
	})
}

//RemoveIPConfigsFromState takes a lock on the service object, and will remove an array of ipconfigs to the CNS Service.
//Used to add IPConfigs to the CNS pool, specifically in the scenario of rebatching.
func (service *HTTPRestService) removeIPConfigsFromState(ipconfigs []cns.ContainerIPConfigState) error {
	service.Lock()
	defer service.Unlock()

	for _, ipconfig := range ipconfigs {
		delete(service.PodIPConfigState, ipconfig.ID)
		var podInfo cns.KubernetesPodInfo
		err := json.Unmarshal(ipconfig.OrchestratorContext, &podInfo)

		// if batch delete failed return
		if err != nil {
			return err
		}

		delete(service.PodIPIDByOrchestratorContext, podInfo.GetOrchestratorContextKey())
	}
	return nil
}

//SetIPConfigAsAllocated takes a lock of the service, and sets the ipconfig in the CNS state as allocated, does not take a lock
func (service *HTTPRestService) setIPConfigAsAllocated(ipconfig cns.ContainerIPConfigState, podInfo cns.KubernetesPodInfo, marshalledOrchestratorContext json.RawMessage) cns.ContainerIPConfigState {
	ipconfig.State = cns.Allocated
	ipconfig.OrchestratorContext = marshalledOrchestratorContext
	service.PodIPIDByOrchestratorContext[podInfo.GetOrchestratorContextKey()] = ipconfig.ID
	service.PodIPConfigState[ipconfig.ID] = ipconfig
	return service.PodIPConfigState[ipconfig.ID]
}

//SetIPConfigAsAllocated and sets the ipconfig in the CNS state as allocated, does not take a lock
func (service *HTTPRestService) setIPConfigAsAvailable(ipconfig cns.ContainerIPConfigState, podInfo cns.KubernetesPodInfo) cns.ContainerIPConfigState {
	ipconfig.State = cns.Available
	ipconfig.OrchestratorContext = nil
	service.PodIPConfigState[ipconfig.ID] = ipconfig
	delete(service.PodIPIDByOrchestratorContext, podInfo.GetOrchestratorContextKey())
	return service.PodIPConfigState[ipconfig.ID]
}

////SetIPConfigAsAllocated takes a lock of the service, and sets the ipconfig in the CNS stateas Available
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

func (service *HTTPRestService) GetExistingIPConfig(podInfo cns.KubernetesPodInfo) (cns.ContainerIPConfigState, bool, error) {
	var (
		ipState cns.ContainerIPConfigState
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

func (service *HTTPRestService) AllocateDesiredIPConfig(podInfo cns.KubernetesPodInfo, desiredIPAddress string, orchestratorContext json.RawMessage) (cns.ContainerIPConfigState, error) {
	var ipState cns.ContainerIPConfigState

	service.Lock()
	defer service.Unlock()

	for _, ipState := range service.PodIPConfigState {
		if ipState.IPConfig.IPAddress == desiredIPAddress {
			if ipState.State == cns.Available {
				return service.setIPConfigAsAllocated(ipState, podInfo, orchestratorContext), nil
			}
			return ipState, fmt.Errorf("Desired IP has already been allocated")
		}
	}
	return ipState, fmt.Errorf("Requested IP not found in pool")
}

func (service *HTTPRestService) AllocateAnyAvailableIPConfig(podInfo cns.KubernetesPodInfo, orchestratorContext json.RawMessage) (cns.ContainerIPConfigState, error) {
	var ipState cns.ContainerIPConfigState

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
func requestIPConfigHelper(service *HTTPRestService, req cns.GetIPConfigRequest) (cns.ContainerIPConfigState, error) {
	var (
		podInfo cns.KubernetesPodInfo
		ipState cns.ContainerIPConfigState
		isExist bool
		err     error
	)

	if service.state.OrchestratorType != cns.Kubernetes {
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
