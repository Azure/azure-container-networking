package httpapi

import (
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
)

// Client implements APIClient interface. Used to update CNS state
type Client struct {
	RestService *restserver.HTTPRestService
}

// UpdateCNSState updates cns state
func (client *Client) UpdateCNSState(createNetworkContainerRequest *cns.CreateNetworkContainerRequest) error {
	//Mat will pick up from here
	return nil
}
