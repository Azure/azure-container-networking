package endpointmanager

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/pkg/errors"
)

// ReleaseIPs implements an Interface in fsnotify for async delete of the HNS endpoint and IP addresses
func (em *EndpointManager) ReleaseIPs(ctx context.Context, ipconfigreq cns.IPConfigsRequest) error {
	return errors.Wrap(em.cli.ReleaseIPs(ctx, ipconfigreq), "failed to release IP from CNS")
}
