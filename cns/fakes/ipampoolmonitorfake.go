//go:build !ignore_uncovered
// +build !ignore_uncovered

package fakes

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
)

type IPAMPoolMonitorFake struct {
	FakeIpsNotInUseCount int
	FakecachedNNC        *v1alpha.NodeNetworkConfig
}

func (ipm *IPAMPoolMonitorFake) Start(ctx context.Context, poolMonitorRefreshMilliseconds int) error {
	return nil
}

func (ipm *IPAMPoolMonitorFake) Update(nnc *v1alpha.NodeNetworkConfig) {
	ipm.FakecachedNNC = nnc
}

func (ipm *IPAMPoolMonitorFake) Reconcile() error {
	return nil
}

func (ipm *IPAMPoolMonitorFake) GetStateSnapshot() cns.IpamPoolMonitorStateSnapshot {
	return cns.IpamPoolMonitorStateSnapshot{
		UpdatingIpsNotInUseCount: ipm.FakeIpsNotInUseCount,
		CachedNNC:                *ipm.FakecachedNNC,
	}
}
