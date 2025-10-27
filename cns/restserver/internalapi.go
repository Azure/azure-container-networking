// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/nodesubnet"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
)

const (
	// Known API names we care about
	expectedIMDSAPIVersion = "2025-07-24"
	PrefixOnNicNCVersion   = "1"
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
			nmaNCs[strings.TrimPrefix(lowerCaseNCGuid(nc.NetworkContainerID), cns.SwiftPrefix)] = nc.Version
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
	programmedNCCount, err := service.syncHostNCVersion(ctx, channelMode)
	// even if we get an error, we want to write the CNI conflist if we have any NC programmed to any version
	if programmedNCCount > 0 {
		// This will only be done once per lifetime of the CNS process. This function is threadsafe and will panic
		// if it fails, so it is safe to call in a non-preemptable goroutine.
		go service.MustGenerateCNIConflistOnce()
	}
	if err != nil {
		logger.Errorf("sync host error %v", err)
	}
	syncHostNCVersionCount.WithLabelValues(strconv.FormatBool(err == nil)).Inc()
	syncHostNCVersionLatency.WithLabelValues(strconv.FormatBool(err == nil)).Observe(time.Since(start).Seconds())
}

var errNonExistentContainerStatus = errors.New("nonExistantContainerstatus")

// syncHostVersion updates the CNS state with the latest programmed versions of NCs attached to the VM. If any NC in local CNS state
// does not match the version that DNC claims to have published, this function will call NMAgent and list the latest programmed versions of
// all NCs and update the CNS state accordingly. This function returns the the total number of NCs on this VM that have been programmed to
// some version, NOT the number of NCs that are up-to-date.
func (service *HTTPRestService) syncHostNCVersion(ctx context.Context, channelMode string) (int, error) {
	outdatedNCs := map[string]struct{}{}
	programmedNCs := map[string]struct{}{}
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

		if localNCVersion > -1 {
			programmedNCs[service.state.ContainerStatus[idx].ID] = struct{}{}
		}
	}
	if len(outdatedNCs) == 0 {
		return len(programmedNCs), nil
	}

	ncVersionListResp, err := service.nma.GetNCVersionList(ctx)
	if err != nil {
		return len(programmedNCs), errors.Wrap(err, "failed to get nc version list from nmagent")
	}

	// Get IMDS NC versions for delegated NIC scenarios
	imdsNCVersions, err := service.GetIMDSNCs(ctx)
	if err != nil {
		// If any of the NMA API check calls, imds calls fails assume that nma build doesn't have the latest changes and create empty map
		imdsNCVersions = make(map[string]string)
	}

	nmaNCs := map[string]string{}
	for _, nc := range ncVersionListResp.Containers {
		nmaNCs[strings.ToLower(nc.NetworkContainerID)] = nc.Version
	}

	// Consolidate both nc's from NMA and IMDS calls
	nmaProgrammedNCs := make(map[string]string)
	for ncID, version := range nmaNCs {
		nmaProgrammedNCs[ncID] = version
	}
	for ncID, version := range imdsNCVersions {
		if _, exists := nmaProgrammedNCs[ncID]; !exists {
			nmaProgrammedNCs[strings.ToLower(ncID)] = version
		} else {
			//nolint:staticcheck // SA1019: suppress deprecated logger.Warnf usage. Todo: legacy logger usage is consistent in cns repo. Migrates when all logger usage is migrated
			logger.Warnf("NC %s exists in both NMA and IMDS responses, which is not expected", ncID)
		}
	}
	hasNC.Set(float64(len(nmaProgrammedNCs)))
	for ncID := range outdatedNCs {
		nmaProgrammedNCVersionStr, ok := nmaProgrammedNCs[ncID]
		if !ok {
			// Neither NMA nor IMDS has this NC that we need programmed yet, bail out
			continue
		}
		nmaProgrammedNCVersion, err := strconv.Atoi(nmaProgrammedNCVersionStr)
		if err != nil {
			logger.Errorf("failed to parse container version of %s: %s", ncID, err)
			continue
		}
		// Check whether it exist in service state and get the related nc info
		ncInfo, exist := service.state.ContainerStatus[ncID]
		if !exist {
			// if we marked this NC as needs update, but it no longer exists in internal state when we reach
			// this point, our internal state has changed unexpectedly and we should bail out and try again.
			return len(programmedNCs), errors.Wrapf(errNonExistentContainerStatus, "can't find NC with ID %s in service state, stop updating this host NC version", ncID)
		}
		// if the NC still exists in state and is programmed to some version (doesn't have to be latest), add it to our set of NCs that have been programmed
		if nmaProgrammedNCVersion > -1 {
			programmedNCs[ncID] = struct{}{}
		}

		localNCVersion, err := strconv.Atoi(ncInfo.HostVersion)
		if err != nil {
			logger.Errorf("failed to parse host nc version string %s: %s", ncInfo.HostVersion, err)
			continue
		}
		if localNCVersion > nmaProgrammedNCVersion {
			//nolint:staticcheck // SA1019: suppress deprecated logger.Printf usage. Todo: legacy logger usage is consistent in cns repo. Migrates when all logger usage is migrated
			logger.Errorf("NC version from consolidated sources is decreasing: have %d, got %d", localNCVersion, nmaProgrammedNCVersion)
			continue
		}
		if channelMode == cns.CRD {
			service.MarkIpsAsAvailableUntransacted(ncInfo.ID, nmaProgrammedNCVersion)
		}
		//nolint:staticcheck // SA1019: suppress deprecated logger.Printf usage. Todo: legacy logger usage is consistent in cns repo. Migrates when all logger usage is migrated
		logger.Printf("Updating NC %s host version from %s to %s", ncID, ncInfo.HostVersion, nmaProgrammedNCVersionStr)
		ncInfo.HostVersion = nmaProgrammedNCVersionStr
		logger.Printf("Updated NC %s host version to %s", ncID, ncInfo.HostVersion)
		service.state.ContainerStatus[ncID] = ncInfo
		// if we successfully updated the NC, pop it from the needs update set.
		delete(outdatedNCs, ncID)
	}
	// if we didn't empty out the needs update set, NMA has not programmed all the NCs we are expecting, and we
	// need to return an error indicating that
	if len(outdatedNCs) > 0 {
		return len(programmedNCs), errors.Errorf("unable to update some NCs: %v, missing or bad response from NMA or IMDS", outdatedNCs)
	}

	return len(programmedNCs), nil
}

