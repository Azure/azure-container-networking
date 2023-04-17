package restserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/dockerclient"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/networkcontainers"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/cns/wireserver"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/store"
	"github.com/pkg/errors"
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
}

// Remove the network info from the service network state
func (service *HTTPRestService) removeNetworkInfo(networkName string) {
	service.Lock()
	defer service.Unlock()
	delete(service.state.Networks, networkName)
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

	if service.Options[acn.OptManageEndpointState] == true {
		err := service.EndpointStateStore.Read(EndpointStoreKey, &service.EndpointState)
		if err != nil {
			if errors.Is(err, store.ErrKeyNotFound) {
				// Nothing to restore.
				logger.Printf("[Azure CNS]  No endpoint state to restore.\n")
			} else {
				logger.Errorf("[Azure CNS]  Failed to restore endpoint state, err:%v. Removing endpoints.json", err)
			}
			return
		}
		logger.Printf("[Azure CNS]  Restored endpoint state, %+v\n", service.EndpointState)

	}
}

func (service *HTTPRestService) saveNetworkContainerGoalState(
	req cns.CreateNetworkContainerRequest,
) (types.ResponseCode, string) {
	// we don't want to overwrite what other calls may have written
	service.Lock()
	defer service.Unlock()

	var (
		hostVersion                string
		existingSecondaryIPConfigs map[string]cns.SecondaryIPConfig // uuid is key
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

	service.state.ContainerStatus[req.NetworkContainerid] = containerstatus{
		ID:                            req.NetworkContainerid,
		VMVersion:                     req.Version,
		CreateNetworkContainerRequest: createNetworkContainerRequest,
		HostVersion:                   hostVersion,
		VfpUpdateComplete:             vfpUpdateComplete,
	}

	switch req.NetworkContainerType {
	case cns.AzureContainerInstance:
		fallthrough
	case cns.Docker:
		fallthrough
	case cns.Kubernetes:
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
				return types.UnexpectedError, errBuf
			}

			orchestratorContext := podInfo.Name() + podInfo.Namespace()

			if service.state.ContainerIDByOrchestratorContext == nil {
				service.state.ContainerIDByOrchestratorContext = make(map[string]*ncList)
			}

			if _, ok := service.state.ContainerIDByOrchestratorContext[orchestratorContext]; !ok {
				service.state.ContainerIDByOrchestratorContext[orchestratorContext] = new(ncList)
			}

			ncs := service.state.ContainerIDByOrchestratorContext[orchestratorContext]
			ncs.Add(req.NetworkContainerid)

			logger.Printf("service.state.ContainerIDByOrchestratorContext[%s] is %+v", orchestratorContext, *service.state.ContainerIDByOrchestratorContext[orchestratorContext])

		case cns.KubernetesCRD:
			// Validate and Update the SecondaryIpConfig state
			returnCode, returnMesage := service.updateIPConfigsStateUntransacted(req, existingSecondaryIPConfigs, hostVersion)
			if returnCode != 0 {
				return returnCode, returnMesage
			}
		default:
			errMsg := fmt.Sprintf("Unsupported orchestrator type: %s", service.state.OrchestratorType)
			logger.Errorf(errMsg)
			return types.UnsupportedOrchestratorType, errMsg
		}

	default:
		errMsg := fmt.Sprintf("Unsupported network container type %s", req.NetworkContainerType)
		logger.Errorf(errMsg)
		return types.UnsupportedNetworkContainerType, errMsg
	}

	service.saveState()
	return 0, ""
}

