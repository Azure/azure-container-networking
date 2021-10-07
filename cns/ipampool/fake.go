package ipampool

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
)

type MonitorFake struct {
	IPsNotInUseCount  int
	NodeNetworkConfig *v1alpha.NodeNetworkConfig
}

func (*MonitorFake) Start(ctx context.Context) error {
	return nil
}

func (f *MonitorFake) Update(nnc *v1alpha.NodeNetworkConfig) {
	f.NodeNetworkConfig = nnc
}

func (*MonitorFake) Reconcile() error {
	return nil
}

func (f *MonitorFake) GetStateSnapshot() cns.IpamPoolMonitorStateSnapshot {
	return cns.IpamPoolMonitorStateSnapshot{
		MaximumFreeIps:           CalculateMaxFreeIPs(*f.NodeNetworkConfig),
		MinimumFreeIps:           CalculateMinFreeIPs(*f.NodeNetworkConfig),
		UpdatingIpsNotInUseCount: f.IPsNotInUseCount,
		CachedNNC:                *f.NodeNetworkConfig,
	}
}
