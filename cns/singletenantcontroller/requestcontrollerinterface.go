package singletenantcontroller

import (
	"context"

	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
)

// RequestController interface for cns to interact with the request controller
type RequestController interface {
	Init(context.Context) error
	Start(context.Context) error
	UpdateCRDSpec(context.Context, v1alpha.NodeNetworkConfigSpec) error
	IsStarted() bool
}
