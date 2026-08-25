package restserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/dockerclient"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/networkcontainers"
	"github.com/Azure/azure-container-networking/cns/nodesubnet"
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

func (service *HTTPRestService) SavePnpIDMacaddressMapping(ctx context.Context) error {
	// If mapping is already set, skip setting it again.
	if len(service.state.PnpIDByMacAddress) != 0 {
		return nil
	}
	p := platform.NewExecClient(nil)
	vfMacAddressMapping, err := platform.FetchMacAddressPnpIDMapping(ctx, p)
	if err != nil {
		return errors.Wrap(err, "failed to fetch MACAddressPnpIDMapping")
	}
	service.state.PnpIDByMacAddress = vfMacAddressMapping
	if err = service.saveState(); err != nil {
		logger.Errorf("Failed to save mapping to statefile: %v", err)
	}
	return nil
}

func (service *HTTPRestService) getPNPIDFromMacAddress(ctx context.Context, macAddress string) (string, error) {
	// If map is empty in state file, CNS needs to populate state file before it returns back the response
	if len(service.state.PnpIDByMacAddress) == 0 {
		if err := service.SavePnpIDMacaddressMapping(ctx); err != nil {
			return "", err
		}
	}
	if _, ok := service.state.PnpIDByMacAddress[macAddress]; !ok {
		return "", errors.New("Backend Network adapter not found")
	}
	return service.state.PnpIDByMacAddress[macAddress], nil
}

// Remove the network info from the service network state
func (service *HTTPRestService) removeNetworkInfo(networkName string) {
	service.Lock()
	defer service.Unlock()
	delete(service.state.Networks, networkName)
}

// saveState writes CNS state to persistent store.
func (service *HTTPRestService) saveState() error {
	err := service.writeState(service.state)
	if err != nil {
		logger.Errorf("[Azure CNS] Failed to save state, err: %v", err)
	}

	return err
}

func (service *HTTPRestService) writeState(state *httpRestServiceState) error {
	// Skip if a store is not provided.
	if service.store == nil {
		logger.Printf("[Azure CNS] store not initialized.")
		return nil
	}

	// Update time stamp.
	state.TimeStamp = time.Now()
	return service.store.Write(storeKey, state)
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
	} else {
		logger.Printf("[Azure CNS]  Restored state, %+v\n", service.state) //nolint:staticcheck // TODO: migrate to zap
	}

	if service.Options[acn.OptManageEndpointState] == true {
		if service.EndpointStateStore == nil {
			//nolint:staticcheck // TODO: migrate to zap
			logger.Errorf("[Azure CNS]  OptManageEndpointState is enabled but EndpointStateStore is not initialized; endpoint state persistence/restoration is disabled.")
			return
		}
		err := service.EndpointStateStore.Read(EndpointStoreKey, &service.EndpointState)
		if err != nil {
			if errors.Is(err, store.ErrKeyNotFound) {
				// Nothing to restore.
				logger.Printf("[Azure CNS]  No endpoint state to restore.\n")
			} else {
				//nolint:staticcheck // TODO: migrate to zap
				logger.Errorf("[Azure CNS]  Failed to restore endpoint state, err:%v", err)
			}
			return
		}
		logger.Printf("[Azure CNS]  Restored endpoint state, %+v\n", service.EndpointState)

	}
}

