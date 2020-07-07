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

// PopulateIP tells CNS of a current pod on the node and that pod's ip
func (clien *Client) PopulateIP(podInfo *cns.KubernetesPodInfo, IPConfig *cns.IPSubnet) error {
	//Mat will pick up from here
	return nil
}