func (service *HTTPRestService) ReconcileIPAssignment(podInfoByIP map[string]cns.PodInfo, ncReqs []*cns.CreateNetworkContainerRequest) types.ResponseCode {
	// index all the secondary IP configs for all the nc reqs, for easier lookup later on.
	allSecIPsIdx := make(map[string]*cns.CreateNetworkContainerRequest)
	for i := range ncReqs {
		for _, secIPConfig := range ncReqs[i].SecondaryIPConfigs {
			allSecIPsIdx[secIPConfig.IPAddress] = ncReqs[i]
		}
	}

	// we now need to reconcile IP assignment.
	// considering that a single pod may have multiple ips (such as in dual stack scenarios)
	// and that IP assignment in CNS (as done by requestIPConfigsHelper) does not allow
	// updates (it returns the existing state if one already exists for the pod's interface),
	// we need to assign all IPs for a pod interface or name+namespace at the same time.
	//
	// iterating over single IPs is not appropriate then, since assignment for the first IP for
	// a pod will prevent the second IP from being added. the following function call transforms
	// pod info indexed by ip address:
	//
	//   {
	//     "10.0.0.1": podInfo{interface: "aaa-eth0"},
	//     "fe80::1":  podInfo{interface: "aaa-eth0"},
	//   }
	//
	// to pod IPs indexed by pod key (interface or name+namespace, depending on scenario):
	//
	//   {
	//     "aaa-eth0": podIPs{v4IP: 10.0.0.1, v6IP: fe80::1}
	//   }
	//
	// such that we can iterate over pod interfaces, and assign all IPs for it at once.

	podKeyToPodIPs, err := newPodKeyToPodIPsMap(podInfoByIP)
	if err != nil {
		logger.Errorf("could not transform pods indexed by IP address to pod IPs indexed by interface: %v", err)
		return types.UnexpectedError
	}

	for podKey, podIPs := range podKeyToPodIPs {
		var (
			desiredIPs []string
			ncIDs      []string
		)

		var ips []net.IP
		if podIPs.v4IP != nil {
			ips = append(ips, podIPs.v4IP)
		}

		if podIPs.v6IP != nil {
			ips = append(ips, podIPs.v6IP)
		}

		for _, ip := range ips {
			if ncReq, ok := allSecIPsIdx[ip.String()]; ok {
				desiredIPs = append(desiredIPs, ip.String())
				ncIDs = append(ncIDs, ncReq.NetworkContainerid)
			} else {
				// it might still be possible to see host networking pods here (where ips are not from ncs) if we are restoring using the kube podinfo provider
				// todo: once kube podinfo provider reconcile flow is removed, this line will not be necessary/should be removed.
				logger.Errorf("ip %s assigned to pod %+v but not found in any nc", ip, podIPs)
			}
		}

		if len(desiredIPs) == 0 {
			// this may happen for pods in the host network
			continue
		}

		jsonContext, err := podIPs.OrchestratorContext()
		if err != nil {
			logger.Errorf("Failed to marshal KubernetesPodInfo, error: %v", err)
			return types.UnexpectedError
		}

		ipconfigsRequest := cns.IPConfigsRequest{
			DesiredIPAddresses:  desiredIPs,
			OrchestratorContext: jsonContext,
			InfraContainerID:    podIPs.InfraContainerID(),
			PodInterfaceID:      podIPs.InterfaceID(),
		}

		if _, err := requestIPConfigsHelper(service, ipconfigsRequest); err != nil {
			//nolint:staticcheck // SA1019: suppress deprecated logger.Printf usage. Todo: legacy logger usage is consistent in cns repo. Migrates when all logger usage is migrated
			logger.Errorf("requestIPConfigsHelper failed for pod key %s, podInfo %+v, ncIDs %v, error: %v", podKey, podIPs, ncIDs, err)
			return types.FailedToAllocateIPConfig
		}
	}

	return types.Success
}

