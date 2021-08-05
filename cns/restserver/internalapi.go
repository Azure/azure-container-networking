// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/nmagentclient"
	"github.com/Azure/azure-container-networking/common"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

// This file contains the internal functions called by either HTTP APIs (api.go) or
// internal APIs (definde in internalapi.go).
// This will be used internally (say by RequestController in case of AKS)

// GetPartitionKey - Get dnc/service partition key
func (service *HTTPRestService) GetPartitionKey() (dncPartitionKey string) {
	service.RLock()
	dncPartitionKey = service.dncPartitionKey
	service.RUnlock()
	return
}

// SetNodeOrchestrator :- Set node orchestrator after registering with mDNC
func (service *HTTPRestService) SetNodeOrchestrator(r *cns.SetOrchestratorTypeRequest) {
	body, _ := json.Marshal(r)
	req, _ := http.NewRequest(http.MethodPost, "", bytes.NewBuffer(body))
	req.Header.Set(common.ContentType, common.JsonContent)
	service.setOrchestratorType(httptest.NewRecorder(), req)
}

// SyncNodeStatus :- Retrieve the latest node state from DNC & returns the first occurence of returnCode and error with respect to contextFromCNI
func (service *HTTPRestService) SyncNodeStatus(dncEP, infraVnet, nodeID string, contextFromCNI json.RawMessage) (returnCode int, errStr string) {
	logger.Printf("[Azure CNS] SyncNodeStatus")
	var (
		response         *http.Response
		err              error
		nodeInfoResponse cns.NodeInfoResponse
		req              *http.Request
		body             []byte
		httpc            = common.GetHttpClient()
	)

	// try to retrieve NodeInfoResponse from mDNC
	response, err = httpc.Get(fmt.Sprintf(common.SyncNodeNetworkContainersURLFmt, dncEP, infraVnet, nodeID, dncApiVersion))
	if err == nil {
		if response.StatusCode == http.StatusOK {
			err = json.NewDecoder(response.Body).Decode(&nodeInfoResponse)
		} else {
			err = fmt.Errorf("%d", response.StatusCode)
		}

		response.Body.Close()
	}

	if err != nil {
		returnCode = UnexpectedError
		errStr = fmt.Sprintf("[Azure-CNS] Failed to sync node with error: %+v", err)
		logger.Errorf(errStr)
		return
	}

	var (
		ncsToBeAdded   = make(map[string]cns.CreateNetworkContainerRequest)
		ncsToBeDeleted = make(map[string]bool)
	)

	// determine new NCs and NCs to be deleted
	service.RLock()
	for ncid := range service.state.ContainerStatus {
		ncsToBeDeleted[ncid] = true
	}

	for _, nc := range nodeInfoResponse.NetworkContainers {
		ncid := nc.NetworkContainerid
		delete(ncsToBeDeleted, ncid)
		if savedNc, exists := service.state.ContainerStatus[ncid]; !exists || savedNc.CreateNetworkContainerRequest.Version < nc.Version {
			ncsToBeAdded[ncid] = nc
		}
	}
	service.RUnlock()

	// check if the version is valid and save it to service state
	for ncid, nc := range ncsToBeAdded {
		var (
			versionURL = fmt.Sprintf(nmagentclient.GetNetworkContainerVersionURLFmt,
				nmagentclient.WireserverIP,
				nc.PrimaryInterfaceIdentifier,
				nc.NetworkContainerid,
				nc.AuthorizationToken)
			w = httptest.NewRecorder()
		)

		ncVersionURLs.Store(nc.NetworkContainerid, versionURL)
		waitingForUpdate, _, _ := service.isNCWaitingForUpdate(nc.Version, nc.NetworkContainerid)

		body, _ = json.Marshal(nc)
		req, _ = http.NewRequest(http.MethodPost, "", bytes.NewBuffer(body))
		req.Header.Set(common.ContentType, common.JsonContent)
		service.createOrUpdateNetworkContainer(w, req)
		if w.Result().StatusCode == http.StatusOK {
			var resp cns.CreateNetworkContainerResponse
			if err = json.Unmarshal(w.Body.Bytes(), &resp); err == nil && resp.Response.ReturnCode == Success {
				service.Lock()
				ncstatus, _ := service.state.ContainerStatus[ncid]
				ncstatus.VfpUpdateComplete = !waitingForUpdate
				service.state.ContainerStatus[ncid] = ncstatus
				service.Unlock()
			}
		}
	}

	service.Lock()
	service.saveState()
	service.Unlock()

	// delete dangling NCs
	for nc := range ncsToBeDeleted {
		var body bytes.Buffer
		json.NewEncoder(&body).Encode(&cns.DeleteNetworkContainerRequest{NetworkContainerid: nc})

		req, err = http.NewRequest(http.MethodPost, "", &body)
		if err == nil {
			req.Header.Set(common.JsonContent, common.JsonContent)
			service.deleteNetworkContainer(httptest.NewRecorder(), req)
		} else {
			logger.Errorf("[Azure-CNS] Failed to delete NC request to sync state: %s", err.Error())
		}

		ncVersionURLs.Delete(nc)
	}

	return
}

