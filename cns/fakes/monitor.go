//go:build !ignore_uncovered
// +build !ignore_uncovered

package fakes

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
		MaximumFreeIps:           int(float64(f.NodeNetworkConfig.Status.Scaler.BatchSize) * (float64(f.NodeNetworkConfig.Status.Scaler.ReleaseThresholdPercent) / 100)), //nolint:gomnd // it's a percent
		MinimumFreeIps:           int(float64(f.NodeNetworkConfig.Status.Scaler.BatchSize) * (float64(f.NodeNetworkConfig.Status.Scaler.RequestThresholdPercent) / 100)), //nolint:gomnd // it's a percent
		UpdatingIpsNotInUseCount: f.IPsNotInUseCount,
		CachedNNC:                *f.NodeNetworkConfig,
	}
}
