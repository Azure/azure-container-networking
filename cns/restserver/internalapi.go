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
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
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

func (service *HTTPRestService) SyncNodeStatus(dncEP, infraVnet, nodeID string, contextFromCNI json.RawMessage) (returnCode types.ResponseCode, errStr string) {
	logger.Printf("[Azure CNS] SyncNodeStatus")
	var (
		resp             *http.Response
		nodeInfoResponse cns.NodeInfoResponse
		body             []byte
		httpc            = common.GetHttpClient()
	)

	// try to retrieve NodeInfoResponse from mDNC
	url := fmt.Sprintf(common.SyncNodeNetworkContainersURLFmt, dncEP, infraVnet, nodeID, dncApiVersion)
	req, _ := http.NewRequestWithContext(context.TODO(), http.MethodGet, url, nil)
	resp, err := httpc.Do(req)
	if err == nil {
		if resp.StatusCode == http.StatusOK {
			err = json.NewDecoder(resp.Body).Decode(&nodeInfoResponse)
		} else {
			err = errors.Errorf("http err: %d", resp.StatusCode)
		}

		resp.Body.Close()
	}

	if err != nil {
		returnCode = types.UnexpectedError
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

	skipNCVersionCheck := false
	ctx, cancel := context.WithTimeout(context.Background(), nmaAPICallTimeout)
	defer cancel()
	ncVersionListResp, err := service.nma.GetNCVersionList(ctx)
	if err != nil {
		skipNCVersionCheck = true
		logger.Errorf("failed to get nc version list from nmagent")
	}

	if !skipNCVersionCheck {
		nmaNCs := map[string]string{}
		for _, nc := range ncVersionListResp.Containers {
			nmaNCs[cns.SwiftPrefix+nc.NetworkContainerID] = nc.Version
		}

		// check if the version is valid and save it to service state
		for ncid := range ncsToBeAdded {
			waitingForUpdate, _, _ := service.isNCWaitingForUpdate(ncsToBeAdded[ncid].Version, ncsToBeAdded[ncid].NetworkContainerid, nmaNCs)

			body, err = json.Marshal(ncsToBeAdded[ncid])
			if err != nil {
				logger.Errorf("[Azure-CNS] Failed to marshal nc with nc id %s and content %v", ncid, ncsToBeAdded[ncid])
			}
			req, err = http.NewRequestWithContext(ctx, http.MethodPost, "", bytes.NewBuffer(body))
			if err != nil {
				logger.Errorf("[Azure CNS] Error received while creating http POST request for nc %v", ncsToBeAdded[ncid])
			}
			req.Header.Set(common.ContentType, common.JsonContent)

			w := httptest.NewRecorder()
			service.createOrUpdateNetworkContainer(w, req)
			result := w.Result()
			if result.StatusCode == http.StatusOK {
				var resp cns.CreateNetworkContainerResponse
				if err = json.Unmarshal(w.Body.Bytes(), &resp); err == nil && resp.Response.ReturnCode == types.Success {
					service.Lock()
					ncstatus := service.state.ContainerStatus[ncid]
					ncstatus.VfpUpdateComplete = !waitingForUpdate
					service.state.ContainerStatus[ncid] = ncstatus
					service.Unlock()
				}
			}
			result.Body.Close()
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
	}
	return
}

// SyncHostNCVersion will check NC version from NMAgent and save it as host NC version in container status.
// If NMAgent NC version got updated, CNS will refresh the pending programming IP status.
func (service *HTTPRestService) SyncHostNCVersion(ctx context.Context, channelMode string) {
	service.Lock()
	defer service.Unlock()
	start := time.Now()
	err := service.syncHostNCVersion(ctx, channelMode)
	if err != nil {
		logger.Errorf("sync host error %v", err)
	}
	syncHostNCVersionCount.WithLabelValues(strconv.FormatBool(err == nil)).Inc()
	syncHostNCVersionLatency.WithLabelValues(strconv.FormatBool(err == nil)).Observe(time.Since(start).Seconds())
}

var errNonExistentContainerStatus = errors.New("nonExistantContainerstatus")

func (service *HTTPRestService) syncHostNCVersion(ctx context.Context, channelMode string) error {
	outdatedNCs := map[string]struct{}{}
	for idx := range service.state.ContainerStatus {
		// Will open a separate PR to convert all the NC version related variable to int. Change from string to int is a pain.
		localNCVersion, err := strconv.Atoi(service.state.ContainerStatus[idx].HostVersion)
		if err != nil {
			logger.Errorf("Received err when change containerstatus.HostVersion %s to int, err msg %v", service.state.ContainerStatus[idx].HostVersion, err)
			continue
		}
		dncNCVersion, err := strconv.Atoi(service.state.ContainerStatus[idx].CreateNetworkContainerRequest.Version)
		if err != nil {
			logger.Errorf("Received err when change nc version %s in containerstatus to int, err msg %v", service.state.ContainerStatus[idx].CreateNetworkContainerRequest.Version, err)
			continue
		}
		// host NC version is the NC version from NMAgent, if it's smaller than NC version from DNC, then append it to indicate it needs update.
		if localNCVersion < dncNCVersion {
			outdatedNCs[service.state.ContainerStatus[idx].ID] = struct{}{}
		} else if localNCVersion > dncNCVersion {
			logger.Errorf("NC version from NMAgent is larger than DNC, NC version from NMAgent is %d, NC version from DNC is %d", localNCVersion, dncNCVersion)
		}
	}
	if len(outdatedNCs) == 0 {
		return nil
	}
	ncVersionListResp, err := service.nma.GetNCVersionList(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get nc version list from nmagent")
	}

	nmaNCs := map[string]string{}
	for _, nc := range ncVersionListResp.Containers {
		nmaNCs[nc.NetworkContainerID] = nc.Version
	}
	for ncID := range outdatedNCs {
		nmaNCVersionStr, ok := nmaNCs[ncID]
		if !ok {
			// NMA doesn't have this NC that we need programmed yet, bail out
			continue
		}
		nmaNCVersion, err := strconv.Atoi(nmaNCVersionStr)
		if err != nil {
			logger.Errorf("failed to parse container version of %s: %s", ncID, err)
			continue
		}
		// Check whether it exist in service state and get the related nc info
		ncInfo, exist := service.state.ContainerStatus[ncID]
		if !exist {
			// if we marked this NC as needs update, but it no longer exists in internal state when we reach
			// this point, our internal state has changed unexpectedly and we should bail out and try again.
			return errors.Wrapf(errNonExistentContainerStatus, "can't find NC with ID %s in service state, stop updating this host NC version", ncID)
		}
		localNCVersion, err := strconv.Atoi(ncInfo.HostVersion)
		if err != nil {
			logger.Errorf("failed to parse host nc version string %s: %s", ncInfo.HostVersion, err)
			continue
		}
		if localNCVersion > nmaNCVersion {
			logger.Errorf("NC version from NMA is decreasing: have %d, got %d", localNCVersion, nmaNCVersion)
			continue
		}
		if channelMode == cns.CRD {
			service.MarkIpsAsAvailableUntransacted(ncInfo.ID, nmaNCVersion)
		}
		logger.Printf("Updating NC %s host version from %s to %s", ncID, ncInfo.HostVersion, nmaNCVersionStr)
		ncInfo.HostVersion = nmaNCVersionStr
		logger.Printf("Updated NC %s host version to %s", ncID, ncInfo.HostVersion)
		service.state.ContainerStatus[ncID] = ncInfo
		// if we successfully updated the NC, pop it from the needs update set.
		delete(outdatedNCs, ncID)
	}
	// if we didn't empty out the needs update set, NMA has not programmed all the NCs we are expecting, and we
	// need to return an error indicating that
	if len(outdatedNCs) > 0 {
		return errors.Errorf("unabled to update some NCs: %v, missing or bad response from NMA", outdatedNCs)
	}

	// if NMA has programmed all the NCs that we expect, we should write the CNI conflist. This will only be done
	// once per lifetime of the CNS process. This function is threadsafe and will panic if it fails, so it is safe
	// to call in a non-preemptable goroutine.
	go service.MustGenerateCNIConflistOnce()
	return nil
}

// This API will be called by CNS RequestController on CRD update.
func (service *HTTPRestService) ReconcileNCState(ncRequest *cns.CreateNetworkContainerRequest, podInfoByIP map[string]cns.PodInfo, nnc *v1alpha.NodeNetworkConfig) types.ResponseCode {
	logger.Printf("Reconciling NC state with podInfo %+v", podInfoByIP)
	// check if ncRequest is null, then return as there is no CRD state yet
	if ncRequest == nil {
		logger.Printf("CNS starting with no NC state, podInfoMap count %d", len(podInfoByIP))
		return types.Success
	}

	// If the NC was created successfully, then reconcile the assigned pod state
	returnCode := service.CreateOrUpdateNetworkContainerInternal(ncRequest)
	if returnCode != types.Success {
		return returnCode
	}

	// now parse the secondaryIP list, if it exists in PodInfo list, then assign that ip.
	for _, secIPConfig := range ncRequest.SecondaryIPConfigs {
		if podInfo, exists := podInfoByIP[secIPConfig.IPAddress]; exists {
			logger.Printf("SecondaryIP %+v is assigned to Pod. %+v, ncId: %s", secIPConfig, podInfo, ncRequest.NetworkContainerid)

			jsonContext, err := podInfo.OrchestratorContext()
			if err != nil {
				logger.Errorf("Failed to marshal KubernetesPodInfo, error: %v", err)
				return types.UnexpectedError
			}

			ipconfigRequest := cns.IPConfigRequest{
				DesiredIPAddresses:  []string{secIPConfig.IPAddress},
				OrchestratorContext: jsonContext,
				InfraContainerID:    podInfo.InfraContainerID(),
				PodInterfaceID:      podInfo.InterfaceID(),
			}

			if _, err := requestIPConfigHelper(service, ipconfigRequest); err != nil {
				logger.Errorf("AllocateIPConfig failed for SecondaryIP %+v, podInfo %+v, ncId %s, error: %v", secIPConfig, podInfo, ncRequest.NetworkContainerid, err)
				return types.FailedToAllocateIPConfig
			}
		} else {
			logger.Printf("SecondaryIP %+v is not assigned. ncId: %s", secIPConfig, ncRequest.NetworkContainerid)
		}
	}

	err := service.MarkExistingIPsAsPendingRelease(nnc.Spec.IPsNotInUse)
	if err != nil {
		logger.Errorf("[Azure CNS] Error. Failed to mark IPs as pending %v", nnc.Spec.IPsNotInUse)
		return types.UnexpectedError
	}

	return 0
}

// GetNetworkContainerInternal gets network container details.
func (service *HTTPRestService) GetNetworkContainerInternal(
	req cns.GetNetworkContainerRequest,
) (cns.GetNetworkContainerResponse, types.ResponseCode) {
	getNetworkContainerResponses := service.getAllNetworkContainerResponses(req)
	return getNetworkContainerResponses[0], getNetworkContainerResponses[0].Response.ReturnCode
}

// DeleteNetworkContainerInternal deletes a network container.
func (service *HTTPRestService) DeleteNetworkContainerInternal(
	req cns.DeleteNetworkContainerRequest,
) types.ResponseCode {
	ncid := req.NetworkContainerid
	_, exist := service.getNetworkContainerDetails(ncid)
	if !exist {
		logger.Printf("network container for id %v doesn't exist", ncid)
		return types.Success
	}

	service.Lock()
	defer service.Unlock()
	if service.state.ContainerStatus != nil {
		delete(service.state.ContainerStatus, ncid)
	}

	if service.state.ContainerIDByOrchestratorContext != nil {
		for orchestratorContext, networkContainerIDs := range service.state.ContainerIDByOrchestratorContext { //nolint:gocritic // copy is ok
			if networkContainerIDs.Contains(ncid) {
				networkContainerIDs.Delete(ncid)
				if *networkContainerIDs == "" {
					delete(service.state.ContainerIDByOrchestratorContext, orchestratorContext)
					break
				}
			}
		}
	}

	service.saveState()
	return types.Success
}

// This API will be called by CNS RequestController on CRD update.
func (service *HTTPRestService) CreateOrUpdateNetworkContainerInternal(req *cns.CreateNetworkContainerRequest) types.ResponseCode {
	if req.NetworkContainerid == "" {
		logger.Errorf("[Azure CNS] Error. NetworkContainerid is empty")
		return types.NetworkContainerNotSpecified
	}

	// For now only RequestController uses this API which will be initialized only for AKS scenario.
	// Validate ContainerType is set as Docker
	if service.state.OrchestratorType != cns.KubernetesCRD && service.state.OrchestratorType != cns.Kubernetes {
		logger.Errorf("[Azure CNS] Error. Unsupported OrchestratorType: %s", service.state.OrchestratorType)
		return types.UnsupportedOrchestratorType
	}

	// Validate PrimaryCA must never be empty
	err := validateIPSubnet(req.IPConfiguration.IPSubnet)
	if err != nil {
		logger.Errorf("[Azure CNS] Error. PrimaryCA is invalid, NC Req: %v", req)
		return types.InvalidPrimaryIPConfig
	}

	// Validate SecondaryIPConfig
	for _, secIPConfig := range req.SecondaryIPConfigs {
		// Validate Ipconfig
		if secIPConfig.IPAddress == "" {
			logger.Errorf("Failed to add IPConfig to state: %+v, empty IPSubnet.IPAddress", secIPConfig)
			return types.InvalidSecondaryIPConfig
		}
	}

	// Validate if state exists already
	existingNCInfo, ok := service.getNetworkContainerDetails(req.NetworkContainerid)
	if ok {
		existingReq := existingNCInfo.CreateNetworkContainerRequest
		if !reflect.DeepEqual(existingReq.IPConfiguration, req.IPConfiguration) {
			logger.Errorf("[Azure CNS] Error. PrimaryCA is not same, NCId %s, old CA %s, new CA %s", req.NetworkContainerid, existingReq.PrimaryInterfaceIdentifier, req.PrimaryInterfaceIdentifier)
			return types.PrimaryCANotSame
		}
	}

	// This will Create Or Update the NC state.
	returnCode, returnMessage := service.saveNetworkContainerGoalState(*req)

	// If the NC was created successfully, log NC snapshot.
	if returnCode == 0 {
		logNCSnapshot(*req)
	} else {
		logger.Errorf(returnMessage)
	}

	if service.Options[common.OptProgramSNATIPTables] == true {
		returnCode, returnMessage = service.programSNATRules(req)
		if returnCode != 0 {
			logger.Errorf(returnMessage)
		}
	}

	return returnCode
}
