// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/dockerclient"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/networkcontainers"
	"github.com/Azure/azure-container-networking/cns/nmagentclient"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/store"
)

// This file contains the utility/helper functions called by either HTTP APIs or Exported/Internal APIs on HTTPRestService

// Get the network info from the service network state
func (service *HTTPRestService) getNetworkInfo(networkName string) (*networkInfo, bool) {
	service.RLock()
	defer service.RUnlock()
	networkInfo, found := service.state.Networks[networkName]

	return networkInfo, found
}

// Set the network info in the service network state
func (service *HTTPRestService) setNetworkInfo(networkName string, networkInfo *networkInfo) {
	service.Lock()
	defer service.Unlock()
	service.state.Networks[networkName] = networkInfo

	return
}

// Remove the network info from the service network state
func (service *HTTPRestService) removeNetworkInfo(networkName string) {
	service.Lock()
	defer service.Unlock()
	delete(service.state.Networks, networkName)

	return
}

// saveState writes CNS state to persistent store.
func (service *HTTPRestService) saveState() error {
	logger.Printf("[Azure CNS] saveState")

	// Skip if a store is not provided.
	if service.store == nil {
		logger.Printf("[Azure CNS]  store not initialized.")
		return nil
	}

	// Update time stamp.
	service.state.TimeStamp = time.Now()
	err := service.store.Write(storeKey, &service.state)
	if err == nil {
		logger.Printf("[Azure CNS]  State saved successfully.\n")
	} else {
		logger.Errorf("[Azure CNS]  Failed to save state., err:%v\n", err)
	}

	return err
}

// restoreState restores CNS state from persistent store.
func (service *HTTPRestService) restoreState() {
	logger.Printf("[Azure CNS] restoreState")

	// Skip if a store is not provided.
	if service.store == nil {
		logger.Printf("[Azure CNS]  store not initialized.")
		return
	}

	// Read any persisted state.
	err := service.store.Read(storeKey, &service.state)
	if err != nil {
		if err == store.ErrKeyNotFound {
			// Nothing to restore.
			logger.Printf("[Azure CNS]  No state to restore.\n")
		} else {
			logger.Errorf("[Azure CNS]  Failed to restore state, err:%v. Removing azure-cns.json", err)
			service.store.Remove()
		}

		return
	}

	logger.Printf("[Azure CNS]  Restored state, %+v\n", service.state)
	return
}

