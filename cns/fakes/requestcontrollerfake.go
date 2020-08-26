package fakes

import (
	"context"

	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

type RequestControllerFake struct {
}

func NewRequestControllerFake() *RequestControllerFake {
	return &RequestControllerFake{}
}

func (rc RequestControllerFake) StartRequestController(exitChan <-chan struct{}) error {
	return nil
}

func (rc RequestControllerFake) UpdateCRDSpec(cntxt context.Context, crdSpec nnc.NodeNetworkConfigSpec) error {
	return nil
}
