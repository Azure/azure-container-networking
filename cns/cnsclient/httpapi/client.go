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
func (client *Client) UpdateCNSState(createNetworkContainerRequest *cns.CreateNetworkContainerRequest, containerIPConfigs []*cns.ContainerIPConfigState) error {
	//TODO: update cns state
	return nil
}

func (client *Client) InitCNSState(*cns.CreateNetworkContainerRequest, []*cns.ContainerIPConfigState) error {
	client.RestService.ReadyToIPAM = true
	//TODO: setup cns state
	return nil
}

// ReadyToIPAM tells the caller if CNS is act as an IPAM for CNI
func (client *Client) ReadyToIPAM() bool {
	return client.RestService.ReadyToIPAM
}
