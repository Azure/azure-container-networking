package singletenantcontroller

import (
	"context"

	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

// RequestController interface for cns to interact with the request controller
type RequestController interface {
	Init(context.Context) error
	Start(context.Context) error
	UpdateCRDSpec(context.Context, nnc.NodeNetworkConfigSpec) error
	IsStarted() bool
}
