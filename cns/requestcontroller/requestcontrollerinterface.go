package requestcontroller

import (
	"context"

	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

// RequestController interface for cns to interact with the request controller
type RequestController interface {
	InitRequestController(context.Context) error
	StartRequestController(context.Context) error
	UpdateCRDSpec(context.Context, nnc.NodeNetworkConfigSpec) error
	IsStarted() bool
}