func (service *HTTPRestService) saveNetworkContainerGoalState(
	req cns.CreateNetworkContainerRequest,
	validateVersion bool,
) (responseCode types.ResponseCode, message string) {
	service.Lock()
	defer service.Unlock()

	var (
		hostVersion                string
		existingSecondaryIPConfigs map[string]cns.SecondaryIPConfig // uuid is key
		vfpUpdateComplete          bool
	)

	existingNCStatus, ok := service.state.ContainerStatus[req.NetworkContainerid]
	if ok {
		hostVersion = existingNCStatus.HostVersion
		existingSecondaryIPConfigs = existingNCStatus.CreateNetworkContainerRequest.SecondaryIPConfigs
		vfpUpdateComplete = existingNCStatus.VfpUpdateComplete

		if validateVersion {
			if returnCode, message := validateNCGoalVersion(existingNCStatus.CreateNetworkContainerRequest, req); returnCode != types.Success {
				return returnCode, message
			}
		}
	}

	if req.NetworkContainerid == nodesubnet.NodeSubnetNCID {
		hostVersion = nodesubnet.NodeSubnetHostVersion
		vfpUpdateComplete = true
	}

	if hostVersion == "" {
		// Host version is the NC version from NMAgent, set it -1 to indicate no result from NMAgent yet.
		// TODO, query NMAgent and with aggresive time out and assign latest host version.
		hostVersion = "-1"
	}

	var (
		ipPlan                   ipConfigsUpdatePlan
		orchestratorContext      string
		updateOrchestratorLookup bool
	)
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
	case cns.COW, cns.BackendNICNC, cns.WebApps:
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
		case cns.WebApps, cns.BackendNICNC: // todo: Is WebApps an OrchastratorType or ContainerType?
			podInfo, err := cns.UnmarshalPodInfo(req.OrchestratorContext)
			if err != nil {
				return types.UnexpectedError, fmt.Sprintf("unmarshalling %s orchestrator context: %v", req.NetworkContainerType, err)
			}
			orchestratorContext = podInfo.Name() + podInfo.Namespace()
			updateOrchestratorLookup = true

		case cns.KubernetesCRD:
			var returnCode types.ResponseCode
			var message string
			ipPlan, returnCode, message = service.buildIPConfigsUpdatePlan(req, existingSecondaryIPConfigs, hostVersion)
			if returnCode != types.Success {
				return returnCode, message
			}
		default:
			return types.UnsupportedOrchestratorType, fmt.Sprintf("unsupported orchestrator type %s", service.state.OrchestratorType)
		}

	default:
		return types.UnsupportedNetworkContainerType, fmt.Sprintf("unsupported network container type %s", req.NetworkContainerType)
	}

	createNetworkContainerRequest := copyCreateNetworkContainerRequest(req, existingSecondaryIPConfigs)
	candidateState := copyHTTPRestServiceState(service.state)
	if candidateState.ContainerStatus == nil {
		candidateState.ContainerStatus = make(map[string]containerstatus)
	}
	candidateState.ContainerStatus[req.NetworkContainerid] = containerstatus{
		ID:                            req.NetworkContainerid,
		VMVersion:                     req.Version,
		CreateNetworkContainerRequest: createNetworkContainerRequest,
		HostVersion:                   hostVersion,
		VfpUpdateComplete:             vfpUpdateComplete,
	}

	if updateOrchestratorLookup {
		if candidateState.ContainerIDByOrchestratorContext == nil {
			candidateState.ContainerIDByOrchestratorContext = make(map[string]*ncList)
		}
		if _, exists := candidateState.ContainerIDByOrchestratorContext[orchestratorContext]; !exists {
			candidateState.ContainerIDByOrchestratorContext[orchestratorContext] = new(ncList)
		}
		candidateState.ContainerIDByOrchestratorContext[orchestratorContext].Add(req.NetworkContainerid)
	}

	if err := service.writeState(candidateState); err != nil {
		return types.UnexpectedError, fmt.Sprintf("persisting goal state for nc %s: %v", req.NetworkContainerid, err)
	}

	service.state = candidateState
	if service.state.OrchestratorType == cns.KubernetesCRD {
		if service.PodIPConfigState == nil {
			service.PodIPConfigState = make(map[string]cns.IPConfigurationStatus)
		}
		service.applyIPConfigsUpdatePlan(createNetworkContainerRequest, ipPlan)
	}
	if updateOrchestratorLookup {
		logger.Printf(
			"service.state.ContainerIDByOrchestratorContext[%s] is %+v",
			orchestratorContext,
			*service.state.ContainerIDByOrchestratorContext[orchestratorContext],
		)
	}

	return types.Success, ""
}

func validateNCGoalVersion(
	existing,
	incoming cns.CreateNetworkContainerRequest,
) (responseCode types.ResponseCode, message string) {
	existingVersion, err := strconv.Atoi(existing.Version)
	if err != nil {
		return types.UnsupportedNCVersion, fmt.Sprintf(
			"invalid committed nc version %q for nc %s: %v",
			existing.Version,
			incoming.NetworkContainerid,
			err,
		)
	}

	incomingVersion, err := strconv.Atoi(incoming.Version)
	if err != nil {
		return types.UnsupportedNCVersion, fmt.Sprintf(
			"invalid incoming nc version %q for nc %s: %v",
			incoming.Version,
			incoming.NetworkContainerid,
			err,
		)
	}

	switch {
	case incomingVersion < existingVersion:
		return types.UnsupportedNCVersion, fmt.Sprintf(
			"nc %s version regressed from %d to %d",
			incoming.NetworkContainerid,
			existingVersion,
			incomingVersion,
		)
	case incomingVersion == existingVersion && !equalNNCNetworkProgrammingGoal(existing, incoming):
		return types.InconsistentIPConfigState, fmt.Sprintf(
			"nc %s goal changed without version advance at version %d",
			incoming.NetworkContainerid,
			incomingVersion,
		)
	default:
		return types.Success, ""
	}
}