func (service *HTTPRestService) saveNetworkContainerGoalState(req cns.CreateNetworkContainerRequest) (int, string) {
	// we don't want to overwrite what other calls may have written
	service.Lock()
	defer service.Unlock()

	var (
		hostVersion                string
		existingSecondaryIPConfigs map[string]cns.SecondaryIPConfig //uuid is key
		vfpUpdateComplete          bool
	)

	if service.state.ContainerStatus == nil {
		service.state.ContainerStatus = make(map[string]containerstatus)
	}

	existingNCStatus, ok := service.state.ContainerStatus[req.NetworkContainerid]
	if ok {
		hostVersion = existingNCStatus.HostVersion
		existingSecondaryIPConfigs = existingNCStatus.CreateNetworkContainerRequest.SecondaryIPConfigs
		vfpUpdateComplete = existingNCStatus.VfpUpdateComplete
	}
	if hostVersion == "" {
		// Host version is the NC version from NMAgent, set it -1 to indicate no result from NMAgent yet.
		// TODO, query NMAgent and with aggresive time out and assign latest host version.
		hostVersion = "-1"
	}

	// Remove the auth token before saving the containerStatus to cns json file
	createNetworkContainerRequest := req
	createNetworkContainerRequest.AuthorizationToken = ""

	service.state.ContainerStatus[req.NetworkContainerid] =
		containerstatus{
			ID:                            req.NetworkContainerid,
			VMVersion:                     req.Version,
			CreateNetworkContainerRequest: createNetworkContainerRequest,
			HostVersion:                   hostVersion,
			VfpUpdateComplete:             vfpUpdateComplete}

	switch req.NetworkContainerType {
	case cns.AzureContainerInstance:
		fallthrough
	case cns.Docker:
		fallthrough
	case cns.Basic:
		fallthrough
	case cns.JobObject:
		fallthrough
	case cns.COW:
		fallthrough
	case cns.WebApps:
		switch service.state.OrchestratorType {
		case cns.Kubernetes:
			fallthrough
		case cns.ServiceFabric:
			fallthrough
		case cns.Batch:
			fallthrough
		case cns.DBforPostgreSQL:
			fallthrough
		case cns.AzureFirstParty:
			fallthrough
		case cns.WebApps: // todo: Is WebApps an OrchastratorType or ContainerType?
			podInfo, err := cns.UnmarshalPodInfo(req.OrchestratorContext)
			if err != nil {
				errBuf := fmt.Sprintf("Unmarshalling %s failed with error %v", req.NetworkContainerType, err)
				return UnexpectedError, errBuf
			}

			logger.Printf("Pod info %v", podInfo)

			if service.state.ContainerIDByOrchestratorContext == nil {
				service.state.ContainerIDByOrchestratorContext = make(map[string]string)
			}

			service.state.ContainerIDByOrchestratorContext[podInfo.Name()+podInfo.Namespace()] = req.NetworkContainerid

		case cns.KubernetesCRD:
			// Validate and Update the SecondaryIpConfig state
			returnCode, returnMesage := service.updateIpConfigsStateUntransacted(req, existingSecondaryIPConfigs, hostVersion)
			if returnCode != 0 {
				return returnCode, returnMesage
			}
		default:
			errMsg := fmt.Sprintf("Unsupported orchestrator type: %s", service.state.OrchestratorType)
			logger.Errorf(errMsg)
			return UnsupportedOrchestratorType, errMsg
		}

	default:
		errMsg := fmt.Sprintf("Unsupported network container type %s", req.NetworkContainerType)
		logger.Errorf(errMsg)
		return UnsupportedNetworkContainerType, errMsg
	}

	service.saveState()
	return 0, ""
}

// This func will compute the deltaIpConfigState which needs to be updated (Added or Deleted) from the inmemory map
// Note: Also this func is an untransacted API as the caller will take a Service lock
func (service *HTTPRestService) updateIpConfigsStateUntransacted(req cns.CreateNetworkContainerRequest, existingSecondaryIPConfigs map[string]cns.SecondaryIPConfig, hostVersion string) (int, string) {
	// parse the existingSecondaryIpConfigState to find the deleted Ips
	newIPConfigs := req.SecondaryIPConfigs
	var tobeDeletedIpConfigs = make(map[string]cns.SecondaryIPConfig)

	// Populate the ToBeDeleted list, Secondary IPs which doesnt exist in New request anymore.
	// We will later remove them from the in-memory cache
	for secondaryIpId, existingIPConfig := range existingSecondaryIPConfigs {
		_, exists := newIPConfigs[secondaryIpId]
		if !exists {
			// IP got removed in the updated request, add it in tobeDeletedIps
			tobeDeletedIpConfigs[secondaryIpId] = existingIPConfig
		}
	}

	// Validate TobeDeletedIps are ready to be deleted.
	for ipId, _ := range tobeDeletedIpConfigs {
		ipConfigStatus, exists := service.PodIPConfigState[ipId]
		if exists {
			// pod ip exists, validate if state is not allocated, else fail
			if ipConfigStatus.State == cns.Allocated {
				errMsg := fmt.Sprintf("Failed to delete an Allocated IP %v", ipConfigStatus)
				return InconsistentIPConfigState, errMsg
			}
		}
	}

	// now actually remove the deletedIPs
	for ipId, _ := range tobeDeletedIpConfigs {
		returncode, errMsg := service.removeToBeDeletedIpsStateUntransacted(ipId, true)
		if returncode != Success {
			return returncode, errMsg
		}
	}

	// Add new IPs
	// TODO, will udpate NC version related variable to int, change it from string to int is a pains
	var hostNCVersionInInt int
	var err error
	if hostNCVersionInInt, err = strconv.Atoi(hostVersion); err != nil {
		return UnsupportedNCVersion, fmt.Sprintf("Invalid hostVersion is %s, err:%s", hostVersion, err)
	}
	service.addIPConfigStateUntransacted(req.NetworkContainerid, hostNCVersionInInt, req.SecondaryIPConfigs, existingSecondaryIPConfigs)

	return 0, ""
}

