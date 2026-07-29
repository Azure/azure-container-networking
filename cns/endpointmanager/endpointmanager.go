package endpointmanager

import (
	"context"
	"net"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
)

type EndpointManager struct {
	cli releaseIPsClient // nolint
	hns hnsEndpointClient
}

type releaseIPsClient interface {
	ReleaseIPs(ctx context.Context, ipconfig cns.IPConfigsRequest) error
	GetEndpoint(ctx context.Context, endpointID string) (*restserver.GetEndpointResponse, error)
}

// hnsEndpointClient is the subset of HNS operations used by the Windows
// endpoint manager. It is defined consumer-side (per acn-go-interfaces-dependencies)
// so tests can inject fakes without depending on any external package.
type hnsEndpointClient interface {
	GetHNSEndpointbyIP(ipv4, ipv6 []net.IPNet) (string, error)
	DeleteHNSEndpointbyID(hnsEndpointID string) error
	DeleteNetworkByIDHnsV2(networkID string) error
}

// Option configures an EndpointManager.
type Option func(*EndpointManager)

// WithHNSClient overrides the HNS client used by the endpoint manager.
// This is primarily intended for tests; the default is platform-appropriate.
func WithHNSClient(hns hnsEndpointClient) Option {
	return func(em *EndpointManager) { em.hns = hns }
}

func WithPlatformReleaseIPsManager(cli releaseIPsClient, opts ...Option) *EndpointManager {
	em := &EndpointManager{cli: cli, hns: defaultHNSClient()}
	for _, opt := range opts {
		opt(em)
	}
	return em
}
