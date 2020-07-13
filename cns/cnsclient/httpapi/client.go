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
func (client *Client) UpdateCNSState(containerIPConfigs []*cns.ContainerIPConfigState) error {
	var (
		ipConfigsToAdd []*cns.ContainerIPConfigState
	)

	//Only add ipconfigs that don't exist in cns state already
	for _, ipConfig := range containerIPConfigs {
		if _, ok := client.RestService.PodIPConfigState[ipConfig.ID]; !ok {
			ipConfigsToAdd = append(ipConfigsToAdd, ipConfig)
		}
	}

	return client.RestService.AddIPConfigsToState(ipConfigsToAdd)
}

// InitCNSState initializes cns state
func (client *Client) InitCNSState(ipConfigs []*cns.ContainerIPConfigState) error {
	client.RestService.ReadyToIPAM = true
	return client.RestService.AddIPConfigsToState(ipConfigs)
}

// ReadyToIPAM tells the caller if CNS is act as an IPAM for CNI
func (client *Client) ReadyToIPAM() bool {
	return client.RestService.ReadyToIPAM
}
