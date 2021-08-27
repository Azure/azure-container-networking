package fakes

import "github.com/Azure/azure-container-networking/crd/nodenetworkconfig"

type fakeNNCClient struct {
	nodenetworkconfig.Client
}

func NewNNCClientFake() nodenetworkconfig.Client {
	return &fakeNNCClient{}
}
