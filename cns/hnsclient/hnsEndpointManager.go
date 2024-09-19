package hnsclient

import (
	"context"
	"net"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/pkg/errors"
)

type releaseIPsClient interface {
	ReleaseIPs(ctx context.Context, ipconfig cns.IPConfigsRequest) error
	GetHNSEndpointID(ctx context.Context, endpointID string) ([]string, [][]net.IPNet, error)
}
type EndpointManager struct {
	cli releaseIPsClient
}

func NewEndpointManager(cli releaseIPsClient) *EndpointManager {
	return &EndpointManager{cli: cli}
}

func (em *EndpointManager) ReleaseIPs(ctx context.Context, ipconfig cns.IPConfigsRequest) error {
	logger.Printf("deleting HNS Endpoint asynchronously")
	// remove HNS endpoint
	if err := em.deleteEndpoint(ctx, ipconfig.InfraContainerID); err != nil {
		logger.Errorf("failed to remove HNS endpoint %s", err.Error())
	}
	em.cli.ReleaseIPs(ctx, ipconfig)
	return nil
}

// call GetEndpoint API to get the state and then remove assiciated HNS
func (em *EndpointManager) deleteEndpoint(ctx context.Context, containerid string) error {
	hnsEndpointIDs, ips, err := em.cli.GetHNSEndpointID(ctx, containerid)
	if err != nil {
		return errors.Wrap(err, "failed to read the endpoint from CNS state")
	}
	for _, hnsEndpointID := range hnsEndpointIDs {
		logger.Printf("deleting HNS Endpoint with id %v", hnsEndpointID)
		if err := DeleteHNSEndpointbyID(hnsEndpointID); err != nil {
			logger.Errorf("failed to remove HNS endpoint %s %s", hnsEndpointID, err.Error())
		}
	}
	for _, ip := range ips {
		// we need to get the HNSENdpoint via the IP address if the HNSEndpointID is not present in the statefile
		var hnsEndpointID string
		if hnsEndpointID, err = GetHNSEndpointbyIP(ip, ip); err != nil {
			return errors.Wrap(err, "failed to find HNS endpoint with id")
		}
		logger.Printf("deleting HNS Endpoint with id %v", hnsEndpointID)
		if err := DeleteHNSEndpointbyID(hnsEndpointID); err != nil {
			logger.Errorf("failed to remove HNS endpoint %s %s", hnsEndpointID, err.Error())
		}
	}
	return err
}
