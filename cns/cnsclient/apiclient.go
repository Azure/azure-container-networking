package cnsclient

import "github.com/Azure/azure-container-networking/cns"

// APIClient interface to update cns state
type APIClient interface {
	InitCNSState(*cns.CreateNetworkContainerRequest, map[string]*cns.KubernetesPodInfo) error
	CreateOrUpdateNC(cns.CreateNetworkContainerRequest) error
}