// addIPConfigStateUntransacted adds the IPConfigs to the PodIpConfigState map with Available state
// If the IP is already added then it will be an idempotent call. Also note, caller will
// acquire/release the service lock.
func (service *HTTPRestService) addIPConfigStateUntransacted(ncId string, hostVersion int, ipconfigs, existingSecondaryIPConfigs map[string]cns.SecondaryIPConfig) {
	// add ipconfigs to state
	for ipId, ipconfig := range ipconfigs {
		// New secondary IP configs has new NC version however, CNS don't want to override existing IPs'with new NC version
		// Set it back to previous NC version if IP already exist.
		if existingIPConfig, existsInPreviousIPConfig := existingSecondaryIPConfigs[ipId]; existsInPreviousIPConfig {
			ipconfig.NCVersion = existingIPConfig.NCVersion
			ipconfigs[ipId] = ipconfig
		}
		logger.Printf("[Azure-Cns] Set IP %s version to %d, programmed host nc version is %d", ipconfig.IPAddress, ipconfig.NCVersion, hostVersion)
		if _, exists := service.PodIPConfigState[ipId]; exists {
			continue
		}
		// Using the updated NC version attached with IP to compare with latest nmagent version and determine IP statues.
		// When reconcile, service.PodIPConfigState doens't exist, rebuild it with the help of NC version attached with IP.
		var newIPCNSStatus string
		if hostVersion < ipconfig.NCVersion {
			newIPCNSStatus = cns.PendingProgramming
		} else {
			newIPCNSStatus = cns.Available
		}
		// add the new State
		ipconfigStatus := cns.IPConfigurationStatus{
			NCID:      ncId,
			ID:        ipId,
			IPAddress: ipconfig.IPAddress,
			State:     newIPCNSStatus,
			PodInfo:   nil,
		}
		logger.Printf("[Azure-Cns] Add IP %s as %s", ipconfig.IPAddress, newIPCNSStatus)

		service.PodIPConfigState[ipId] = ipconfigStatus

		// Todo Update batch API and maintain the count
	}
}

// Todo: call this when request is received
func validateIPSubnet(ipSubnet cns.IPSubnet) error {
	if ipSubnet.IPAddress == "" {
		return fmt.Errorf("Failed to add IPConfig to state: %+v, empty IPSubnet.IPAddress", ipSubnet)
	}
	if ipSubnet.PrefixLength == 0 {
		return fmt.Errorf("Failed to add IPConfig to state: %+v, empty IPSubnet.PrefixLength", ipSubnet)
	}
	return nil
}

// removeToBeDeletedIpsStateUntransacted removes IPConfigs from the PodIpConfigState map
// Caller will acquire/release the service lock.
func (service *HTTPRestService) removeToBeDeletedIpsStateUntransacted(ipId string, skipValidation bool) (int, string) {

	// this is set if caller has already done the validation
	if !skipValidation {
		ipConfigStatus, exists := service.PodIPConfigState[ipId]
		if exists {
			// pod ip exists, validate if state is not allocated, else fail
			if ipConfigStatus.State == cns.Allocated {
				errMsg := fmt.Sprintf("Failed to delete an Allocated IP %v", ipConfigStatus)
				return InconsistentIPConfigState, errMsg
			}
		}
	}

	// Delete this ip from PODIpConfigState Map
	logger.Printf("[Azure-Cns] Delete the PodIpConfigState, IpId: %s, IPConfigStatus: %v", ipId, service.PodIPConfigState[ipId])
	delete(service.PodIPConfigState, ipId)
	return 0, ""
}

