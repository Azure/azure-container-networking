package kubecontroller

import (
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
)

type cnsclient interface {
	ReconcileNCState(ncRequest *cns.CreateNetworkContainerRequest, podInfoByIP map[string]cns.PodInfo, scalar v1alpha.Scaler, spec v1alpha.NodeNetworkConfigSpec) error
	CreateOrUpdateNC(ncRequest cns.CreateNetworkContainerRequest) error
	UpdateIPAMPoolMonitor(scalar v1alpha.Scaler, spec v1alpha.NodeNetworkConfigSpec)
}
