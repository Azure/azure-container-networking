package fakes

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"

	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

type IPAMPoolMonitorFake struct {
	FakeMinimumIps       int
	FakeMaximumIps       int
	FakeIpsNotInUseCount int
	FakecachedNNC        nnc.NodeNetworkConfig
}

func NewIPAMPoolMonitorFake() *IPAMPoolMonitorFake {
	return &IPAMPoolMonitorFake{}
}

func (ipm *IPAMPoolMonitorFake) Start(ctx context.Context, poolMonitorRefreshMilliseconds int) error {
	return nil
}

func (ipm *IPAMPoolMonitorFake) Update(scalar nnc.Scaler, spec nnc.NodeNetworkConfigSpec) error {
	return nil
}

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
