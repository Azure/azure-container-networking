package restserver

import (
	"fmt"
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

// ncAlreadyExistsForMAC returns true if the NC already exists in NC state with the given MAC.
// Caller must hold the service lock.
func (service *HTTPRestService) ncAlreadyExistsForMAC(ncID, mac string) bool {
	nc, ok := service.state.ContainerStatus[ncID]
	if !ok {
		return false
	}
	return normalizeMAC(nc.CreateNetworkContainerRequest.NetworkInterfaceInfo.MACAddress) == normalizeMAC(mac)
}

// cleanupStaleHNSResources removes HNS endpoints/networks left behind by a previous NC
// that used the same delegated NIC MAC and was unable to be cleaned up (eg. no CNI DEL was called), then deletes the stale EndpointState entries.
func (service *HTTPRestService) cleanupStaleHNSResources(ncID, mac string) (returnErr error) {
	service.Lock()
	defer service.Unlock()

	// Same NC+MAC already in NC state - this is a repeat CreateNC, not stale.
	if service.ncAlreadyExistsForMAC(ncID, mac) {
		return nil
	}

	containerID, ipInfo := service.findEndpointStateByMAC(mac)
	if ipInfo == nil {
		return nil
	}

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
				"ncID":          ncID,
				"mac":           mac,
				"containerID":   containerID,
				"hnsEndpointID": ipInfo.HnsEndpointID,
				"hnsNetworkID":  ipInfo.HnsNetworkID,
				"result":        result,
				"errorMsg":      errMsg,
			},
		})
	}()

	logger.Printf("[cleanupStaleHNSResources] cleaning up stale HNS resources for container %s (MAC %s)", containerID, mac) //nolint:staticcheck // TODO: migrate to zap logger

	if ipInfo.HnsEndpointID != "" {
		if err := defaultHNSClient.DeleteEndpointByID(ipInfo.HnsEndpointID); err != nil {
			return fmt.Errorf("failed to delete stale HNS endpoint %s: %w", ipInfo.HnsEndpointID, err)
		}
	}

	if ipInfo.HnsNetworkID != "" {
		if err := defaultHNSClient.DeleteNetworkByID(ipInfo.HnsNetworkID); err != nil {
			return fmt.Errorf("failed to delete stale HNS network %s: %w", ipInfo.HnsNetworkID, err)
		}
	}

	if err := service.DeleteEndpointStateHelper(containerID); err != nil {
		return fmt.Errorf("failed to remove stale endpoint state for container %s: %w", containerID, err)
	}

	logger.Printf("[cleanupStaleHNSResources] Successfully removed stale endpoint state and HNS resources for container %s", containerID) //nolint:staticcheck // TODO: migrate to zap logger

	return nil
}