func (service *HTTPRestService) CreateNCs(ncReqs []*cns.CreateNetworkContainerRequest) types.ResponseCode {
	for _, ncReq := range ncReqs {
		returnCode := service.CreateOrUpdateNetworkContainerInternal(ncReq)
		if returnCode != types.Success {
			return returnCode
		}
	}

	return types.Success
}

func (service *HTTPRestService) ReconcileIPAMStateForSwift(ncReqs []*cns.CreateNetworkContainerRequest, podInfoByIP map[string]cns.PodInfo, nnc *v1alpha.NodeNetworkConfig) types.ResponseCode {
	logger.Printf("Reconciling CNS IPAM state with nc requests: [%+v], PodInfo [%+v], NNC: [%+v]", ncReqs, podInfoByIP, nnc)
	// if no nc reqs, there is no CRD state yet
	if len(ncReqs) == 0 {
		logger.Printf("CNS starting with no NC state, podInfoMap count %d", len(podInfoByIP))
		return types.Success
	}

	// first step in reconciliation is to create all the NCs in CNS, no IP assignment yet.
	if returnCode := service.CreateNCs(ncReqs); returnCode != types.Success {
		return returnCode
	}

	logger.Debugf("ncReqs created successfully, now save IPs")
	// now reconcile IPAM state.
	if returnCode := service.ReconcileIPAssignment(podInfoByIP, ncReqs); returnCode != types.Success {
		return returnCode
	}

	if err := service.MarkExistingIPsAsPendingRelease(nnc.Spec.IPsNotInUse); err != nil {
		logger.Errorf("[Azure CNS] Error. Failed to mark IPs as pending %v", nnc.Spec.IPsNotInUse)
		return types.UnexpectedError
	}

	return types.Success
}

// todo: there is some redundancy between this funcation and ReconcileIPAMStateForNodeSubnet. The difference is that this one
// doesn't include the NNC parameter. We may want to unify the common parts.
func (service *HTTPRestService) ReconcileIPAMStateForNodeSubnet(ncReqs []*cns.CreateNetworkContainerRequest, podInfoByIP map[string]cns.PodInfo) types.ResponseCode {
	logger.Printf("Reconciling CNS IPAM state with nc requests: [%+v], PodInfo [%+v]", ncReqs, podInfoByIP)

	if len(ncReqs) != 1 {
		logger.Errorf("Nodesubnet should always have 1 NC to hold secondary IPs")
		return types.NetworkContainerNotSpecified
	}

	// first step in reconciliation is to create all the NCs in CNS, no IP assignment yet.
	if returnCode := service.CreateNCs(ncReqs); returnCode != types.Success {
		return returnCode
	}

	logger.Debugf("ncReqs created successfully, now save IPs")
	// now reconcile IPAM state.
	if returnCode := service.ReconcileIPAssignment(podInfoByIP, ncReqs); returnCode != types.Success {
		return returnCode
	}

	return types.Success
}

var (
	errIPParse             = errors.New("parse IP")
	errMultipleIPPerFamily = errors.New("multiple IPs per family")
)