// SyncHostNCVersion will check NC version from NMAgent and save it as host NC version in container status.
// If NMAgent NC version got updated, CNS will refresh the pending programming IP status.
func (service *HTTPRestService) SyncHostNCVersion(ctx context.Context, channelMode string, syncHostNCTimeoutMilliSec time.Duration) {
	var hostVersionNeedUpdateNcList []string
	service.RLock()
	for _, containerstatus := range service.state.ContainerStatus {
		// Will open a separate PR to convert all the NC version related variable to int. Change from string to int is a pain.
		hostVersion, err := strconv.Atoi(containerstatus.HostVersion)
		if err != nil {
			logger.Errorf("Received err when change containerstatus.HostVersion %s to int, err msg %v", containerstatus.HostVersion, err)
			continue
		}
		dncNcVersion, err := strconv.Atoi(containerstatus.CreateNetworkContainerRequest.Version)
		if err != nil {
			logger.Errorf("Received err when change nc version %s in containerstatus to int, err msg %v", containerstatus.CreateNetworkContainerRequest.Version, err)
			continue
		}
		// host NC version is the NC version from NMAgent, if it's smaller than NC version from DNC, then append it to indicate it needs update.
		if hostVersion < dncNcVersion {
			hostVersionNeedUpdateNcList = append(hostVersionNeedUpdateNcList, containerstatus.ID)
		} else if hostVersion > dncNcVersion {
			logger.Errorf("NC version from NMAgent is larger than DNC, NC version from NMAgent is %d, NC version from DNC is %d", hostVersion, dncNcVersion)
		}
	}
	service.RUnlock()
	if len(hostVersionNeedUpdateNcList) > 0 {
		logger.Printf("Updating version of the following NC IDs: %v", hostVersionNeedUpdateNcList)
		ncVersionChannel := make(chan map[string]int)
		ctxWithTimeout, _ := context.WithTimeout(ctx, syncHostNCTimeoutMilliSec*time.Millisecond)
		go func() {
			ncVersionChannel <- service.nmagentClient.GetNcVersionListWithOutToken(hostVersionNeedUpdateNcList)
			close(ncVersionChannel)
		}()
		select {
		case newHostNCVersionList := <-ncVersionChannel:
			if newHostNCVersionList == nil {
				logger.Errorf("Can't get vfp programmed NC version list from url without token")
			} else {
				service.Lock()
				for ncID, newHostNCVersion := range newHostNCVersionList {
					// Check whether it exist in service state and get the related nc info
					if ncInfo, exist := service.state.ContainerStatus[ncID]; !exist {
						logger.Errorf("Can't find NC with ID %s in service state, stop updating this host NC version", ncID)
					} else {
						if channelMode == cns.CRD {
							service.MarkIpsAsAvailableUntransacted(ncInfo.ID, newHostNCVersion)
						}
						oldHostNCVersion := ncInfo.HostVersion
						ncInfo.HostVersion = strconv.Itoa(newHostNCVersion)
						service.state.ContainerStatus[ncID] = ncInfo
						logger.Printf("Updated NC %s host version from %s to %s", ncID, oldHostNCVersion, ncInfo.HostVersion)
					}
				}
				service.Unlock()
			}
		case <-ctxWithTimeout.Done():
			logger.Errorf("Timeout when getting vfp programmed NC version list from url without token")
		}
	}
}

// This API will be called by CNS RequestController on CRD update.
func (service *HTTPRestService) ReconcileNCState(ncRequest *cns.CreateNetworkContainerRequest, podInfoByIp map[string]cns.PodInfo, scalar nnc.Scaler, spec nnc.NodeNetworkConfigSpec) int {
	logger.Printf("Reconciling NC state with podInfo %+v", podInfoByIp)
	// check if ncRequest is null, then return as there is no CRD state yet
	if ncRequest == nil {
		logger.Printf("CNS starting with no NC state, podInfoMap count %d", len(podInfoByIp))
		return Success
	}

	// If the NC was created successfully, then reconcile the allocated pod state
	returnCode := service.CreateOrUpdateNetworkContainerInternal(*ncRequest)
	if returnCode != Success {
		return returnCode
	}
	returnCode = service.UpdateIPAMPoolMonitorInternal(scalar, spec)
	if returnCode != Success {
		return returnCode
	}

	// now parse the secondaryIP list, if it exists in PodInfo list, then allocate that ip
	for _, secIpConfig := range ncRequest.SecondaryIPConfigs {
		if podInfo, exists := podInfoByIp[secIpConfig.IPAddress]; exists {
			logger.Printf("SecondaryIP %+v is allocated to Pod. %+v, ncId: %s", secIpConfig, podInfo, ncRequest.NetworkContainerid)

			jsonContext, err := podInfo.OrchestratorContext()
			if err != nil {
				logger.Errorf("Failed to marshal KubernetesPodInfo, error: %v", err)
				return UnexpectedError
			}

			ipconfigRequest := cns.IPConfigRequest{
				DesiredIPAddress:    secIpConfig.IPAddress,
				OrchestratorContext: jsonContext,
				PodInterfaceID:      podInfo.InterfaceID(),
				InfraContainerID:    podInfo.InfraContainerID(),
			}

			if _, err := requestIPConfigHelper(service, ipconfigRequest); err != nil {
				logger.Errorf("AllocateIPConfig failed for SecondaryIP %+v, podInfo %+v, ncId %s, error: %v", secIpConfig, podInfo, ncRequest.NetworkContainerid, err)
				return FailedToAllocateIpConfig
			}
		} else {
			logger.Printf("SecondaryIP %+v is not allocated. ncId: %s", secIpConfig, ncRequest.NetworkContainerid)
		}
	}

	err := service.MarkExistingIPsAsPending(spec.IPsNotInUse)
	if err != nil {
		logger.Errorf("[Azure CNS] Error. Failed to mark IP's as pending %v", spec.IPsNotInUse)
		return UnexpectedError
	}

	return 0
}

