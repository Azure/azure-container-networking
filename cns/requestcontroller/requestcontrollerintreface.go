package requestcontroller

import (
	"context"

	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

// interface for cns to interact with the request controller
type RequestController interface {
	StartRequestController(exitChan chan bool) error
	ReleaseIPsByUUIDs(cntxt context.Context, listOfIPUUIDS []string, newRequestedIPCount int) error
}

// interface for request controller to interact with cns
type CNSInteractor interface {
	UpdateCNSState(nnc.NodeNetworkConfigStatus) error
}
