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

func NewPodState(ipaddress string, prefixLength uint8, id, ncid, state string) *cns.ContainerIPConfigState {
	ipconfig := newIPConfig(ipaddress, prefixLength)

	return &cns.ContainerIPConfigState{
		IPConfig: ipconfig,
		ID:       id,
		NCID:     ncid,
		State:    state,
	}
}

func NewPodStateWithOrchestratorContext(ipaddress string, prefixLength uint8, id, ncid, state string, orchestratorContext cns.KubernetesPodInfo) (*cns.ContainerIPConfigState, error) {
	ipconfig := newIPConfig(ipaddress, prefixLength)
	b, err := json.Marshal(orchestratorContext)
	return &cns.ContainerIPConfigState{
		IPConfig:            ipconfig,
		ID:                  id,
		NCID:                ncid,
		State:               state,
		OrchestratorContext: b,
	}, err
}

//AddIPConfigsToState takes a lock on the service object, and will add an array of ipconfigs to the CNS Service.
//Used to add IPConfigs to the CNS pool, specifically in the scenario of rebatching.
func (service *HTTPRestService) AddIPConfigsToState(ipconfigs []*cns.ContainerIPConfigState) error {
	service.Lock()
	defer service.Unlock()

	for i, ipconfig := range ipconfigs {
		service.PodIPConfigState[ipconfig.ID] = ipconfig

		if ipconfig.State == cns.Allocated {
			var podInfo cns.KubernetesPodInfo

			if err := json.Unmarshal(ipconfig.OrchestratorContext, &podInfo); err != nil {

				if err := service.RemoveIPConfigsFromState(ipconfigs[0:i]); err != nil {
					return fmt.Errorf("Failed remove IPConfig after AddIpConfigs: %v", err)
				}

				return fmt.Errorf("Failed to add IPConfig to state: %+v with error: %v", ipconfig, err)
			}

			service.PodIPIDByOrchestratorContext[podInfo.GetOrchestratorContextKey()] = ipconfig.ID
		}
	}

	return nil
}

//RemoveIPConfigsFromState takes a lock on the service object, and will remove an array of ipconfigs to the CNS Service.
//Used to add IPConfigs to the CNS pool, specifically in the scenario of rebatching.
func (service *HTTPRestService) RemoveIPConfigsFromState(ipconfigs []*cns.ContainerIPConfigState) error {
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
func (service *HTTPRestService) setIPConfigAsAllocated(ipconfig *cns.ContainerIPConfigState, podInfo cns.KubernetesPodInfo, marshalledOrchestratorContext json.RawMessage) *cns.ContainerIPConfigState {
	ipconfig.State = cns.Allocated
	ipconfig.OrchestratorContext = marshalledOrchestratorContext
	service.PodIPIDByOrchestratorContext[podInfo.GetOrchestratorContextKey()] = ipconfig.ID
	service.PodIPConfigState[ipconfig.ID] = ipconfig
	return service.PodIPConfigState[ipconfig.ID]
}

//SetIPConfigAsAllocated and sets the ipconfig in the CNS state as allocated, does not take a lock
func (service *HTTPRestService) setIPConfigAsAvailable(ipconfig *cns.ContainerIPConfigState, podInfo cns.KubernetesPodInfo) *cns.ContainerIPConfigState {
	ipconfig.State = cns.Available
	ipconfig.OrchestratorContext = nil
	service.PodIPConfigState[ipconfig.ID] = ipconfig
	service.PodIPIDByOrchestratorContext[podInfo.GetOrchestratorContextKey()] = ""
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
			return fmt.Errorf("Pod->IPIP exists but IPID to IPConfig doesn't exist")
		}
	} else {
		return fmt.Errorf("SetIPConfigAsAvailable failed to release, no allocation found for pod")
	}
	return nil
}

func (service *HTTPRestService) GetExistingIPConfig(podInfo cns.KubernetesPodInfo) (*cns.ContainerIPConfigState, bool, error) {
	var (
		ipState *cns.ContainerIPConfigState
		isExist bool
		err     error
	)

	service.RLock()
	defer service.RUnlock()

	ipID := service.PodIPIDByOrchestratorContext[podInfo.GetOrchestratorContextKey()]
	if ipID != "" {
		if ipState, isExist = service.PodIPConfigState[ipID]; isExist {
			return ipState, isExist, nil
		}
		return ipState, isExist, fmt.Errorf("Pod->IPIP exists but IPID to IPConfig doesn't exist")
	}

	return ipState, isExist, err
}

func (service *HTTPRestService) GetDesiredIPConfig(podInfo cns.KubernetesPodInfo, desiredIPAddress string, orchestratorContext json.RawMessage) (*cns.ContainerIPConfigState, error) {
	var ipState *cns.ContainerIPConfigState

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

func (service *HTTPRestService) GetAnyAvailableIPConfig(podInfo cns.KubernetesPodInfo, orchestratorContext json.RawMessage) (*cns.ContainerIPConfigState, error) {
	var ipState *cns.ContainerIPConfigState

	service.Lock()
	defer service.Unlock()

	for _, ipState = range service.PodIPConfigState {
		if ipState.State == cns.Available {
			return service.setIPConfigAsAllocated(ipState, podInfo, orchestratorContext), nil
		}
	}
	return ipState, fmt.Errorf("No more free IP's available, trigger batch")
}

// cni -> allocate ipconfig
// 			|- fetch nc from state by constructing nc id
func (service *HTTPRestService) requestIPConfigHandler(w http.ResponseWriter, r *http.Request) {
	var (
		err           error
		ncrequest     cns.GetNetworkContainerRequest
		ipState       *cns.ContainerIPConfigState
		returnCode    int
		returnMessage string
	)

	err = service.Listener.Decode(w, r, &ncrequest)
	logger.Request(service.Name, &ncrequest, err)
	if err != nil {
		return
	}

	// retrieve ipconfig from nc
	if ipState, err = requestIPConfigHelper(service, ncrequest); err != nil {
		returnCode = UnexpectedError
		returnMessage = fmt.Sprintf("AllocateIPConfig failed: %v", err)
	}

	resp := cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}

	reserveResp := &cns.GetNetworkContainerResponse{
		Response: resp,
	}
	reserveResp.IPConfiguration.IPSubnet = ipState.IPConfig

	err = service.Listener.Encode(w, &reserveResp)
	logger.Response(service.Name, reserveResp, resp.ReturnCode, ReturnCodeToString(resp.ReturnCode), err)
}

func (service *HTTPRestService) releaseIPConfigHandler(w http.ResponseWriter, r *http.Request) {
	var (
		podInfo    cns.KubernetesPodInfo
		req        cns.GetNetworkContainerRequest
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

// If IPConfig is already allocated for pod, it returns that else it returns one of the available ipconfigs.
func requestIPConfigHelper(service *HTTPRestService, req cns.GetNetworkContainerRequest) (*cns.ContainerIPConfigState, error) {
	var (
		podInfo cns.KubernetesPodInfo
		ipState *cns.ContainerIPConfigState
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
	if ipState, isExist, err = service.GetExistingIPConfig(podInfo); err != nil || isExist {
		return ipState, err
	}

	// return desired IPConfig
	if req.DesiredIPConfig.IPAddress != "" {
		return service.GetDesiredIPConfig(podInfo, req.DesiredIPConfig.IPAddress, req.OrchestratorContext)
	}

	// return any free IPConfig
	return service.GetAnyAvailableIPConfig(podInfo, req.OrchestratorContext)
}
