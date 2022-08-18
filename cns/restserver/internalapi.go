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
	"github.com/Azure/azure-container-networking/cns/nmagent"
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

// SyncNodeStatus :- Retrieve the latest node state from DNC & returns the first occurence of returnCode and error with respect to contextFromCNI
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

	// check if the version is valid and save it to service state
	for ncid, nc := range ncsToBeAdded {
		var (
			versionURL = fmt.Sprintf(nmagent.GetNetworkContainerVersionURLFmt,
				nmagent.WireserverIP,
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
			if err = json.Unmarshal(w.Body.Bytes(), &resp); err == nil && resp.Response.ReturnCode == types.Success {
				service.Lock()
				ncstatus := service.state.ContainerStatus[ncid]
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
func (service *HTTPRestService) SyncHostNCVersion(ctx context.Context, channelMode string) {
	service.Lock()
	defer service.Unlock()
	start := time.Now()
	err := service.syncHostNCVersion(ctx, channelMode)
	if err != nil {
		logger.Errorf("sync host error %v", err)
	}
	syncHostNCVersion.WithLabelValues(strconv.FormatBool(err == nil)).Observe(time.Since(start).Seconds())
}

var errNonExistentContainerStatus = errors.New("nonExistantContainerstatus")

func (service *HTTPRestService) syncHostNCVersion(ctx context.Context, channelMode string) error {
	var hostVersionNeedsUpdateContainers []string
	for idx := range service.state.ContainerStatus {
		// Will open a separate PR to convert all the NC version related variable to int. Change from string to int is a pain.
		hostVersion, err := strconv.Atoi(service.state.ContainerStatus[idx].HostVersion)
		if err != nil {
			logger.Errorf("Received err when change containerstatus.HostVersion %s to int, err msg %v", service.state.ContainerStatus[idx].HostVersion, err)
			continue
		}
		dncNcVersion, err := strconv.Atoi(service.state.ContainerStatus[idx].CreateNetworkContainerRequest.Version)
		if err != nil {
			logger.Errorf("Received err when change nc version %s in containerstatus to int, err msg %v", service.state.ContainerStatus[idx].CreateNetworkContainerRequest.Version, err)
			continue
		}
		// host NC version is the NC version from NMAgent, if it's smaller than NC version from DNC, then append it to indicate it needs update.
		if hostVersion < dncNcVersion {
			hostVersionNeedsUpdateContainers = append(hostVersionNeedsUpdateContainers, service.state.ContainerStatus[idx].ID)
		} else if hostVersion > dncNcVersion {
			logger.Errorf("NC version from NMAgent is larger than DNC, NC version from NMAgent is %d, NC version from DNC is %d", hostVersion, dncNcVersion)
		}
	}
	if len(hostVersionNeedsUpdateContainers) == 0 {
		return nil
	}
	ncList, err := service.nmagentClient.GetNCVersionList(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get nc version list from nmagent")
	}

	newHostNCVersionList := map[string]string{}
	for _, nc := range ncList.Containers {
		newHostNCVersionList[nc.NetworkContainerID] = nc.Version
	}
	for _, ncID := range hostVersionNeedsUpdateContainers {
		versionStr, ok := newHostNCVersionList[ncID]
		if !ok {
			continue
		}
		version, err := strconv.Atoi(versionStr)
		if err != nil {
			return errors.Wrapf(err, "failed to parse container version of %s", ncID)
		}

		// Check whether it exist in service state and get the related nc info
		ncInfo, exist := service.state.ContainerStatus[ncID]
		if !exist {
			return errors.Wrapf(errNonExistentContainerStatus, "can't find NC with ID %s in service state, stop updating this host NC version", ncID)
		}
		if channelMode == cns.CRD {
			service.MarkIpsAsAvailableUntransacted(ncInfo.ID, version)
		}
		oldHostNCVersion := ncInfo.HostVersion
		ncInfo.HostVersion = versionStr
		service.state.ContainerStatus[ncID] = ncInfo
		logger.Printf("Updated NC %s host version from %s to %s", ncID, oldHostNCVersion, ncInfo.HostVersion)
	}
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
	for _, secIpConfig := range ncRequest.SecondaryIPConfigs {
		if podInfo, exists := podInfoByIP[secIpConfig.IPAddress]; exists {
			logger.Printf("SecondaryIP %+v is assigned to Pod. %+v, ncId: %s", secIpConfig, podInfo, ncRequest.NetworkContainerid)

			jsonContext, err := podInfo.OrchestratorContext()
			if err != nil {
				logger.Errorf("Failed to marshal KubernetesPodInfo, error: %v", err)
				return types.UnexpectedError
			}

			ipconfigRequest := cns.IPConfigRequest{
				DesiredIPAddress:    secIpConfig.IPAddress,
				OrchestratorContext: jsonContext,
				InfraContainerID:    podInfo.InfraContainerID(),
				PodInterfaceID:      podInfo.InterfaceID(),
			}

			if _, err := requestIPConfigHelper(service, ipconfigRequest); err != nil {
				logger.Errorf("AllocateIPConfig failed for SecondaryIP %+v, podInfo %+v, ncId %s, error: %v", secIpConfig, podInfo, ncRequest.NetworkContainerid, err)
				return types.FailedToAllocateIPConfig
			}
		} else {
			logger.Printf("SecondaryIP %+v is not assigned. ncId: %s", secIpConfig, ncRequest.NetworkContainerid)
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
	getNetworkContainerResponse := service.getNetworkContainerResponse(req)
	returnCode := getNetworkContainerResponse.Response.ReturnCode
	return getNetworkContainerResponse, returnCode
}

// DeleteNetworkContainerInternal deletes a network container.
func (service *HTTPRestService) DeleteNetworkContainerInternal(
	req cns.DeleteNetworkContainerRequest,
) types.ResponseCode {
	_, exist := service.getNetworkContainerDetails(req.NetworkContainerid)
	if !exist {
		logger.Printf("network container for id %v doesn't exist", req.NetworkContainerid)
		return types.Success
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
	for _, secIpconfig := range req.SecondaryIPConfigs {
		// Validate Ipconfig
		if secIpconfig.IPAddress == "" {
			logger.Errorf("Failed to add IPConfig to state: %+v, empty IPSubnet.IPAddress", secIpconfig)
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
