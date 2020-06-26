package kubernetes

import (
	"github.com/Azure/azure-container-networking/cns/restserver"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

//This K8sCNSInteractor implements the CNSInteractor interface. It's used by nodenetworkconfigreconciler to translate
// CRD status to HttpRestService changes
type K8sCNSInteractor struct {
	RestService *restserver.HTTPRestService
}

func (interactor *K8sCNSInteractor) UpdateCNSState(status nnc.NodeNetworkConfigStatus) error {
	//TODO: translate CNS Status into CNS Rest Service changes.
	//Mat will pick up from here
	return nil
}