func (service *HTTPRestService) getNetworkContainerResponse(req cns.GetNetworkContainerRequest) cns.GetNetworkContainerResponse {
	var (
		containerID                 string
		getNetworkContainerResponse cns.GetNetworkContainerResponse
		exists                      bool
		waitingForUpdate            bool
	)

	service.Lock()
	defer service.Unlock()

	switch service.state.OrchestratorType {
	case cns.Kubernetes:
		fallthrough
	case cns.ServiceFabric:
		fallthrough
	case cns.Batch:
		fallthrough
	case cns.DBforPostgreSQL:
		fallthrough
	case cns.AzureFirstParty:
		podInfo, err := cns.UnmarshalPodInfo(req.OrchestratorContext)
		if err != nil {
			getNetworkContainerResponse.Response.ReturnCode = UnexpectedError
			getNetworkContainerResponse.Response.Message = fmt.Sprintf("Unmarshalling orchestrator context failed with error %v", err)
			return getNetworkContainerResponse
		}

		logger.Printf("pod info %+v", podInfo)

		containerID, exists = service.state.ContainerIDByOrchestratorContext[podInfo.Name()+podInfo.Namespace()]

		if exists {
			// If the goal state is available with CNS, check if the NC is pending VFP programming
			waitingForUpdate, getNetworkContainerResponse.Response.ReturnCode, getNetworkContainerResponse.Response.Message =
				service.isNCWaitingForUpdate(service.state.ContainerStatus[containerID].CreateNetworkContainerRequest.Version, containerID)
			// If the return code is not success, return the error to the caller
			if getNetworkContainerResponse.Response.ReturnCode == NetworkContainerVfpProgramPending {
				logger.Errorf("[Azure-CNS] isNCWaitingForUpdate failed for NC: %s with error: %s",
					containerID, getNetworkContainerResponse.Response.Message)
				return getNetworkContainerResponse
			}

			vfpUpdateComplete := !waitingForUpdate
			ncstatus, _ := service.state.ContainerStatus[containerID]
			// Update the container status if-
			// 1. VfpUpdateCompleted successfully
			// 2. VfpUpdateComplete changed to false
			if (getNetworkContainerResponse.Response.ReturnCode == NetworkContainerVfpProgramComplete &&
				vfpUpdateComplete == true && ncstatus.VfpUpdateComplete != vfpUpdateComplete) ||
				(vfpUpdateComplete == false && ncstatus.VfpUpdateComplete != vfpUpdateComplete) {
				logger.Printf("[Azure-CNS] Setting VfpUpdateComplete to %t for NC: %s", vfpUpdateComplete, containerID)
				ncstatus.VfpUpdateComplete = vfpUpdateComplete
				service.state.ContainerStatus[containerID] = ncstatus
				service.saveState()
			}

		} else if service.ChannelMode == cns.Managed {
			// If the NC goal state doesn't exist in CNS running in managed mode, call DNC to retrieve the goal state
			var (
				dncEP     = service.GetOption(acn.OptPrivateEndpoint).(string)
				infraVnet = service.GetOption(acn.OptInfrastructureNetworkID).(string)
				nodeID    = service.GetOption(acn.OptNodeID).(string)
			)

			service.Unlock()
			getNetworkContainerResponse.Response.ReturnCode, getNetworkContainerResponse.Response.Message = service.SyncNodeStatus(dncEP, infraVnet, nodeID, req.OrchestratorContext)
			service.Lock()
			if getNetworkContainerResponse.Response.ReturnCode == NotFound {
				return getNetworkContainerResponse
			}

			containerID = service.state.ContainerIDByOrchestratorContext[podInfo.Name()+podInfo.Namespace()]
		}

		logger.Printf("containerid %v", containerID)
		break

	default:
		getNetworkContainerResponse.Response.ReturnCode = UnsupportedOrchestratorType
		getNetworkContainerResponse.Response.Message = fmt.Sprintf("Invalid orchestrator type %v", service.state.OrchestratorType)
		return getNetworkContainerResponse
	}

	containerStatus := service.state.ContainerStatus
	containerDetails, ok := containerStatus[containerID]
	if !ok {
		getNetworkContainerResponse.Response.ReturnCode = UnknownContainerID
		getNetworkContainerResponse.Response.Message = "NetworkContainer doesn't exist."
		return getNetworkContainerResponse
	}

	savedReq := containerDetails.CreateNetworkContainerRequest
	getNetworkContainerResponse = cns.GetNetworkContainerResponse{
		NetworkContainerID:         savedReq.NetworkContainerid,
		IPConfiguration:            savedReq.IPConfiguration,
		Routes:                     savedReq.Routes,
		CnetAddressSpace:           savedReq.CnetAddressSpace,
		MultiTenancyInfo:           savedReq.MultiTenancyInfo,
		PrimaryInterfaceIdentifier: savedReq.PrimaryInterfaceIdentifier,
		LocalIPConfiguration:       savedReq.LocalIPConfiguration,
		AllowHostToNCCommunication: savedReq.AllowHostToNCCommunication,
		AllowNCToHostCommunication: savedReq.AllowNCToHostCommunication,
	}

	return getNetworkContainerResponse
}

