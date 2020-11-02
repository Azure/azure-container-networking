package fakes

import (
	"context"

	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

type IPAMPoolMonitorFake struct{}

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