func equalNNCNetworkProgrammingGoal(existing, incoming cns.CreateNetworkContainerRequest) bool {
	if existing.NetworkContainerid != incoming.NetworkContainerid ||
		existing.NetworkContainerType != incoming.NetworkContainerType ||
		existing.HostPrimaryIP != incoming.HostPrimaryIP ||
		existing.NCStatus != incoming.NCStatus ||
		existing.NetworkInterfaceInfo != incoming.NetworkInterfaceInfo ||
		!equalIPConfiguration(existing.IPConfiguration, incoming.IPConfiguration) ||
		len(existing.SecondaryIPConfigs) != len(incoming.SecondaryIPConfigs) {
		return false
	}

	for ipID, existingConfig := range existing.SecondaryIPConfigs {
		incomingConfig, ok := incoming.SecondaryIPConfigs[ipID]
		if !ok || existingConfig.IPAddress != incomingConfig.IPAddress {
			return false
		}
	}

	return true
}

func equalIPConfiguration(existing, incoming cns.IPConfiguration) bool {
	return existing.IPSubnet == incoming.IPSubnet &&
		existing.IPSubnetV6 == incoming.IPSubnetV6 &&
		slices.Equal(existing.DNSServers, incoming.DNSServers) &&
		existing.GatewayIPAddress == incoming.GatewayIPAddress &&
		existing.GatewayIPv6Address == incoming.GatewayIPv6Address
}

func getToBeDeletedIPConfigs(
	existingIPConfigs,
	incomingIPConfigs map[string]cns.SecondaryIPConfig,
) map[string]cns.SecondaryIPConfig {
	toBeDeletedIPConfigs := make(map[string]cns.SecondaryIPConfig)
	for ipID, existingIPConfig := range existingIPConfigs {
		if _, exists := incomingIPConfigs[ipID]; !exists {
			toBeDeletedIPConfigs[ipID] = existingIPConfig
		}
	}
	return toBeDeletedIPConfigs
}

type ipConfigIdentity struct {
	ipID string
	ncID string
}

func validateUniqueIPAddresses(
	currentIPConfigs map[string]cns.IPConfigurationStatus,
	toBeDeletedIPConfigs map[string]cns.SecondaryIPConfig,
	incomingIPConfigs map[string]cns.SecondaryIPConfig,
	incomingNCID string,
) (responseCode types.ResponseCode, message string) {
	identitiesByAddress := make(map[netip.Addr]ipConfigIdentity, len(currentIPConfigs)+len(incomingIPConfigs))
	currentAddressesByID := make(map[string]netip.Addr, len(currentIPConfigs))

	for ipID := range currentIPConfigs {
		if _, deleting := toBeDeletedIPConfigs[ipID]; deleting {
			continue
		}
		currentIPConfig := currentIPConfigs[ipID]

		address, err := netip.ParseAddr(currentIPConfig.IPAddress)
		if err != nil {
			return types.InvalidSecondaryIPConfig, fmt.Sprintf(
				"invalid IP address %q for IP ID %s in nc %s: %v",
				currentIPConfig.IPAddress,
				ipID,
				currentIPConfig.NCID,
				err,
			)
		}
		address = address.Unmap()
		if existing, duplicate := identitiesByAddress[address]; duplicate && existing.ipID != ipID {
			return duplicateIPAddressError(address, existing, ipConfigIdentity{ipID: ipID, ncID: currentIPConfig.NCID})
		}

		currentIdentity := ipConfigIdentity{ipID: ipID, ncID: currentIPConfig.NCID}
		identitiesByAddress[address] = currentIdentity
		currentAddressesByID[ipID] = address
	}

	for ipID, incomingIPConfig := range incomingIPConfigs {
		address, err := netip.ParseAddr(incomingIPConfig.IPAddress)
		if err != nil {
			return types.InvalidSecondaryIPConfig, fmt.Sprintf(
				"invalid IP address %q for IP ID %s in nc %s: %v",
				incomingIPConfig.IPAddress,
				ipID,
				incomingNCID,
				err,
			)
		}
		address = address.Unmap()

		if currentAddress, exists := currentAddressesByID[ipID]; exists {
			if currentAddress != address {
				return types.InconsistentIPConfigState, fmt.Sprintf(
					"ip ID %s in nc %s changed address from %s to %s",
					ipID,
					incomingNCID,
					currentAddress,
					address,
				)
			}
			continue
		}

		incomingIdentity := ipConfigIdentity{ipID: ipID, ncID: incomingNCID}
		if existing, duplicate := identitiesByAddress[address]; duplicate {
			return duplicateIPAddressError(address, existing, incomingIdentity)
		}
		identitiesByAddress[address] = incomingIdentity
	}

	return types.Success, ""
}