// restoreNetworkState restores Network state that existed before reboot.
func (service *HTTPRestService) restoreNetworkState() error {
	logger.Printf("[Azure CNS] Enter Restoring Network State")

	if service.store == nil {
		logger.Printf("[Azure CNS] Store is not initialized, nothing to restore for network state.")
		return nil
	}

	rebooted := false
	modTime, err := service.store.GetModificationTime()

	if err == nil {
		logger.Printf("[Azure CNS] Store timestamp is %v.", modTime)

		rebootTime, err := platform.GetLastRebootTime()
		if err == nil && rebootTime.After(modTime) {
			logger.Printf("[Azure CNS] reboot time %v mod time %v", rebootTime, modTime)
			rebooted = true
		}
	}

	if rebooted {
		for _, nwInfo := range service.state.Networks {
			enableSnat := true

			logger.Printf("[Azure CNS] Restore nwinfo %v", nwInfo)

			if nwInfo.Options != nil {
				if _, ok := nwInfo.Options[dockerclient.OptDisableSnat]; ok {
					enableSnat = false
				}
			}

			if enableSnat {
				err := platform.SetOutboundSNAT(nwInfo.NicInfo.Subnet)
				if err != nil {
					logger.Printf("[Azure CNS] Error setting up SNAT outbound rule %v", err)
					return err
				}
			}
		}
	}

	return nil
}

func (service *HTTPRestService) attachOrDetachHelper(req cns.ConfigureContainerNetworkingRequest, operation, method string) cns.Response {
	if method != "POST" {
		return cns.Response{
			ReturnCode: InvalidParameter,
			Message:    "[Azure CNS] Error. " + operation + "ContainerToNetwork did not receive a POST."}
	}
	if req.Containerid == "" {
		return cns.Response{
			ReturnCode: DockerContainerNotSpecified,
			Message:    "[Azure CNS] Error. Containerid is empty"}
	}
	if req.NetworkContainerid == "" {
		return cns.Response{
			ReturnCode: NetworkContainerNotSpecified,
			Message:    "[Azure CNS] Error. NetworkContainerid is empty"}
	}

	existing, ok := service.getNetworkContainerDetails(cns.SwiftPrefix + req.NetworkContainerid)
	if service.ChannelMode == cns.Managed && operation == attach {
		if ok {
			if !existing.VfpUpdateComplete {
				_, returnCode, message := service.isNCWaitingForUpdate(existing.CreateNetworkContainerRequest.Version, req.NetworkContainerid)
				if returnCode == NetworkContainerVfpProgramPending {
					return cns.Response{
						ReturnCode: returnCode,
						Message:    message}
				}
			}
		} else {
			var (
				dncEP     = service.GetOption(acn.OptPrivateEndpoint).(string)
				infraVnet = service.GetOption(acn.OptInfrastructureNetworkID).(string)
				nodeID    = service.GetOption(acn.OptNodeID).(string)
			)

			returnCode, msg := service.SyncNodeStatus(dncEP, infraVnet, nodeID, json.RawMessage{})
			if returnCode != Success {
				return cns.Response{
					ReturnCode: returnCode,
					Message:    msg}
			}

			existing, _ = service.getNetworkContainerDetails(cns.SwiftPrefix + req.NetworkContainerid)
		}
	} else if !ok {
		return cns.Response{
			ReturnCode: NotFound,
			Message:    fmt.Sprintf("[Azure CNS] Error. Network Container %s does not exist.", req.NetworkContainerid)}
	}

	returnCode := 0
	returnMessage := ""
	switch service.state.OrchestratorType {
	case cns.Batch:
		podInfo, err := cns.UnmarshalPodInfo(existing.CreateNetworkContainerRequest.OrchestratorContext)
		if err != nil {
			returnCode = UnexpectedError
			returnMessage = fmt.Sprintf("Unmarshalling orchestrator context failed with error %+v", err)
		} else {
			nc := service.networkContainer
			netPluginConfig := service.getNetPluginDetails()
			switch operation {
			case attach:
				err = nc.Attach(podInfo, req.Containerid, netPluginConfig)
			case detach:
				err = nc.Detach(podInfo, req.Containerid, netPluginConfig)
			}
			if err != nil {
				returnCode = UnexpectedError
				returnMessage = fmt.Sprintf("[Azure CNS] Error. "+operation+"ContainerToNetwork failed %+v", err.Error())
			}
		}

	default:
		returnMessage = fmt.Sprintf("[Azure CNS] Invalid orchestrator type %v", service.state.OrchestratorType)
		returnCode = UnsupportedOrchestratorType
	}

	return cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage}
}

