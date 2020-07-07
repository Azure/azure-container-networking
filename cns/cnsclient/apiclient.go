package cnsclient

import "github.com/Azure/azure-container-networking/cns"

// APIClient interface to update cns state
type APIClient interface {
	UpdateCNSState(createNetworkContainerRequest *cns.CreateNetworkContainerRequest) error
	PopulateIP(podInfo *cns.KubernetesPodInfo, IPConfig *cns.IPSubnet) error
}
