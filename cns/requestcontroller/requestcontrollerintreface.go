package requestcontroller

import (
	"context"

	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

// interface for cns to interact with the request controller
type RequestController interface {
	StartRequestController(exitChan chan bool) error
	UpdateCRDSpec(cntxt context.Context, crdSpec nnc.NodeNetworkConfigSpec) error
}

// interface for request controller to interact with cns
//CNSClient
type CNSClient interface {
	UpdateCNSState() error
	//pass in cns type
}