func (service *HTTPRestService) getNetPluginDetails() *networkcontainers.NetPluginConfiguration {
	pluginBinPath, _ := service.GetOption(acn.OptNetPluginPath).(string)
	configPath, _ := service.GetOption(acn.OptNetPluginConfigFile).(string)
	return networkcontainers.NewNetPluginConfiguration(pluginBinPath, configPath)
}

func (service *HTTPRestService) getNetworkContainerDetails(networkContainerID string) (containerstatus, bool) {
	service.RLock()
	defer service.RUnlock()

	containerDetails, containerExists := service.state.ContainerStatus[networkContainerID]

	return containerDetails, containerExists
}

// Check if the network is joined
func (service *HTTPRestService) isNetworkJoined(networkID string) bool {
	namedLock.LockAcquire(stateJoinedNetworks)
	defer namedLock.LockRelease(stateJoinedNetworks)

	_, exists := service.state.joinedNetworks[networkID]

	return exists
}

// Set the network as joined
func (service *HTTPRestService) setNetworkStateJoined(networkID string) {
	namedLock.LockAcquire(stateJoinedNetworks)
	defer namedLock.LockRelease(stateJoinedNetworks)

	service.state.joinedNetworks[networkID] = struct{}{}
}

// Join Network by calling nmagent
func (service *HTTPRestService) joinNetwork(
	networkID string,
	joinNetworkURL string) (*http.Response, error, error) {
	var err error
	joinResponse, joinErr := nmagentclient.JoinNetwork(
		networkID,
		joinNetworkURL)

	if joinErr == nil && joinResponse.StatusCode == http.StatusOK {
		// Network joined successfully
		service.setNetworkStateJoined(networkID)
		logger.Printf("[Azure-CNS] setNetworkStateJoined for network: %s", networkID)
	} else {
		err = fmt.Errorf("Failed to join network: %s", networkID)
	}

	return joinResponse, joinErr, err
}

func logNCSnapshot(createNetworkContainerRequest cns.CreateNetworkContainerRequest) {
	var aiEvent = aitelemetry.Event{
		EventName:  logger.CnsNCSnapshotEventStr,
		Properties: make(map[string]string),
		ResourceID: createNetworkContainerRequest.NetworkContainerid,
	}

	aiEvent.Properties[logger.IpConfigurationStr] = fmt.Sprintf("%+v", createNetworkContainerRequest.IPConfiguration)
	aiEvent.Properties[logger.LocalIPConfigurationStr] = fmt.Sprintf("%+v", createNetworkContainerRequest.LocalIPConfiguration)
	aiEvent.Properties[logger.PrimaryInterfaceIdentifierStr] = createNetworkContainerRequest.PrimaryInterfaceIdentifier
	aiEvent.Properties[logger.MultiTenancyInfoStr] = fmt.Sprintf("%+v", createNetworkContainerRequest.MultiTenancyInfo)
	aiEvent.Properties[logger.CnetAddressSpaceStr] = fmt.Sprintf("%+v", createNetworkContainerRequest.CnetAddressSpace)
	aiEvent.Properties[logger.AllowNCToHostCommunicationStr] = fmt.Sprintf("%t", createNetworkContainerRequest.AllowNCToHostCommunication)
	aiEvent.Properties[logger.AllowHostToNCCommunicationStr] = fmt.Sprintf("%t", createNetworkContainerRequest.AllowHostToNCCommunication)
	aiEvent.Properties[logger.NetworkContainerTypeStr] = createNetworkContainerRequest.NetworkContainerType
	aiEvent.Properties[logger.OrchestratorContextStr] = fmt.Sprintf("%s", createNetworkContainerRequest.OrchestratorContext)

	// TODO - Add for SecondaryIPs (Task: https://msazure.visualstudio.com/One/_workitems/edit/7711831)

	logger.LogEvent(aiEvent)
}

