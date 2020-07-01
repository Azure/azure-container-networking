package kubecontroller

import (
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
)

// CNSInteractor implements the CNSClient interface.
type CNSInteractor struct {
	RestService *restserver.HTTPRestService
}

// UpdateCNSState updates cns state
func (interactor *CNSInteractor) UpdateCNSState(createNetworkContainerRequest *cns.CreateNetworkContainerRequest) error {
	//Mat will pick up from here
	return nil
}
