package endpointmanager

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/hnsclient"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/pkg/errors"
)

// ReleaseIPs implements an Interface in fsnotify for async delete of the HNS endpoint and IP addresses
func (em *EndpointManager) ReleaseIPs(ctx context.Context, ipconfigreq cns.IPConfigsRequest) error {
	logger.Printf("deleting HNS Endpoint asynchronously")
	// remove HNS endpoint
	if err := em.deleteEndpoint(ctx, ipconfigreq.InfraContainerID); err != nil {
		logger.Errorf("failed to remove HNS endpoint %s", err.Error())
	}
	return errors.Wrap(em.cli.ReleaseIPs(ctx, ipconfigreq), "failed to release IP from CNS")
}

// deleteEndpoint API to get the state and then remove assiciated HNS
func (em *EndpointManager) deleteEndpoint(ctx context.Context, containerid string) error {
	endpointResponse, err := em.cli.GetEndpoint(ctx, containerid)
	if err != nil {
		return errors.Wrap(err, "failed to read the endpoint from CNS state")
	}
	for _, ipInfo := range endpointResponse.EndpointInfo.IfnameToIPMap {
		hnsEndpointID := ipInfo.HnsEndpointID
		// we need to get the HNSENdpoint via the IP address if the HNSEndpointID is not present in the statefile
		if ipInfo.HnsEndpointID == "" {
			if hnsEndpointID, err = hnsclient.GetHNSEndpointbyIP(ipInfo.IPv4, ipInfo.IPv6); err != nil {
				return errors.Wrap(err, "failed to find HNS endpoint with id")
			}
		}
		logger.Printf("deleting HNS Endpoint with id %v", hnsEndpointID)
		if err := hnsclient.DeleteHNSEndpointbyID(hnsEndpointID); err != nil {
			return errors.Wrap(err, "failed to delete HNS endpoint with id "+ipInfo.HnsEndpointID)
		}
	}
	return nil
}
