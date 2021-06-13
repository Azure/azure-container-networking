package cnsclient

import (
	"github.com/Azure/azure-container-networking/cns"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

// APIClient interface to update cns state
type APIClient interface {
	ReconcileNCState(nc *cns.CreateNetworkContainerRequest, pods map[string]cns.PodInfo, scalar nnc.Scaler, spec nnc.NodeNetworkConfigSpec) error
	CreateOrUpdateNC(nc cns.CreateNetworkContainerRequest) error
	UpdateIPAMPoolMonitor(scalar nnc.Scaler, spec nnc.NodeNetworkConfigSpec) error
	GetNC(nc cns.GetNetworkContainerRequest) (cns.GetNetworkContainerResponse, error)
	DeleteNC(nc cns.DeleteNetworkContainerRequest) error
}
