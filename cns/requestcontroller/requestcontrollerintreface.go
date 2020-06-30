package requestcontroller

import (
	"context"

	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

// interface for cns to interact with the request controller
type RequestController interface {
	StartRequestController(exitChan chan bool) error
	UpdateCRDSpec(cntxt context.Context, crdSpec nnc.NodeNetworkConfigSpec) error
	//pass in cns client
}

// interface for request controller to interact with cns
//CNSClient
type CNSInteractor interface {
	UpdateCNSState(nnc.NodeNetworkConfigStatus) error
	//pass in cns type
}