func duplicateIPAddressError(
	address netip.Addr,
	existing,
	incoming ipConfigIdentity,
) (responseCode types.ResponseCode, message string) {
	return types.InconsistentIPConfigState, fmt.Sprintf(
		"duplicate IP %s for IP IDs %s in nc %s and %s in nc %s",
		address,
		existing.ipID,
		existing.ncID,
		incoming.ipID,
		incoming.ncID,
	)
}

func copyCreateNetworkContainerRequest(
	req cns.CreateNetworkContainerRequest,
	existingSecondaryIPConfigs map[string]cns.SecondaryIPConfig,
) cns.CreateNetworkContainerRequest {
	copied := req
	copied.AuthorizationToken = ""
	copied.OrchestratorContext = append([]byte(nil), req.OrchestratorContext...)
	copied.LocalIPConfiguration = copyIPConfiguration(req.LocalIPConfiguration)
	copied.IPConfiguration = copyIPConfiguration(req.IPConfiguration)
	copied.IPv6Configuration = copyIPConfiguration(req.IPv6Configuration)
	copied.SecondaryIPConfigs = make(map[string]cns.SecondaryIPConfig, len(req.SecondaryIPConfigs))
	for ipID, ipConfig := range req.SecondaryIPConfigs {
		if existingIPConfig, exists := existingSecondaryIPConfigs[ipID]; exists {
			ipConfig.NCVersion = existingIPConfig.NCVersion
		}
		copied.SecondaryIPConfigs[ipID] = ipConfig
	}
	copied.CnetAddressSpace = append([]cns.IPSubnet(nil), req.CnetAddressSpace...)
	copied.Routes = append([]cns.Route(nil), req.Routes...)
	copied.EndpointPolicies = make([]cns.NetworkContainerRequestPolicies, len(req.EndpointPolicies))
	for i, policy := range req.EndpointPolicies {
		copied.EndpointPolicies[i] = policy
		copied.EndpointPolicies[i].Settings = append([]byte(nil), policy.Settings...)
	}
	return copied
}

func copyIPConfiguration(config cns.IPConfiguration) cns.IPConfiguration {
	copied := config
	copied.DNSServers = append([]string(nil), config.DNSServers...)
	return copied
}

func copyHTTPRestServiceState(state *httpRestServiceState) *httpRestServiceState {
	copied := *state
	if state.ContainerStatus != nil {
		copied.ContainerStatus = make(map[string]containerstatus, len(state.ContainerStatus))
		for ncID, status := range state.ContainerStatus {
			copied.ContainerStatus[ncID] = status
		}
	}
	if state.ContainerIDByOrchestratorContext != nil {
		copied.ContainerIDByOrchestratorContext = make(map[string]*ncList, len(state.ContainerIDByOrchestratorContext))
		for orchestratorContext, networkContainerIDs := range state.ContainerIDByOrchestratorContext {
			if networkContainerIDs == nil {
				copied.ContainerIDByOrchestratorContext[orchestratorContext] = nil
				continue
			}
			copiedNetworkContainerIDs := *networkContainerIDs
			copied.ContainerIDByOrchestratorContext[orchestratorContext] = &copiedNetworkContainerIDs
		}
	}
	return &copied
}

type ipConfigsUpdatePlan struct {
	toBeDeletedIPConfigs map[string]cns.SecondaryIPConfig
	hostVersion          int
}

