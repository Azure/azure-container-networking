package endpointmanager

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
)

type EndpointManager struct {
	cli releaseIPsClient // nolint
}

type releaseIPsClient interface {
	ReleaseIPs(ctx context.Context, ipconfig cns.IPConfigsRequest) error
	GetEndpoint(ctx context.Context, endpointID string) (*restserver.GetEndpointResponse, error)
}

func WithPlatformReleaseIPsManager(cli releaseIPsClient) *EndpointManager {
	return &EndpointManager{cli: cli}
}
