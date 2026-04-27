package restserver

import (
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/hnsclient"
	"github.com/Azure/azure-container-networking/cns/logger"
)

type HNSClient interface {
	DeleteEndpointByID(endpointID string) error
	DeleteNetworkByID(networkID string) error
}

type hnsClientImpl struct{}

func (hnsClientImpl) DeleteEndpointByID(endpointID string) error {
	if err := hnsclient.DeleteHNSEndpointbyID(endpointID); err != nil {
		return fmt.Errorf("delete HNS endpoint %s: %w", endpointID, err)
	}
	return nil
}

func (hnsClientImpl) DeleteNetworkByID(networkID string) error {
	if err := hnsclient.DeleteNetworkByIDHnsV2(networkID); err != nil {
		return fmt.Errorf("delete HNS network %s: %w", networkID, err)
	}
	return nil
}

var defaultHNSClient HNSClient = hnsClientImpl{} //nolint:gochecknoglobals // swapped in tests

func normalizeMAC(mac string) string {
	return strings.ToLower(strings.ReplaceAll(mac, "-", ":"))
}

// Caller must hold the service lock.
func (service *HTTPRestService) findEndpointStateByMAC(mac string) (string, *IPInfo) {
	for containerID, epInfo := range service.EndpointState {
		for _, ipInfo := range epInfo.IfnameToIPMap {
			if (ipInfo.NICType == cns.DelegatedVMNIC || ipInfo.NICType == cns.NodeNetworkInterfaceFrontendNIC) &&
				normalizeMAC(ipInfo.MacAddress) == normalizeMAC(mac) {
				return containerID, ipInfo
			}
		}
	}
	return "", nil
}

// Caller must hold the service lock.
func (service *HTTPRestService) findStaleContainerByApipaIP(ncID, apipaIP string) (containerID, ifName string, info *IPInfo) {
	if apipaIP == "" {
		return "", "", nil
	}

	target := net.ParseIP(apipaIP).To4()
	if target == nil {
		return "", "", nil
	}

	for cID, epInfo := range service.EndpointState {
		for name, ipInfo := range epInfo.IfnameToIPMap {
			if ipInfo.NICType != cns.ApipaNIC {
				continue
			}

			if ipInfo.NetworkContainerID == ncID {
				continue
			}

			for _, ipNet := range ipInfo.IPv4 {
				if ipNet.IP.Equal(target) {
					return cID, name, ipInfo
				}
			}
		}
	}

	return "", "", nil
}

// ncAlreadyExistsForMAC returns true if the NC already exists in NC state with the given MAC.
// Caller must hold the service lock.
func (service *HTTPRestService) ncAlreadyExistsForMAC(ncID, mac string) bool {
	nc, ok := service.state.ContainerStatus[ncID]
	if !ok {
		return false
	}
	return normalizeMAC(nc.CreateNetworkContainerRequest.NetworkInterfaceInfo.MACAddress) == normalizeMAC(mac)
}