// newPodKeyToPodIPsMap groups IPs by interface id and returns them indexed by interface id.
func newPodKeyToPodIPsMap(podInfoByIP map[string]cns.PodInfo) (map[string]podIPs, error) {
	podKeyToPodIPs := make(map[string]podIPs)

	for ipStr, podInfo := range podInfoByIP {
		id := podInfo.Key()

		ips, ok := podKeyToPodIPs[id]
		if !ok {
			ips.PodInfo = podInfo
		}

		ip := net.ParseIP(ipStr)
		switch {
		case ip == nil:
			return nil, errors.Wrapf(errIPParse, "could not parse ip string %q on pod %+v", ipStr, podInfo)
		case ip.To4() != nil:
			if ips.v4IP != nil {
				return nil, errors.Wrapf(errMultipleIPPerFamily, "multiple ipv4 addresses (%v, %v) associated to pod %+v", ips.v4IP, ip, podInfo)
			}

			ips.v4IP = ip
		case ip.To16() != nil:
			if ips.v6IP != nil {
				return nil, errors.Wrapf(errMultipleIPPerFamily, "multiple ipv6 addresses (%v, %v) associated to pod %+v", ips.v6IP, ip, podInfo)
			}

			ips.v6IP = ip
		}

		podKeyToPodIPs[id] = ips
	}

	return podKeyToPodIPs, nil
}

