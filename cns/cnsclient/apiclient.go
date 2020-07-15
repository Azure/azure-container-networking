package cnsclient

import "github.com/Azure/azure-container-networking/cns"

// APIClient interface to update cns state
type APIClient interface {
<<<<<<< HEAD
	UpdateCNSState([]*cns.ContainerIPConfigState) error
	InitCNSState([]*cns.ContainerIPConfigState) error
	ReadyToIPAM() bool
=======
	CreateOrUpdateNC(*cns.CreateNetworkContainerRequest) error
	InitCNSState(*cns.CreateNetworkContainerRequest, map[string]*cns.KubernetesPodInfo) error
>>>>>>> api-template
}
