// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"encoding/json"
	"fmt"
	"net/http"
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
func (service *HTTPRestService) restoreState() error {
	logger.Printf("[Azure CNS] restoreState")

	// Skip if a store is not provided.
	if service.store == nil {
		logger.Printf("[Azure CNS]  store not initialized.")
		return nil
	}

	// Read any persisted state.
	err := service.store.Read(storeKey, &service.state)
	if err != nil {
		if err == store.ErrKeyNotFound {
			// Nothing to restore.
			logger.Printf("[Azure CNS]  No state to restore.\n")
			return nil
		}

		logger.Errorf("[Azure CNS]  Failed to restore state, err:%v\n", err)
		return err
	}

	logger.Printf("[Azure CNS]  Restored state, %+v\n", service.state)
	return nil
}

func (service *HTTPRestService) saveNetworkContainerGoalState(req cns.CreateNetworkContainerRequest) (int, string) {
	// we don't want to overwrite what other calls may have written
	service.Lock()
	defer service.Unlock()

	existing, ok := service.state.ContainerStatus[req.NetworkContainerid]
	var hostVersion string
	if ok {
		hostVersion = existing.HostVersion
	}

	if service.state.ContainerStatus == nil {
		service.state.ContainerStatus = make(map[string]containerstatus)
	}

	service.state.ContainerStatus[req.NetworkContainerid] =
		containerstatus{
			ID:                            req.NetworkContainerid,
			VMVersion:                     req.Version,
			CreateNetworkContainerRequest: req,
			HostVersion:                   hostVersion}

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
		case cns.WebApps:
			var podInfo cns.KubernetesPodInfo
			err := json.Unmarshal(req.OrchestratorContext, &podInfo)
			if err != nil {
				errBuf := fmt.Sprintf("Unmarshalling %s failed with error %v", req.NetworkContainerType, err)
				return UnexpectedError, errBuf
			}

			logger.Printf("Pod info %v", podInfo)

			if service.state.ContainerIDByOrchestratorContext == nil {
				service.state.ContainerIDByOrchestratorContext = make(map[string]string)
			}

			service.state.ContainerIDByOrchestratorContext[podInfo.PodName+podInfo.PodNamespace] = req.NetworkContainerid
			break

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

func (service *HTTPRestService) getNetworkContainerResponse(req cns.GetNetworkContainerRequest) cns.GetNetworkContainerResponse {
	var containerID string
	var getNetworkContainerResponse cns.GetNetworkContainerResponse

	service.RLock()
	defer service.RUnlock()

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
		var podInfo cns.KubernetesPodInfo
		err := json.Unmarshal(req.OrchestratorContext, &podInfo)
		if err != nil {
			getNetworkContainerResponse.Response.ReturnCode = UnexpectedError
			getNetworkContainerResponse.Response.Message = fmt.Sprintf("Unmarshalling orchestrator context failed with error %v", err)
			return getNetworkContainerResponse
		}

		logger.Printf("pod info %+v", podInfo)
		containerID = service.state.ContainerIDByOrchestratorContext[podInfo.PodName+podInfo.PodNamespace]
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

	if !ok {
		return cns.Response{
			ReturnCode: NotFound,
			Message:    fmt.Sprintf("[Azure CNS] Error. Network Container %s does not exist.", req.NetworkContainerid)}
	}

	returnCode := 0
	returnMessage := ""
	switch service.state.OrchestratorType {
	case cns.Batch:
		var podInfo cns.KubernetesPodInfo
		err := json.Unmarshal(existing.CreateNetworkContainerRequest.OrchestratorContext, &podInfo)
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
func (service *HTTPRestService) SendNCSnapShotPeriodically(ncSnapshotIntervalInMinutes int, stopSnapshot chan bool) {

	// Emit snapshot on startup and then emit it periodically.
	service.logNCSnapshots()

	snapshot := time.NewTicker(time.Minute * time.Duration(ncSnapshotIntervalInMinutes)).C
	for {
		select {
		case <-snapshot:
			service.logNCSnapshots()
		case <-stopSnapshot:
			return
		}
	}
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
	default:
		s = "UnknownError"
	}

	return
}