// Sends network container snapshots to App Insights telemetry.
func (service *HTTPRestService) logNCSnapshots() {

	for _, ncStatus := range service.state.ContainerStatus {
		logNCSnapshot(ncStatus.CreateNetworkContainerRequest)
	}

	logger.Printf("[Azure CNS] Logging periodic NC snapshots. NC Count %d", len(service.state.ContainerStatus))
}

// Sets up periodic timer for sending network container snapshots
func (service *HTTPRestService) SendNCSnapShotPeriodically(ctx context.Context, ncSnapshotIntervalInMinutes int) {
	// Emit snapshot on startup and then emit it periodically.
	service.logNCSnapshots()

	ticker := time.NewTicker(time.Minute * time.Duration(ncSnapshotIntervalInMinutes))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			service.logNCSnapshots()
		}
	}
}

func (service *HTTPRestService) validateIpConfigRequest(ipConfigRequest cns.IPConfigRequest) (cns.PodInfo, int, string) {
	if service.state.OrchestratorType != cns.KubernetesCRD {
		return nil, UnsupportedOrchestratorType, "ReleaseIPConfig API supported only for kubernetes orchestrator"
	}

	if ipConfigRequest.OrchestratorContext == nil {
		return nil, EmptyOrchestratorContext, fmt.Sprintf("OrchastratorContext is not set in the req: %+v", ipConfigRequest)
	}

	// retrieve podinfo from orchestrator context
	podInfo, err := cns.NewPodInfoFromIPConfigRequest(ipConfigRequest)
	if err != nil {
		return podInfo, UnsupportedOrchestratorContext, err.Error()
	}
	return podInfo, Success, ""
}

func (service *HTTPRestService) populateIpConfigInfoUntransacted(ipConfigStatus cns.IPConfigurationStatus, podIpInfo *cns.PodIpInfo) error {
	var (
		ncStatus               containerstatus
		exists                 bool
		primaryIpConfiguration cns.IPConfiguration
	)

	if ncStatus, exists = service.state.ContainerStatus[ipConfigStatus.NCID]; !exists {
		return fmt.Errorf("Failed to get NC Configuration for NcId: %s", ipConfigStatus.NCID)
	}

	primaryIpConfiguration = ncStatus.CreateNetworkContainerRequest.IPConfiguration

	podIpInfo.PodIPConfig = cns.IPSubnet{
		IPAddress:    ipConfigStatus.IPAddress,
		PrefixLength: primaryIpConfiguration.IPSubnet.PrefixLength,
	}

	podIpInfo.NetworkContainerPrimaryIPConfig = primaryIpConfiguration

	hostInterfaceInfo, err := service.imdsClient.GetPrimaryInterfaceInfoFromMemory()
	if err != nil {
		return fmt.Errorf("Failed to get the HostInterfaceInfo %s", err)
	}

	podIpInfo.HostPrimaryIPInfo.PrimaryIP = hostInterfaceInfo.PrimaryIP
	podIpInfo.HostPrimaryIPInfo.Subnet = hostInterfaceInfo.Subnet
	podIpInfo.HostPrimaryIPInfo.Gateway = hostInterfaceInfo.Gateway

	return nil
}

