package cnsclient

import (
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
)

// APIClient interface to update cns state
type APIClient interface {
	ReconcileNCState(nc *cns.CreateNetworkContainerRequest, pods map[string]cns.PodInfo, nnc v1alpha.NodeNetworkConfig) error
	CreateOrUpdateNC(nc cns.CreateNetworkContainerRequest) error
	GetNC(nc cns.GetNetworkContainerRequest) (cns.GetNetworkContainerResponse, error)
	DeleteNC(nc cns.DeleteNetworkContainerRequest) error
}
