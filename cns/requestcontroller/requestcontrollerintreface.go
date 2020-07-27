package requestcontroller

import (
	"context"

	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

// RequestController interface for cns to interact with the request controller
type RequestController interface {
	StartRequestController(exitChan <-chan struct{}) error
	UpdateCRDSpec(cntxt context.Context, crdSpec nnc.NodeNetworkConfigSpec) error
}