// isNCWaitingForUpdate :- Determine whether NC version on NMA matches programmed version
// Return error and waitingForUpdate as true only CNS gets response from NMAgent indicating the VFP programming is pending
// This returns success / waitingForUpdate as false in all other cases.
func (service *HTTPRestService) isNCWaitingForUpdate(ncVersion, ncid string) (waitingForUpdate bool, returnCode int, message string) {
	waitingForUpdate = true
	ncStatus, ok := service.state.ContainerStatus[ncid]
	if ok {
		if ncStatus.VfpUpdateComplete &&
			(ncStatus.CreateNetworkContainerRequest.Version == ncVersion) {
			logger.Printf("[Azure CNS] Network container: %s, version: %s has VFP programming already completed", ncid, ncVersion)
			returnCode = NetworkContainerVfpProgramCheckSkipped
			waitingForUpdate = false
			return
		}
	}

	getNCVersionURL, ok := ncVersionURLs.Load(ncid)
	if !ok {
		logger.Printf("[Azure CNS] getNCVersionURL for Network container %s not found. Skipping GetNCVersionStatus check from NMAgent",
			ncid)
		returnCode = NetworkContainerVfpProgramCheckSkipped
		return
	}

	response, err := nmagentclient.GetNetworkContainerVersion(ncid, getNCVersionURL.(string))
	if err != nil {
		logger.Printf("[Azure CNS] Failed to get NC version status from NMAgent with error: %+v. "+
			"Skipping GetNCVersionStatus check from NMAgent", err)
		returnCode = NetworkContainerVfpProgramCheckSkipped
		return
	}

	if response.StatusCode != http.StatusOK {
		logger.Printf("[Azure CNS] Failed to get NC version status from NMAgent with http status %d. "+
			"Skipping GetNCVersionStatus check from NMAgent", response.StatusCode)
		returnCode = NetworkContainerVfpProgramCheckSkipped
		return
	}

	var versionResponse nmagentclient.NMANetworkContainerResponse
	rBytes, _ := ioutil.ReadAll(response.Body)
	json.Unmarshal(rBytes, &versionResponse)
	if versionResponse.ResponseCode != "200" {
		returnCode = NetworkContainerVfpProgramPending
		message = fmt.Sprintf("Failed to get NC version status from NMAgent. NC: %s, Response %s", ncid, rBytes)
		return
	}

	ncTargetVersion, _ := strconv.Atoi(ncVersion)
	nmaProgrammedNCVersion, _ := strconv.Atoi(versionResponse.Version)
	if ncTargetVersion > nmaProgrammedNCVersion {
		returnCode = NetworkContainerVfpProgramPending
		message = fmt.Sprintf("Network container: %s version: %d is not yet programmed by NMAgent. Programmed version: %d",
			ncid, ncTargetVersion, nmaProgrammedNCVersion)
	} else {
		returnCode = NetworkContainerVfpProgramComplete
		waitingForUpdate = false
		message = fmt.Sprintf("Vfp programming complete")
		logger.Printf("[Azure CNS] Vfp programming complete for NC: %s with version: %d", ncid, ncTargetVersion)
	}

	return
}

// ReturnCodeToString - Converts an error code to appropriate string.
func ReturnCodeToString(returnCode int) (s string) {
	switch returnCode {
	case Success:
		s = "Success"
	case UnsupportedNetworkType:
		s = "UnsupportedNetworkType"
	case InvalidParameter:
		s = "InvalidParameter"
	case UnreachableHost:
		s = "UnreachableHost"
	case ReservationNotFound:
		s = "ReservationNotFound"
	case MalformedSubnet:
		s = "MalformedSubnet"
	case UnreachableDockerDaemon:
		s = "UnreachableDockerDaemon"
	case UnspecifiedNetworkName:
		s = "UnspecifiedNetworkName"
	case NotFound:
		s = "NotFound"
	case AddressUnavailable:
		s = "AddressUnavailable"
	case NetworkContainerNotSpecified:
		s = "NetworkContainerNotSpecified"
	case CallToHostFailed:
		s = "CallToHostFailed"
	case UnknownContainerID:
		s = "UnknownContainerID"
	case UnsupportedOrchestratorType:
		s = "UnsupportedOrchestratorType"
	case UnexpectedError:
		s = "UnexpectedError"
	case DockerContainerNotSpecified:
		s = "DockerContainerNotSpecified"
	case NetworkContainerVfpProgramPending:
		s = "NetworkContainerVfpProgramPending"
	default:
		s = "UnknownError"
	}

	return
}
