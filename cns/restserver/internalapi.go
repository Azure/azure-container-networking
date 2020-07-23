// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/nmagentclient"
	"github.com/Azure/azure-container-networking/common"
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
		ncid := cns.SwiftPrefix + nc.NetworkContainerid
		delete(ncsToBeDeleted, ncid)
		if savedNc, exists := service.state.ContainerStatus[ncid]; !exists || savedNc.CreateNetworkContainerRequest.Version < nc.Version {
			ncsToBeAdded[ncid] = nc
		}
	}
	service.RUnlock()

	// check if the version is valid and save it to service state
	for ncid, nc := range ncsToBeAdded {
		var (
			versionURL = fmt.Sprintf(nodeInfoResponse.GetNCVersionURLFmt,
				nmagentclient.WireserverIP,
				nc.PrimaryInterfaceIdentifier,
				nc.NetworkContainerid,
				nc.AuthorizationToken)
			w = httptest.NewRecorder()
		)

		ncVersionURLs.Store(nc.NetworkContainerid, versionURL)
		waitingForUpdate, tmpReturnCode, tmpErrStr := isNCWaitingForUpdate(nc.Version, nc.NetworkContainerid)
		if tmpReturnCode != Success && bytes.Compare(nc.OrchestratorContext, contextFromCNI) == 0 {
			returnCode = tmpReturnCode
			errStr = tmpErrStr
		}

		if tmpReturnCode == UnexpectedError {
			continue
		}

		body, _ = json.Marshal(nc)
		req, _ = http.NewRequest(http.MethodPost, "", bytes.NewBuffer(body))
		req.Header.Set(common.ContentType, common.JsonContent)
		service.createOrUpdateNetworkContainer(w, req)
		if w.Result().StatusCode == http.StatusOK {
			var resp cns.CreateNetworkContainerResponse
			if err = json.Unmarshal(w.Body.Bytes(), &resp); err == nil && resp.Response.ReturnCode == Success {
				service.Lock()
				ncstatus, _ := service.state.ContainerStatus[ncid]
				ncstatus.WaitingForUpdate = waitingForUpdate
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

// This API will be called by CNS RequestController on CRD update.
func (service *HTTPRestService) CreateOrUpdateNetworkContainerInternal(req cns.CreateNetworkContainerRequest) int {
	if req.NetworkContainerid == "" {
		logger.Errorf("[Azure CNS] Error. NetworkContainerid is empty")
		return NetworkContainerNotSpecified
	}

	// For now only RequestController uses this API which will be initialized only for AKS scenario.
	// Validate ContainerType is set as Docker
	if service.state.OrchestratorType != cns.KubernetesCRD {
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
	for ipId, secIpconfig := range req.SecondaryIPConfigs {
		// Validate Ipconfig
		err := validateIPSubnet(secIpconfig.IPSubnet)
		if err != nil {
			logger.Errorf("[Azure CNS] Error. SecondaryIpConfig, Id:%s is invalid, SecondaryIPConfig: %v, ncId: %s", ipId, secIpconfig, req.NetworkContainerid)
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