func (service *HTTPRestService) buildIPConfigsUpdatePlan(
	req cns.CreateNetworkContainerRequest,
	existingSecondaryIPConfigs map[string]cns.SecondaryIPConfig,
	hostVersion string,
) (ipConfigsUpdatePlan, types.ResponseCode, string) {
	plan := ipConfigsUpdatePlan{
		toBeDeletedIPConfigs: getToBeDeletedIPConfigs(existingSecondaryIPConfigs, req.SecondaryIPConfigs),
	}

	if returnCode, message := validateUniqueIPAddresses(
		service.PodIPConfigState,
		plan.toBeDeletedIPConfigs,
		req.SecondaryIPConfigs,
		req.NetworkContainerid,
	); returnCode != types.Success {
		return ipConfigsUpdatePlan{}, returnCode, message
	}

	for ipID := range plan.toBeDeletedIPConfigs {
		ipConfigStatus, exists := service.PodIPConfigState[ipID]
		if exists && ipConfigStatus.GetState() == types.Assigned {
			return ipConfigsUpdatePlan{}, types.InconsistentIPConfigState, fmt.Sprintf(
				"cannot delete assigned IP %s with IP ID %s from nc %s",
				ipConfigStatus.IPAddress,
				ipID,
				ipConfigStatus.NCID,
			)
		}
	}

	hostVersionInt, err := strconv.Atoi(hostVersion)
	if err != nil {
		return ipConfigsUpdatePlan{}, types.UnsupportedNCVersion, fmt.Sprintf("invalid host version %q: %v", hostVersion, err)
	}
	plan.hostVersion = hostVersionInt

	return plan, types.Success, ""
}

func (service *HTTPRestService) applyIPConfigsUpdatePlan(
	req cns.CreateNetworkContainerRequest,
	plan ipConfigsUpdatePlan,
) {
	for ipID := range plan.toBeDeletedIPConfigs {
		logger.Printf(
			"[Azure-Cns] Delete the PodIpConfigState, IpId: %s, IPConfigStatus: %v",
			ipID,
			service.PodIPConfigState[ipID],
		)
		delete(service.PodIPConfigState, ipID)
	}

	service.addIPConfigStateUntransacted(
		req.NetworkContainerid,
		plan.hostVersion,
		req.SecondaryIPConfigs,
	)
}