// podIPs are all the IPs associated with a pod, along with pod info
type podIPs struct {
	cns.PodInfo
	v4IP net.IP
	v6IP net.IP
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

func (service *HTTPRestService) MustEnsureNoStaleNCs(validNCIDs []string) {
	valid := make(map[string]struct{})
	for _, ncID := range validNCIDs {
		valid[ncID] = struct{}{}
	}

	service.Lock()
	defer service.Unlock()

	ncIDToAssignedIPs := make(map[string][]cns.IPConfigurationStatus)
	for _, ipInfo := range service.PodIPConfigState { // nolint:gocritic // copy is fine; it's a larger change to modify the map to hold pointers
		if ipInfo.GetState() == types.Assigned {
			ncIDToAssignedIPs[ipInfo.NCID] = append(ncIDToAssignedIPs[ipInfo.NCID], ipInfo)
		}
	}

	mutated := false
	for ncID := range service.state.ContainerStatus {
		if _, ok := valid[ncID]; !ok {
			// stale NCs with assigned IPs are an unexpected CNS state which we need to alert on.
			if assignedIPs, hasAssignedIPs := ncIDToAssignedIPs[ncID]; hasAssignedIPs {
				msg := fmt.Sprintf("Unexpected state: found stale NC ID %s in CNS state with %d assigned IPs: %+v", ncID, len(assignedIPs), assignedIPs)
				logger.Errorf(msg)
				panic(msg)
			}

			logger.Errorf("[Azure CNS] Found stale NC ID %s in CNS state. Removing...", ncID)
			delete(service.state.ContainerStatus, ncID)
			mutated = true
		}
	}

	if mutated {
		_ = service.saveState()
	}
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

	if req.NetworkContainerid == nodesubnet.NodeSubnetNCID {
		// For NodeSubnet scenarios, Validate PrimaryCA must be empty
		if req.IPConfiguration.IPSubnet.IPAddress != "" {
			logger.Errorf("[Azure CNS] Error. PrimaryCA is invalid, NC Req: %v", req)
			return types.InvalidPrimaryIPConfig
		}
	} else {
		// For Swift scenarios, Validate PrimaryCA must never be empty
		err := validateIPSubnet(req.IPConfiguration.IPSubnet)
		if err != nil {
			logger.Errorf("[Azure CNS] Error. PrimaryCA is invalid, NC Req: %v", req)
			return types.InvalidPrimaryIPConfig
		}
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
		if !reflect.DeepEqual(existingReq.IPConfiguration.IPSubnet, req.IPConfiguration.IPSubnet) {
			// check for potential overlay subnet expansion - checking if new subnet is a superset of old subnet
			isCIDRSuperset := validateCIDRSuperset(
				fmt.Sprintf("%s/%d", req.IPConfiguration.IPSubnet.IPAddress, req.IPConfiguration.IPSubnet.PrefixLength),
				fmt.Sprintf("%s/%d", existingReq.IPConfiguration.IPSubnet.IPAddress, existingReq.IPConfiguration.IPSubnet.PrefixLength))
			if !isCIDRSuperset {
				logger.Errorf("[Azure CNS] Error. PrimaryCA is not same, NCId %s, old CA %s/%d, new CA %s/%d", //nolint:staticcheck // Suppress SA1019: logger.Errorf is deprecated
					req.NetworkContainerid,
					existingReq.IPConfiguration.IPSubnet.IPAddress,
					existingReq.IPConfiguration.IPSubnet.PrefixLength,
					req.IPConfiguration.IPSubnet.IPAddress,
					req.IPConfiguration.IPSubnet.PrefixLength)
				return types.PrimaryCANotSame
			}
		}
	}

	// This will Create Or Update the NC state.
	returnCode, returnMessage := service.saveNetworkContainerGoalState(*req)

	// If the NC was created successfully, log NC snapshot.
	if returnCode == 0 {
		logNCSnapshot(*req)
		service.publishIPStateMetrics()
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

func (service *HTTPRestService) SetVFForAccelnetNICs() error {
	return service.setVFForAccelnetNICs()
}

func (service *HTTPRestService) isNCDetailsAPIExists(ctx context.Context) bool {
	versionsResp, err := service.imdsClient.GetIMDSVersions(ctx)
	if err != nil {
		//nolint:staticcheck // SA1019: suppress deprecated logger.Printf usage. Todo: legacy logger usage is consistent in cns repo. Migrates when all logger usage is migrated
		logger.Errorf("Failed to get IMDS versions: %v", err)
		return false
	}

	// Check if the expected API version exists in the response
	for _, version := range versionsResp.APIVersions {
		if version == expectedIMDSAPIVersion {
			//nolint:staticcheck // SA1019: suppress deprecated logger.Debugf usage. Todo: legacy logger usage is consistent in cns repo. Migrates when all logger usage is migrated
			logger.Debugf("IMDS has expected API version")
			return true
		}
	}
	return false
}

// GetIMDSNCs gets NC versions from IMDS and returns them as a map
func (service *HTTPRestService) GetIMDSNCs(ctx context.Context) (map[string]string, error) {
	imdsClient := service.imdsClient
	if imdsClient == nil {
		//nolint:staticcheck // SA1019: suppress deprecated logger.Printf usage. Todo: legacy logger usage is consistent in cns repo. Migrates when all logger usage is migrated
		logger.Errorf("IMDS client is not available")
		return make(map[string]string), nil
	}
	// Check NC version support
	if !service.isNCDetailsAPIExists(ctx) {
		//nolint:staticcheck // SA1019: suppress deprecated logger.Printf usage. Todo: legacy logger usage is consistent in cns repo. Migrates when all logger usage is migrated
		logger.Errorf("IMDS does not support NC details API")
		return make(map[string]string), nil
	}

	// Get all network interfaces from IMDS
	networkInterfaces, err := imdsClient.GetNetworkInterfaces(ctx)
	if err != nil {
		//nolint:staticcheck // SA1019: suppress deprecated logger.Printf usage. Todo: legacy logger usage is consistent in cns repo. Migrates when all logger usage is migrated
		logger.Errorf("Failed to get network interfaces from IMDS: %v", err)
		return make(map[string]string), nil
	}

	// Build ncs map from the network interfaces
	ncs := make(map[string]string)
	for _, iface := range networkInterfaces {
		//nolint:staticcheck // SA1019: suppress deprecated logger.Debugf usage. Todo: legacy logger usage is consistent in cns repo. Migrates when all logger usage is migrated
		logger.Debugf("Nc id: %s and mac address: %s from IMDS call", iface.InterfaceCompartmentID, iface.MacAddress.String())
		// IMDS returns interfaceCompartmentID, as nc id guid has different context on nma. We map these to NC ID
		ncID := iface.InterfaceCompartmentID

		if ncID != "" {
			ncs[ncID] = PrefixOnNicNCVersion // for prefix on nic version scenario nc version is 1
		}
	}

	return ncs, nil
}

// IsCIDRSuperset returns true if newCIDR is a superset of oldCIDR (i.e., all IPs in oldCIDR are contained in newCIDR).
func validateCIDRSuperset(newCIDR, oldCIDR string) bool {
	// Parse newCIDR and oldCIDR into netip.Prefix
	newPrefix, err := netip.ParsePrefix(newCIDR)
	if err != nil {
		return false
	}

	oldPrefix, err := netip.ParsePrefix(oldCIDR)
	if err != nil {
		return false
	}

	// Condition 1: Check if the new prefix length is smaller (larger range) than the old prefix length
	if newPrefix.Bits() >= oldPrefix.Bits() {
		return false
	}

	// Condition 2: Check for Overlap - this will also ensure containment
	if !newPrefix.Overlaps(oldPrefix) {
		return false
	}

	return true
}
