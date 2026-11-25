package restserver

import "github.com/Azure/azure-container-networking/cns"

func (service *HTTPRestService) cleanupStaleHNSResources(_ cns.CreateNetworkContainerRequest) error {
	return nil
}