// addIPConfigStateUntransacted adds the IPConfigs to the PodIpConfigState map with Available state
// If the IP is already added then it will be an idempotent call. Also note, caller will
// acquire/release the service lock.
func (service *HTTPRestService) addIPConfigStateUntransacted(
	ncID string,
	hostVersion int,
	ipconfigs map[string]cns.SecondaryIPConfig,
) {
	// add ipconfigs to state
	for ipID, ipconfig := range ipconfigs {
		if ipState, exists := service.PodIPConfigState[ipID]; exists {
			logger.Printf("[Azure-Cns] Set ipId %s, IP %s version to %d, programmed host nc version is %d, "+
				"ipState: %s", ipID, ipconfig.IPAddress, ipconfig.NCVersion, hostVersion, ipState)
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
			// store nmaNCID as lower case to allow case insensitive comparison with nc stored in CNS
			nmaNCs[strings.TrimPrefix(lowerCaseNCGuid(nc.NetworkContainerID), cns.SwiftPrefix)] = nc.Version
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
			IPv6Configuration:          savedReq.IPv6Configuration,
			Routes:                     savedReq.Routes,
			CnetAddressSpace:           savedReq.CnetAddressSpace,
			MultiTenancyInfo:           savedReq.MultiTenancyInfo,
			PrimaryInterfaceIdentifier: savedReq.PrimaryInterfaceIdentifier,
			LocalIPConfiguration:       savedReq.LocalIPConfiguration,
			AllowHostToNCCommunication: savedReq.AllowHostToNCCommunication,
			AllowNCToHostCommunication: savedReq.AllowNCToHostCommunication,
			SkipDefaultRoutes:          savedReq.SkipDefaultRoutes,
			NetworkInterfaceInfo:       savedReq.NetworkInterfaceInfo,
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
		p := platform.NewExecClient(nil)
		rebootTime, err := p.GetLastRebootTime()
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
					// store nmaNCID as lower case to allow case insensitive comparison with nc stored in CNS
					nmaNCs[strings.TrimPrefix(lowerCaseNCGuid(nc.NetworkContainerID), cns.SwiftPrefix)] = nc.Version
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

func (service *HTTPRestService) validateIPConfigsRequest(ctx context.Context, ipConfigsRequest cns.IPConfigsRequest) (cns.PodInfo, types.ResponseCode, string) {
	if ipConfigsRequest.OrchestratorContext == nil {
		return nil, types.EmptyOrchestratorContext, fmt.Sprintf("OrchestratorContext is not set in the req: %+v", ipConfigsRequest)
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

	podIPInfo.SkipDefaultRoutes = ncStatus.CreateNetworkContainerRequest.SkipDefaultRoutes

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
	podIPInfo.MacAddress = ncStatus.CreateNetworkContainerRequest.NetworkInterfaceInfo.MACAddress
	podIPInfo.NICType = cns.InfraNIC

	return nil
}

// lowerCaseNCGuid() splits incoming NCID by "Swift_" and lowercase NC GUID; i.e,"Swift_ABCD-CD" -> "Swift_abcd-cd"
func lowerCaseNCGuid(ncid string) string {
	ncidHasSwiftPrefix := strings.HasPrefix(ncid, cns.SwiftPrefix)
	if ncidHasSwiftPrefix {
		return cns.SwiftPrefix + strings.ToLower(strings.TrimPrefix(ncid, cns.SwiftPrefix))
	}
	return strings.ToLower(ncid)
}

// isNCWaitingForUpdate :- Determine whether NC version on NMA matches programmed version
// Return error and waitingForUpdate as true only CNS gets response from NMAgent indicating
// the VFP programming is pending
// This returns success / waitingForUpdate as false in all other cases.
// V2 is using the nmagent get nc version list api v2 which doesn't need authentication token
func (service *HTTPRestService) isNCWaitingForUpdate(ncVersion, ncid string, ncVersionList map[string]string) (waitingForUpdate bool, returnCode types.ResponseCode, message string) {
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

	// get the ncVersionList with nc GUID as lower case
	// when looking up if the ncid is present in ncVersionList, convert it to lowercase and then look up
	nmaProgrammedNCVersionStr, ok := ncVersionList[strings.TrimPrefix(lowerCaseNCGuid(ncid), cns.SwiftPrefix)]
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
			IPv6Configuration:          ncDetails.CreateNetworkContainerRequest.IPv6Configuration,
			Routes:                     ncDetails.CreateNetworkContainerRequest.Routes,
			CnetAddressSpace:           ncDetails.CreateNetworkContainerRequest.CnetAddressSpace,
			MultiTenancyInfo:           ncDetails.CreateNetworkContainerRequest.MultiTenancyInfo,
			PrimaryInterfaceIdentifier: ncDetails.CreateNetworkContainerRequest.PrimaryInterfaceIdentifier,
			LocalIPConfiguration:       ncDetails.CreateNetworkContainerRequest.LocalIPConfiguration,
			AllowHostToNCCommunication: ncDetails.CreateNetworkContainerRequest.AllowHostToNCCommunication,
			AllowNCToHostCommunication: ncDetails.CreateNetworkContainerRequest.AllowNCToHostCommunication,
			SkipDefaultRoutes:          ncDetails.CreateNetworkContainerRequest.SkipDefaultRoutes,
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
	err := acn.Encode(w, &response)
	logger.Response(service.Name, response, response.Response.ReturnCode, err)
}

// handlePostNetworkContainers stores all the NCs (from the request that client sent) into CNS's state file
func (service *HTTPRestService) handlePostNetworkContainers(w http.ResponseWriter, r *http.Request) {
	logger.Printf("[Azure CNS] handlePostNetworkContainers")
	var req cns.PostNetworkContainersRequest
	err := acn.Decode(w, r, &req)
	logger.Request(service.Name, &req, err)
	if err != nil {
		response := cns.PostNetworkContainersResponse{
			Response: cns.Response{
				ReturnCode: types.InvalidRequest,
				Message:    fmt.Sprintf("[Azure CNS] handlePostNetworkContainers failed with error: %s", err.Error()),
			},
		}
		err = acn.Encode(w, &response)
		logger.Response(service.Name, response, response.Response.ReturnCode, err)
		return
	}
	if err := req.Validate(); err != nil { //nolint:govet // shadow okay
		logger.Errorf("[Azure CNS] handlePostNetworkContainers failed with error: %s", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	createNCsResp := service.createNetworkContainers(req.CreateNetworkContainerRequests)
	response := cns.PostNetworkContainersResponse{
		Response: createNCsResp,
	}
	err = acn.Encode(w, &response)
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
		saveNcReturnCode, saveNcReturnMessage := service.saveNetworkContainerGoalState(createNcReq, false)
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
	serviceErr := acn.Encode(w, &response)
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