// cleanupContainerHNSResources deletes all HNS resources (delegated NIC endpoint+network
// and any APIPA endpoints) for a SwiftV2 container, then removes the container from EndpointState.
// Caller must hold the service lock.
func (service *HTTPRestService) cleanupContainerHNSResources(containerID string) (hnsEndpointID, hnsNetworkID string, _ error) {
	logger.Printf("[cleanupContainerHNSResources] cleaning up all HNS resources for container %s", containerID) //nolint:staticcheck // TODO: migrate to zap logger

	var apipaEndpointID string
	if epInfo, ok := service.EndpointState[containerID]; ok {
		for _, info := range epInfo.IfnameToIPMap {
			switch { //nolint:staticcheck // DelegatedVMNIC and NodeNetworkInterfaceFrontendNIC share the same underlying value
			case info.NICType == cns.ApipaNIC:
				apipaEndpointID = info.HnsEndpointID
			case info.NICType == cns.DelegatedVMNIC || info.NICType == cns.NodeNetworkInterfaceFrontendNIC:
				hnsEndpointID = info.HnsEndpointID
				hnsNetworkID = info.HnsNetworkID
			}
		}
	}

	if apipaEndpointID != "" {
		logger.Printf("[cleanupContainerHNSResources] deleting APIPA HNS endpoint %s in container %s", apipaEndpointID, containerID) //nolint:staticcheck // TODO: migrate to zap logger
		if err := defaultHNSClient.DeleteEndpointByID(apipaEndpointID); err != nil {
			return hnsEndpointID, hnsNetworkID, fmt.Errorf("failed to delete APIPA HNS endpoint %s in container %s: %w", apipaEndpointID, containerID, err)
		}
	}

	if hnsEndpointID != "" {
		logger.Printf("[cleanupContainerHNSResources] deleting HNS endpoint %s in container %s", hnsEndpointID, containerID) //nolint:staticcheck // TODO: migrate to zap logger
		if err := defaultHNSClient.DeleteEndpointByID(hnsEndpointID); err != nil {
			return hnsEndpointID, hnsNetworkID, fmt.Errorf("failed to delete HNS endpoint %s: %w", hnsEndpointID, err)
		}
	}

	if hnsNetworkID != "" {
		logger.Printf("[cleanupContainerHNSResources] deleting HNS network %s in container %s", hnsNetworkID, containerID) //nolint:staticcheck // TODO: migrate to zap logger
		if err := defaultHNSClient.DeleteNetworkByID(hnsNetworkID); err != nil {
			return hnsEndpointID, hnsNetworkID, fmt.Errorf("failed to delete HNS network %s: %w", hnsNetworkID, err)
		}
	}

	if err := service.DeleteEndpointStateHelper(containerID); err != nil {
		return hnsEndpointID, hnsNetworkID, fmt.Errorf("failed to remove container %s from endpoint state file: %w", containerID, err)
	}

	logger.Printf("[cleanupContainerHNSResources] successfully removed all HNS resources and state for container %s", containerID) //nolint:staticcheck // TODO: migrate to zap logger

	return hnsEndpointID, hnsNetworkID, nil
}

// cleanupStaleHNSResources removes HNS endpoints/networks left behind by a previous SwiftV2 NC
// that used the same delegated NIC MAC and was unable to be cleaned up (eg. no CNI DEL was called), then deletes the stale EndpointState entries.
func (service *HTTPRestService) cleanupStaleHNSResources(ncID, mac, apipaIP string) (returnErr error) {
	service.Lock()
	defer service.Unlock()

	// Same NC+MAC already in NC state - this is a repeat CreateNC, not stale.
	if service.ncAlreadyExistsForMAC(ncID, mac) {
		return nil
	}

	var apipaContainerID, apipaEndpointID, containerID, hnsEndpointID, hnsNetworkID string
	defer func() {
		result := "success"
		errMsg := ""
		if returnErr != nil {
			result = "failure"
			errMsg = returnErr.Error()
		}
		logger.SendMetric(aitelemetry.Metric{ //nolint:staticcheck // TODO: migrate to zap logger
			Name:  logger.StaleHNSCleanupMetricStr,
			Value: 1.0,
			CustomDimensions: map[string]string{
				"ncID":             ncID,
				"mac":              mac,
				"containerID":      containerID,
				"hnsEndpointID":    hnsEndpointID,
				"hnsNetworkID":     hnsNetworkID,
				"apipaIP":          apipaIP,
				"apipaContainerID": apipaContainerID,
				"apipaEndpointID":  apipaEndpointID,
				"result":           result,
				"errorMsg":         errMsg,
			},
		})
	}()

	// Clean up the container that has a stale APIPA endpoint colliding with the incoming NC's APIPA IP
	apipaContainerID, _, apipaInfo := service.findStaleContainerByApipaIP(ncID, apipaIP)
	if apipaInfo != nil {
		apipaEndpointID = apipaInfo.HnsEndpointID
		if _, _, err := service.cleanupContainerHNSResources(apipaContainerID); err != nil {
			return err
		}
	}

	// Find the stale container by delegated NIC MAC and clean up all its HNS resources (delegated NIC + any attached APIPA endpoint)
	containerID, _ = service.findEndpointStateByMAC(mac)
	if containerID == "" {
		return nil
	}

	var err error
	hnsEndpointID, hnsNetworkID, err = service.cleanupContainerHNSResources(containerID)
	if err != nil {
		return err
	}

	return nil
}
