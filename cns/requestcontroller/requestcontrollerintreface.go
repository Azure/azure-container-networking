package requestcontroller

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

// RequestController interface for cns to interact with the request controller
type RequestController interface {
	StartRequestController(exitChan chan bool) error
	UpdateCRDSpec(cntxt context.Context, crdSpec *nnc.NodeNetworkConfigSpec) error
}

// CNSClient interface for request controller to interact with cns
type CNSClient interface {
	UpdateCNSState(createNetworkContainerRequest *cns.CreateNetworkContainerRequest) error
}