// This func will compute the deltaIpConfigState which needs to be updated (Added or Deleted) from the inmemory map
// Note: Also this func is an untransacted API as the caller will take a Service lock
func (service *HTTPRestService) updateIPConfigsStateUntransacted(
	req cns.CreateNetworkContainerRequest, existingSecondaryIPConfigs map[string]cns.SecondaryIPConfig, hostVersion string,
) (types.ResponseCode, string) {
	// parse the existingSecondaryIpConfigState to find the deleted Ips
	newIPConfigs := req.SecondaryIPConfigs
	tobeDeletedIPConfigs := make(map[string]cns.SecondaryIPConfig)

	// Populate the ToBeDeleted list, Secondary IPs which doesnt exist in New request anymore.
	// We will later remove them from the in-memory cache
	for secondaryIpId, existingIPConfig := range existingSecondaryIPConfigs {
		_, exists := newIPConfigs[secondaryIpId]
		if !exists {
			// IP got removed in the updated request, add it in tobeDeletedIps
			tobeDeletedIPConfigs[secondaryIpId] = existingIPConfig
		}
	}

	// Validate TobeDeletedIps are ready to be deleted.
	for ipID := range tobeDeletedIPConfigs {
		ipConfigStatus, exists := service.PodIPConfigState[ipID]
		if exists {
			// pod ip exists, validate if state is not assigned, else fail
			if ipConfigStatus.GetState() == types.Assigned {
				errMsg := fmt.Sprintf("Failed to delete an Assigned IP %v", ipConfigStatus)
				return types.InconsistentIPConfigState, errMsg
			}
		}
	}

	// now actually remove the deletedIPs
	for ipID := range tobeDeletedIPConfigs {
		returncode, errMsg := service.removeToBeDeletedIPStateUntransacted(ipID, true)
		if returncode != types.Success {
			return returncode, errMsg
		}
	}

	// Add new IPs
	// TODO, will udpate NC version related variable to int, change it from string to int is a pains
	var hostNCVersionInInt int
	var err error
	if hostNCVersionInInt, err = strconv.Atoi(hostVersion); err != nil {
		return types.UnsupportedNCVersion, fmt.Sprintf("Invalid hostVersion is %s, err:%s", hostVersion, err)
	}
	service.addIPConfigStateUntransacted(req.NetworkContainerid, hostNCVersionInInt, req.SecondaryIPConfigs,
		existingSecondaryIPConfigs)

	return 0, ""
}

