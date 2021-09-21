package multitenantoperator

import "github.com/Azure/azure-container-networking/cns"

type cnsclient interface {
	DeleteNC(req cns.DeleteNetworkContainerRequest) error
	GetNC(req cns.GetNetworkContainerRequest) (cns.GetNetworkContainerResponse, error)
	CreateOrUpdateNC(ncRequest cns.CreateNetworkContainerRequest) error
}
