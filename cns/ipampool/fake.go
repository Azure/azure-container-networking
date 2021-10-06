//go:build !ignore_uncovered
// +build !ignore_uncovered

package ipampool

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
)

type Fake struct {
	IPsNotInUseCount  int
	NodeNetworkConfig *v1alpha.NodeNetworkConfig
}

func (*Fake) Start(ctx context.Context, poolMonitorRefreshMilliseconds int) error {
	return nil
}

func (f *Fake) Update(nnc *v1alpha.NodeNetworkConfig) {
	f.NodeNetworkConfig = nnc
}

func (*Fake) Reconcile() error {
	return nil
}

func (f *Fake) GetStateSnapshot() cns.IpamPoolMonitorStateSnapshot {
	return cns.IpamPoolMonitorStateSnapshot{
		MaximumFreeIps:           CalculateMaxFreeIPs(*f.NodeNetworkConfig),
		MinimumFreeIps:           CalculateMinFreeIPs(*f.NodeNetworkConfig),
		UpdatingIpsNotInUseCount: f.IPsNotInUseCount,
		CachedNNC:                *f.NodeNetworkConfig,
	}
}