// addIPConfigStateUntransacted adds the IPConfigs to the PodIpConfigState map with Available state
// If the IP is already added then it will be an idempotent call. Also note, caller will
// acquire/release the service lock.
func (service *HTTPRestService) addIPConfigStateUntransacted(ncID string, hostVersion int, ipconfigs,
	existingSecondaryIPConfigs map[string]cns.SecondaryIPConfig,
) {
	// add ipconfigs to state
	for ipID, ipconfig := range ipconfigs {
		// New secondary IP configs has new NC version however, CNS don't want to override existing IPs'with new
		// NC version. Set it back to previous NC version if IP already exist.
		if existingIPConfig, existsInPreviousIPConfig := existingSecondaryIPConfigs[ipID]; existsInPreviousIPConfig {
			ipconfig.NCVersion = existingIPConfig.NCVersion
			ipconfigs[ipID] = ipconfig
		}

		if ipState, exists := service.PodIPConfigState[ipID]; exists {
			logger.Printf("[Azure-Cns] Set ipId %s, IP %s version to %d, programmed host nc version is %d, "+
				"ipState: %+v", ipID, ipconfig.IPAddress, ipconfig.NCVersion, hostVersion, ipState)
			continue
		}

		logger.Printf("[Azure-Cns] Set ipId %s, IP %s version to %d, programmed host nc version is %d",
			ipID, ipconfig.IPAddress, ipconfig.NCVersion, hostVersion)
		// Using the updated NC version attached with IP to compare with latest nmagent version and determine IP statues.
		// When reconcile, service.PodIPConfigState doens't exist, rebuild it with the help of NC version attached with IP.
		var newIPCNSStatus types.IPState
		if hostVersion < ipconfig.NCVersion {
			newIPCNSStatus = types.PendingProgramming
		} else {
			newIPCNSStatus = types.Available
		}
		// add the new State
		ipconfigStatus := cns.IPConfigurationStatus{
			NCID:      ncID,
			ID:        ipID,
			IPAddress: ipconfig.IPAddress,
			PodInfo:   nil,
		}
		ipconfigStatus.WithStateMiddleware(stateTransitionMiddleware)
		ipconfigStatus.SetState(newIPCNSStatus)
		logger.Printf("[Azure-Cns] Add IP %s as %s", ipconfig.IPAddress, newIPCNSStatus)

		service.PodIPConfigState[ipID] = ipconfigStatus

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

// removeToBeDeletedIPStateUntransacted removes IPConfigs from the PodIpConfigState map
// Caller will acquire/release the service lock.
func (service *HTTPRestService) removeToBeDeletedIPStateUntransacted(
	ipID string, skipValidation bool,
) (types.ResponseCode, string) {
	// this is set if caller has already done the validation
	if !skipValidation {
		ipConfigStatus, exists := service.PodIPConfigState[ipID]
		if exists {
			// pod ip exists, validate if state is not assigned, else fail
			if ipConfigStatus.GetState() == types.Assigned {
				errMsg := fmt.Sprintf("Failed to delete an Assigned IP %v", ipConfigStatus)
				return types.InconsistentIPConfigState, errMsg
			}
		}
	}

	// Delete this ip from PODIpConfigState Map
	logger.Printf("[Azure-Cns] Delete the PodIpConfigState, IpId: %s, IPConfigStatus: %v",
		ipID,
		service.PodIPConfigState[ipID])
	delete(service.PodIPConfigState, ipID)
	return 0, ""
}

func (service *HTTPRestService) getAllNetworkContainerResponses(
	req cns.GetNetworkContainerRequest,
) []cns.GetNetworkContainerResponse {
	var (
		getNetworkContainerResponse cns.GetNetworkContainerResponse
		ncs                         []string
		skipNCVersionCheck          = false
	)

	service.Lock()
	defer service.Unlock()

	switch service.state.OrchestratorType {
	case cns.Kubernetes, cns.ServiceFabric, cns.Batch, cns.DBforPostgreSQL, cns.AzureFirstParty:
		podInfo, err := cns.UnmarshalPodInfo(req.OrchestratorContext)
		getNetworkContainersResponse := []cns.GetNetworkContainerResponse{}

		if err != nil {
			response := cns.Response{
				ReturnCode: types.UnexpectedError,
				Message:    fmt.Sprintf("Unmarshalling orchestrator context failed with error %v", err),
			}

			getNetworkContainerResponse.Response = response
			getNetworkContainersResponse = append(getNetworkContainersResponse, getNetworkContainerResponse)
			return getNetworkContainersResponse
		}

		// get networkContainerIDs as string, "nc1, nc2"
		orchestratorContext := podInfo.Name() + podInfo.Namespace()
		if service.state.ContainerIDByOrchestratorContext[orchestratorContext] != nil {
			ncs = strings.Split(string(*service.state.ContainerIDByOrchestratorContext[orchestratorContext]), ",")
		}

		// This indicates that there are no ncs for the given orchestrator context
		if len(ncs) == 0 {
			response := cns.Response{
				ReturnCode: types.UnknownContainerID,
				Message:    fmt.Sprintf("Failed to find networkContainerID for orchestratorContext %s", orchestratorContext),
			}

			getNetworkContainerResponse.Response = response
			getNetworkContainersResponse = append(getNetworkContainersResponse, getNetworkContainerResponse)
			return getNetworkContainersResponse
		}

		ctx, cancel := context.WithTimeout(context.Background(), nmaAPICallTimeout)
		defer cancel()
		ncVersionListResp, err := service.nma.GetNCVersionList(ctx)
		if err != nil {
			skipNCVersionCheck = true
			logger.Errorf("failed to get nc version list from nmagent")
			// TODO: Add telemetry as this has potential to have containers in the running state w/o datapath working
		}
		nmaNCs := map[string]string{}
		for _, nc := range ncVersionListResp.Containers {
			nmaNCs[cns.SwiftPrefix+nc.NetworkContainerID] = nc.Version
		}

		if !skipNCVersionCheck {
			for _, ncid := range ncs {
				waitingForUpdate := false
				// If the goal state is available with CNS, check if the NC is pending VFP programming
				waitingForUpdate, getNetworkContainerResponse.Response.ReturnCode, getNetworkContainerResponse.Response.Message = service.isNCWaitingForUpdate(service.state.ContainerStatus[ncid].CreateNetworkContainerRequest.Version, ncid, nmaNCs) //nolint:lll // bad code
				// If the return code is not success, return the error to the caller
				if getNetworkContainerResponse.Response.ReturnCode == types.NetworkContainerVfpProgramPending {
					logger.Errorf("[Azure-CNS] isNCWaitingForUpdate failed for NCID: %s", ncid)
				}

				vfpUpdateComplete := !waitingForUpdate
				ncstatus := service.state.ContainerStatus[ncid]
				// Update the container status if-
				// 1. VfpUpdateCompleted successfully
				// 2. VfpUpdateComplete changed to false
				if (getNetworkContainerResponse.Response.ReturnCode == types.NetworkContainerVfpProgramComplete &&
					vfpUpdateComplete && ncstatus.VfpUpdateComplete != vfpUpdateComplete) ||
					(!vfpUpdateComplete && ncstatus.VfpUpdateComplete != vfpUpdateComplete) {
					logger.Printf("[Azure-CNS] Setting VfpUpdateComplete to %t for NCID: %s", vfpUpdateComplete, ncid)
					ncstatus.VfpUpdateComplete = vfpUpdateComplete
					service.state.ContainerStatus[ncid] = ncstatus
					if err = service.saveState(); err != nil {
						logger.Errorf("Failed to save goal states for nc %+v due to %s", getNetworkContainerResponse, err)
					}
				}
			}
		}

		if service.ChannelMode == cns.Managed {
			// If the NC goal state doesn't exist in CNS running in managed mode, call DNC to retrieve the goal state
			var (
				dncEP     = service.GetOption(acn.OptPrivateEndpoint).(string)
				infraVnet = service.GetOption(acn.OptInfrastructureNetworkID).(string)
				nodeID    = service.GetOption(acn.OptNodeID).(string)
			)

			service.Unlock()
			getNetworkContainerResponse.Response.ReturnCode, getNetworkContainerResponse.Response.Message = service.SyncNodeStatus(dncEP, infraVnet, nodeID, req.OrchestratorContext)
			service.Lock()
			if getNetworkContainerResponse.Response.ReturnCode == types.NotFound {
				getNetworkContainersResponse = append(getNetworkContainersResponse, getNetworkContainerResponse)
				return getNetworkContainersResponse
			}
		}
	default:
		getNetworkContainersResponse := []cns.GetNetworkContainerResponse{}
		response := cns.Response{
			ReturnCode: types.UnsupportedOrchestratorType,
			Message:    fmt.Sprintf("Invalid orchestrator type %v", service.state.OrchestratorType),
		}

		getNetworkContainerResponse.Response = response
		getNetworkContainersResponse = append(getNetworkContainersResponse, getNetworkContainerResponse)
		return getNetworkContainersResponse
	}

	getNetworkContainersResponse := []cns.GetNetworkContainerResponse{}

	for _, ncid := range ncs {
		containerStatus := service.state.ContainerStatus
		containerDetails, ok := containerStatus[ncid]
		if !ok {
			response := cns.Response{
				ReturnCode: types.UnknownContainerID,
				Message:    "NetworkContainer doesn't exist.",
			}

			getNetworkContainerResponse.Response = response
			getNetworkContainersResponse = append(getNetworkContainersResponse, getNetworkContainerResponse)
			continue
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

		// If the NC version check wasn't skipped, take into account the VFP programming status when returning the response
		if !skipNCVersionCheck {
			if !containerDetails.VfpUpdateComplete {
				getNetworkContainerResponse.Response = cns.Response{
					ReturnCode: types.NetworkContainerVfpProgramPending,
					Message:    "NetworkContainer VFP programming is pending",
				}
			}
		}
		getNetworkContainersResponse = append(getNetworkContainersResponse, getNetworkContainerResponse)
	}

	logger.Printf("getNetworkContainersResponses are %+v", getNetworkContainersResponse)

	return getNetworkContainersResponse
}

// restoreNetworkState restores Network state that existed before reboot.
func (service *HTTPRestService) restoreNetworkState() error {
	logger.Printf("[Azure CNS] Enter Restoring Network State")

	if service.store == nil {
		logger.Printf("[Azure CNS] Store is not initialized, nothing to restore for network state.")
		return nil
	}

	if !service.store.Exists() {
		logger.Printf("[Azure CNS] Store does not exist, nothing to restore for network state.")
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
			ReturnCode: types.InvalidParameter,
			Message:    "[Azure CNS] Error. " + operation + "ContainerToNetwork did not receive a POST.",
		}
	}
	if req.Containerid == "" {
		return cns.Response{
			ReturnCode: types.DockerContainerNotSpecified,
			Message:    "[Azure CNS] Error. Containerid is empty",
		}
	}
	if req.NetworkContainerid == "" {
		return cns.Response{
			ReturnCode: types.NetworkContainerNotSpecified,
			Message:    "[Azure CNS] Error. NetworkContainerid is empty",
		}
	}

	existing, ok := service.getNetworkContainerDetails(cns.SwiftPrefix + req.NetworkContainerid)
	if service.ChannelMode == cns.Managed && operation == attach {
		if ok {
			if !existing.VfpUpdateComplete {
				ctx, cancel := context.WithTimeout(context.Background(), nmaAPICallTimeout)
				defer cancel()
				ncVersionListResp, err := service.nma.GetNCVersionList(ctx)
				if err != nil {
					logger.Errorf("failed to get nc version list from nmagent")
					return cns.Response{
						ReturnCode: types.NmAgentInternalServerError,
						Message:    err.Error(),
					}
				}
				nmaNCs := map[string]string{}
				for _, nc := range ncVersionListResp.Containers {
					nmaNCs[nc.NetworkContainerID] = nc.Version
				}
				_, returnCode, message := service.isNCWaitingForUpdate(existing.CreateNetworkContainerRequest.Version, req.NetworkContainerid, nmaNCs)
				if returnCode == types.NetworkContainerVfpProgramPending {
					return cns.Response{
						ReturnCode: returnCode,
						Message:    message,
					}
				}
			}
		} else {
			var (
				dncEP     = service.GetOption(acn.OptPrivateEndpoint).(string)
				infraVnet = service.GetOption(acn.OptInfrastructureNetworkID).(string)
				nodeID    = service.GetOption(acn.OptNodeID).(string)
			)

			returnCode, msg := service.SyncNodeStatus(dncEP, infraVnet, nodeID, json.RawMessage{})
			if returnCode != types.Success {
				return cns.Response{
					ReturnCode: returnCode,
					Message:    msg,
				}
			}

			existing, _ = service.getNetworkContainerDetails(cns.SwiftPrefix + req.NetworkContainerid)
		}
	} else if !ok {
		return cns.Response{
			ReturnCode: types.NotFound,
			Message:    fmt.Sprintf("[Azure CNS] Error. Network Container %s does not exist.", req.NetworkContainerid),
		}
	}

	var returnCode types.ResponseCode
	var returnMessage string
	switch service.state.OrchestratorType {
	case cns.Batch:
		podInfo, err := cns.UnmarshalPodInfo(existing.CreateNetworkContainerRequest.OrchestratorContext)
		if err != nil {
			returnCode = types.UnexpectedError
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
				returnCode = types.UnexpectedError
				returnMessage = fmt.Sprintf("[Azure CNS] Error. "+operation+"ContainerToNetwork failed %+v", err.Error())
			}
		}

	default:
		returnMessage = fmt.Sprintf("[Azure CNS] Invalid orchestrator type %v", service.state.OrchestratorType)
		returnCode = types.UnsupportedOrchestratorType
	}

	return cns.Response{
		ReturnCode: returnCode,
		Message:    returnMessage,
	}
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

// areNCsPresent returns true if NCs are present in CNS, false if no NCs are present
func (service *HTTPRestService) areNCsPresent() bool {
	if len(service.state.ContainerStatus) == 0 && len(service.state.ContainerIDByOrchestratorContext) == 0 {
		return false
	}
	return true
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

func logNCSnapshot(createNetworkContainerRequest cns.CreateNetworkContainerRequest) {
	aiEvent := aitelemetry.Event{
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

func (service *HTTPRestService) validateIPConfigsRequest(
	ipConfigsRequest cns.IPConfigsRequest,
) (cns.PodInfo, types.ResponseCode, string) {
	if service.state.OrchestratorType != cns.KubernetesCRD && service.state.OrchestratorType != cns.Kubernetes {
		return nil, types.UnsupportedOrchestratorType, "ReleaseIPConfig API supported only for kubernetes orchestrator"
	}

	if ipConfigsRequest.OrchestratorContext == nil {
		return nil,
			types.EmptyOrchestratorContext,
			fmt.Sprintf("OrchastratorContext is not set in the req: %+v", ipConfigsRequest)
	}

	// retrieve podinfo from orchestrator context
	podInfo, err := cns.NewPodInfoFromIPConfigsRequest(ipConfigsRequest)
	if err != nil {
		return podInfo, types.UnsupportedOrchestratorContext, err.Error()
	}
	return podInfo, types.Success, ""
}

// getPrimaryHostInterface returns the cached InterfaceInfo, if available, otherwise
// queries the IMDS to get the primary interface info and caches it in the server state
// before returning the result.
func (service *HTTPRestService) getPrimaryHostInterface(ctx context.Context) (*wireserver.InterfaceInfo, error) {
	if service.state.primaryInterface == nil {
		res, err := service.wscli.GetInterfaces(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get interfaces from IMDS")
		}
		primary, err := wireserver.GetPrimaryInterfaceFromResult(res)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get primary interface from IMDS response")
		}
		service.state.primaryInterface = primary
	}
	return service.state.primaryInterface, nil
}

//nolint:gocritic // ignore hugeParam pls
func (service *HTTPRestService) populateIPConfigInfoUntransacted(ipConfigStatus cns.IPConfigurationStatus, podIPInfo *cns.PodIpInfo) error {
	ncStatus, exists := service.state.ContainerStatus[ipConfigStatus.NCID]
	if !exists {
		return fmt.Errorf("Failed to get NC Configuration for NcId: %s", ipConfigStatus.NCID)
	}

	primaryIPCfg := ncStatus.CreateNetworkContainerRequest.IPConfiguration

	podIPInfo.PodIPConfig = cns.IPSubnet{
		IPAddress:    ipConfigStatus.IPAddress,
		PrefixLength: primaryIPCfg.IPSubnet.PrefixLength,
	}

	podIPInfo.NetworkContainerPrimaryIPConfig = primaryIPCfg
	primaryHostInterface, err := service.getPrimaryHostInterface(context.TODO())
	if err != nil {
		return err
	}

	podIPInfo.HostPrimaryIPInfo.PrimaryIP = primaryHostInterface.PrimaryIP
	podIPInfo.HostPrimaryIPInfo.Subnet = primaryHostInterface.Subnet
	podIPInfo.HostPrimaryIPInfo.Gateway = primaryHostInterface.Gateway

	return nil
}

// isNCWaitingForUpdate :- Determine whether NC version on NMA matches programmed version
// Return error and waitingForUpdate as true only CNS gets response from NMAgent indicating
// the VFP programming is pending
// This returns success / waitingForUpdate as false in all other cases.
// V2 is using the nmagent get nc version list api v2 which doesn't need authentication token
func (service *HTTPRestService) isNCWaitingForUpdate(
	ncVersion, ncid string, ncVersionList map[string]string,
) (waitingForUpdate bool, returnCode types.ResponseCode, message string) {
	ncStatus, ok := service.state.ContainerStatus[ncid]
	if ok {
		if ncStatus.VfpUpdateComplete &&
			(ncStatus.CreateNetworkContainerRequest.Version == ncVersion) {
			logger.Printf("[Azure CNS] Network container: %s, version: %s has VFP programming already completed", ncid, ncVersion)
			return false, types.NetworkContainerVfpProgramCheckSkipped, ""
		}
	}

	ncTargetVersion, err := strconv.Atoi(ncVersion)
	if err != nil {
		// NMA doesn't have this NC version in string type, bail out
		logger.Printf("[Azure CNS] NC %s version %v from NMAgent NC version list is not string "+
			"Skipping GetNCVersionStatus check from NMAgent", ncVersion, ncid)
		return true, types.NetworkContainerVfpProgramPending, ""
	}

	// accept both upper and lower GUID from ncid(Swift_GUID)
	// check each ncid in lower case if it's in ncVersionList
	nmaProgrammedNCVersionStr, ok := ncVersionList[strings.ToLower(ncid)]
	if !ok {
		// NMA doesn't have this NC that we need programmed yet, bail out
		logger.Printf("[Azure CNS] Failed to get NC %s doesn't exist in NMAgent NC version list "+
			"Skipping GetNCVersionStatus check from NMAgent", ncid)
		return true, types.NetworkContainerVfpProgramPending, ""

	}

	nmaProgrammedNCVersion, err := strconv.Atoi(nmaProgrammedNCVersionStr)
	if err != nil {
		// it's unclear whether or not this can actually happen. In the NMAgent
		// documentation, Version is described as a string, but in practice the
		// values appear to be exclusively integers. Nevertheless, NMAgent is
		// allowed to make this parameter anything (by contract), so we should
		// defend against it by erroring appropriately:
		logger.Printf("[Azure CNS] Failed to get NC version status from NMAgent with error: %+v. "+
			"Skipping GetNCVersionStatus check from NMAgent", err)
		return true, types.NetworkContainerVfpProgramCheckSkipped, ""
	}

	if ncTargetVersion > nmaProgrammedNCVersion {
		msg := fmt.Sprintf("Network container: %s version: %d is not yet programmed by NMAgent. Programmed version: %d",
			ncid, ncTargetVersion, nmaProgrammedNCVersion)
		return false, types.NetworkContainerVfpProgramPending, msg
	}

	msg := "Vfp programming complete"
	logger.Printf("[Azure CNS] Vfp programming complete for NC: %s with version: %d", ncid, ncTargetVersion)
	return false, types.NetworkContainerVfpProgramComplete, msg
}

// handleGetNetworkContainers returns all NCs in CNS
func (service *HTTPRestService) handleGetNetworkContainers(w http.ResponseWriter) {
	logger.Printf("[Azure CNS] handleGetNetworkContainers")
	service.RLock()
	networkContainers := make([]cns.GetNetworkContainerResponse, len(service.state.ContainerStatus))
	i := 0
	for ncID := range service.state.ContainerStatus {
		ncDetails := service.state.ContainerStatus[ncID]
		getNcResp := cns.GetNetworkContainerResponse{
			NetworkContainerID:         ncDetails.CreateNetworkContainerRequest.NetworkContainerid,
			IPConfiguration:            ncDetails.CreateNetworkContainerRequest.IPConfiguration,
			Routes:                     ncDetails.CreateNetworkContainerRequest.Routes,
			CnetAddressSpace:           ncDetails.CreateNetworkContainerRequest.CnetAddressSpace,
			MultiTenancyInfo:           ncDetails.CreateNetworkContainerRequest.MultiTenancyInfo,
			PrimaryInterfaceIdentifier: ncDetails.CreateNetworkContainerRequest.PrimaryInterfaceIdentifier,
			LocalIPConfiguration:       ncDetails.CreateNetworkContainerRequest.LocalIPConfiguration,
			AllowHostToNCCommunication: ncDetails.CreateNetworkContainerRequest.AllowHostToNCCommunication,
			AllowNCToHostCommunication: ncDetails.CreateNetworkContainerRequest.AllowNCToHostCommunication,
		}
		networkContainers[i] = getNcResp
		i++
	}
	service.RUnlock()

	response := cns.GetAllNetworkContainersResponse{
		NetworkContainers: networkContainers,
		Response: cns.Response{
			ReturnCode: types.Success,
		},
	}
	err := service.Listener.Encode(w, &response)
	logger.Response(service.Name, response, response.Response.ReturnCode, err)
}

// handlePostNetworkContainers stores all the NCs (from the request that client sent) into CNS's state file
func (service *HTTPRestService) handlePostNetworkContainers(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] handlePostNetworkContainers")
	var req cns.PostNetworkContainersRequest
	err := service.Listener.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)
	if err != nil {
		response := cns.PostNetworkContainersResponse{
			Response: cns.Response{
				ReturnCode: types.InvalidRequest,
				Message:    fmt.Sprintf("[Azure CNS] handlePostNetworkContainers failed with error: %s", err.Error()),
			},
		}
		err = service.Listener.Encode(w, &response)
		logger.Response(service.Name, response, response.Response.ReturnCode, err)
		return
	}

	createNCsResp := service.createNetworkContainers(req.CreateNetworkContainerRequests)
	response := cns.PostNetworkContainersResponse{
		Response: createNCsResp,
	}
	err = service.Listener.Encode(w, &response)
	logger.Response(service.Name, response, response.Response.ReturnCode, err)
}

func (service *HTTPRestService) createNetworkContainers(createNetworkContainerRequests []cns.CreateNetworkContainerRequest) cns.Response {
	for i := 0; i < len(createNetworkContainerRequests); i++ {
		createNcReq := createNetworkContainerRequests[i]
		ncDetails, found := service.getNetworkContainerDetails(createNcReq.NetworkContainerid)
		// Create NC if it doesn't exist, or it exists and the requested version is different from the saved version
		if !found || (found && ncDetails.VMVersion != createNcReq.Version) {
			nc := service.networkContainer
			if err := nc.Create(createNcReq); err != nil {
				return cns.Response{
					ReturnCode: types.UnexpectedError,
					Message:    fmt.Sprintf("[Azure CNS] Create Network Containers failed with error: %s", err.Error()),
				}
			}
		}
		// Save NC Goal State details
		saveNcReturnCode, saveNcReturnMessage := service.saveNetworkContainerGoalState(createNcReq)
		// If NC was created successfully, log NC snapshot.
		if saveNcReturnCode != types.Success {
			return cns.Response{
				ReturnCode: saveNcReturnCode,
				Message:    saveNcReturnMessage,
			}
		}
		logNCSnapshot(createNcReq)
	}

	return cns.Response{
		ReturnCode: types.Success,
		Message:    "",
	}
}

// setResponse encodes the http response
func (service *HTTPRestService) setResponse(w http.ResponseWriter, returnCode types.ResponseCode, response interface{}) {
	serviceErr := service.Listener.Encode(w, &response)
	logger.Response(service.Name, response, returnCode, serviceErr)
}

// ncList contains comma-separated list of unique NCs
type ncList string

// only add unique NC to ncList
func (n *ncList) Add(nc string) {
	var ncs []string
	if len(*n) > 0 {
		ncs = strings.Split(string(*n), ",")
	}
	for _, v := range ncs {
		// if NC is already present in ncList, do not add it
		if nc == v {
			return
		}
	}

	ncs = append(ncs, nc)
	*n = ncList(strings.Join(ncs, ","))
}

// delete nc from ncList
// split the slice around the index that contains the NC to delete so that neigher of two resulting nc slices cotnains this NC
// use append menthod to join the new NC slices
func (n *ncList) Delete(nc string) {
	ncs := strings.Split(string(*n), ",")
	for i, v := range ncs {
		if nc == v {
			ncs = append((ncs)[:i], (ncs)[i+1:]...)
			break
		}
	}
	*n = ncList(strings.Join(ncs, ","))
}

// check if ncList contains NC
func (n *ncList) Contains(nc string) bool {
	return strings.Contains(string(*n), nc)
}
