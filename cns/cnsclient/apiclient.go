package cnsclient

import (
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
)

// APIClient interface to update cns state
type APIClient interface {
	ReconcileNCState(nc *cns.CreateNetworkContainerRequest, pods map[string]cns.PodInfo, scalar v1alpha.Scaler, spec v1alpha.NodeNetworkConfigSpec) error
	CreateOrUpdateNC(nc cns.CreateNetworkContainerRequest) error
	UpdateIPAMPoolMonitor(scalar v1alpha.Scaler, spec v1alpha.NodeNetworkConfigSpec)
	GetNC(nc cns.GetNetworkContainerRequest) (cns.GetNetworkContainerResponse, error)
	DeleteNC(nc cns.DeleteNetworkContainerRequest) error
}
