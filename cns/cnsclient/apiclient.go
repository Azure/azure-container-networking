package cnsclient

import "github.com/Azure/azure-container-networking/cns"

// APIClient interface to update cns state
type APIClient interface {
	ReconcileNCState(nc *cns.CreateNetworkContainerRequest, pods map[string]cns.KubernetesPodInfo, batchSize int64, requestThreshold, releaseThreshold float64) error
	CreateOrUpdateNC(cns.CreateNetworkContainerRequest) error
}
