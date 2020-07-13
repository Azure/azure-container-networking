package cnsclient

import "github.com/Azure/azure-container-networking/cns"

// APIClient interface to update cns state
type APIClient interface {
	UpdateCNSState([]*cns.ContainerIPConfigState) error
	InitCNSState([]*cns.ContainerIPConfigState) error
	ReadyToIPAM() bool
}
