package endpointmanager

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
)

// ReleaseIPs implements an Interface in fsnotify for async delete of the HNS endpoint and IP addresses
func (em *EndpointManager) ReleaseIPs(_ context.Context, _ cns.IPConfigsRequest) error {
	return nil
}
