package network

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
)

type cnsclient interface {
	RequestIPAddress(ctx context.Context, ipconfig cns.IPConfigRequest) (*cns.IPConfigResponse, error)
	ReleaseIPAddress(ctx context.Context, ipconfig cns.IPConfigRequest) error
	GetNetworkConfiguration(ctx context.Context, orchestratorContext []byte) (*cns.GetNetworkContainerResponse, error)
}
