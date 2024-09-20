package endpointmanager

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
)

// ReleaseIPs implements an Interface in fsnotify for async delete of the HNS endpoint and IP addresses
func (em *EndpointManager) ReleaseIPs(ctx context.Context, req cns.IPConfigsRequest) error {
	return em.cli.ReleaseIPs(ctx, req)
}
