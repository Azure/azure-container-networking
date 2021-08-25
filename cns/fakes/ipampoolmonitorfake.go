package fakes

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
)

type IPAMPoolMonitorFake struct {
	FakeMinimumIps       int
	FakeMaximumIps       int
	FakeIpsNotInUseCount int
	FakecachedNNC        v1alpha.NodeNetworkConfig
}

func (ipm *IPAMPoolMonitorFake) Start(context.Context, int) error {
	return nil
}

func (ipm *IPAMPoolMonitorFake) Update(v1alpha.NodeNetworkConfig) {}

func (ipm *IPAMPoolMonitorFake) Reconcile() error {
	return nil
}

func (ipm *IPAMPoolMonitorFake) GetStateSnapshot() cns.IpamPoolMonitorStateSnapshot {
	return cns.IpamPoolMonitorStateSnapshot{
		MinimumFreeIps:           int64(ipm.FakeMinimumIps),
		MaximumFreeIps:           int64(ipm.FakeMaximumIps),
		UpdatingIpsNotInUseCount: ipm.FakeIpsNotInUseCount,
		CachedNNC:                ipm.FakecachedNNC,
	}
}
