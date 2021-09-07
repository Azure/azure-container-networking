package kubecontroller

import (
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
)

type cnsclient interface {
	ReconcileNCState(*cns.CreateNetworkContainerRequest, map[string]cns.PodInfo, v1alpha.Scaler, v1alpha.NodeNetworkConfigSpec) types.ResponseCode
	CreateOrUpdateNetworkContainerInternal(*cns.CreateNetworkContainerRequest) types.ResponseCode
	UpdateIPAMPoolMonitor(scalar v1alpha.Scaler, spec v1alpha.NodeNetworkConfigSpec)
}
