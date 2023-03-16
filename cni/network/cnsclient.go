package network

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
)

type cnsclient interface {
	RequestIPAddress(ctx context.Context, ipconfig cns.IPConfigRequest) (*cns.IPConfigResponse, error)
	ReleaseIPAddress(ctx context.Context, ipconfig cns.IPConfigRequest) error
	RequestIPs(ctx context.Context, ipconfig cns.IPConfigsRequest) (*cns.IPConfigsResponse, error)
	ReleaseIPs(ctx context.Context, ipconfig cns.IPConfigsRequest) error
	GetNetworkContainer(ctx context.Context, orchestratorContext []byte) (*cns.GetNetworkContainerResponse, error)
	GetAllNetworkContainers(ctx context.Context, orchestratorContext []byte) ([]cns.GetNetworkContainerResponse, error)
}