// GetNetworkContainerInternal gets network container details.
func (service *HTTPRestService) GetNetworkContainerInternal(req cns.GetNetworkContainerRequest) (cns.GetNetworkContainerResponse, int) {
	getNetworkContainerResponse := service.getNetworkContainerResponse(req)
	returnCode := getNetworkContainerResponse.Response.ReturnCode
	return getNetworkContainerResponse, returnCode
}

// DeleteNetworkContainerInternal deletes a network container.
func (service *HTTPRestService) DeleteNetworkContainerInternal(req cns.DeleteNetworkContainerRequest) int {
	_, exist := service.getNetworkContainerDetails(req.NetworkContainerid)
	if !exist {
		logger.Printf("network container for id %v doesn't exist", req.NetworkContainerid)
		return Success
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
	return Success
}

// This API will be called by CNS RequestController on CRD update.
func (service *HTTPRestService) CreateOrUpdateNetworkContainerInternal(req cns.CreateNetworkContainerRequest) int {
	if req.NetworkContainerid == "" {
		logger.Errorf("[Azure CNS] Error. NetworkContainerid is empty")
		return NetworkContainerNotSpecified
	}

	// For now only RequestController uses this API which will be initialized only for AKS scenario.
	// Validate ContainerType is set as Docker
	if service.state.OrchestratorType != cns.KubernetesCRD && service.state.OrchestratorType != cns.Kubernetes {
		logger.Errorf("[Azure CNS] Error. Unsupported OrchestratorType: %s", service.state.OrchestratorType)
		return UnsupportedOrchestratorType
	}

	// Validate PrimaryCA must never be empty
	err := validateIPSubnet(req.IPConfiguration.IPSubnet)
	if err != nil {
		logger.Errorf("[Azure CNS] Error. PrimaryCA is invalid, NC Req: %v", req)
		return InvalidPrimaryIPConfig
	}

	// Validate SecondaryIPConfig
	for _, secIpconfig := range req.SecondaryIPConfigs {
		// Validate Ipconfig
		if secIpconfig.IPAddress == "" {
			logger.Errorf("Failed to add IPConfig to state: %+v, empty IPSubnet.IPAddress", secIpconfig)
			return InvalidSecondaryIPConfig
		}
	}

	// Validate if state exists already
	existingNCInfo, ok := service.getNetworkContainerDetails(req.NetworkContainerid)
	if ok {
		existingReq := existingNCInfo.CreateNetworkContainerRequest
		if reflect.DeepEqual(existingReq.IPConfiguration, req.IPConfiguration) != true {
			logger.Errorf("[Azure CNS] Error. PrimaryCA is not same, NCId %s, old CA %s, new CA %s", req.NetworkContainerid, existingReq.PrimaryInterfaceIdentifier, req.PrimaryInterfaceIdentifier)
			return PrimaryCANotSame
		}
	}

	// This will Create Or Update the NC state.
	returnCode, returnMessage := service.saveNetworkContainerGoalState(req)

	// If the NC was created successfully, log NC snapshot.
	if returnCode == 0 {
		logNCSnapshot(req)
	} else {
		logger.Errorf(returnMessage)
	}

	return returnCode
}

func (service *HTTPRestService) UpdateIPAMPoolMonitorInternal(scalar nnc.Scaler, spec nnc.NodeNetworkConfigSpec) int {
	if err := service.IPAMPoolMonitor.Update(scalar, spec); err != nil {
		logger.Errorf("[cns-rc] Error creating or updating IPAM Pool Monitor: %v", err)
		// requeue
		return UnexpectedError
	}

	return 0
}
