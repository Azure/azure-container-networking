package fakes

import (
	"context"

	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
)

var _ nodenetworkconfig.Client = (*fakeclient)(nil)

type fakeclient struct {
	nnc *v1alpha.NodeNetworkConfig
}

func NewFakeNNCClient(nnc *v1alpha.NodeNetworkConfig) nodenetworkconfig.Client {
	return &fakeclient{
		nnc: nnc,
	}
}

func (fc *fakeclient) UpdateSpec(_ context.Context, spec *v1alpha.NodeNetworkConfigSpec) (*v1alpha.NodeNetworkConfig, error) {
	fc.nnc.Spec = *spec
	return fc.nnc, nil
}

func (fc *fakeclient) Get(context.Context) (*v1alpha.NodeNetworkConfig, error) {
	return nil, nil
}
