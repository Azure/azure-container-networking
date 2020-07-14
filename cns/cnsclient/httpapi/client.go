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
func (client *Client) UpdateCNSState(ipConfigs []*cns.ContainerIPConfigState) error {
	var (
		ipConfigsToAdd []*cns.ContainerIPConfigState
	)

	//Lock to read ipconfigs
	client.RestService.Lock()

	//Only add ipconfigs that don't exist in cns state already
	for _, ipConfig := range ipConfigs {
		if _, ok := client.RestService.PodIPConfigState[ipConfig.ID]; !ok {
			ipConfig.State = cns.Available
			ipConfigsToAdd = append(ipConfigsToAdd, ipConfig)
		}
	}

	client.RestService.Unlock()

	return client.RestService.AddIPConfigsToState(ipConfigsToAdd)
}

// InitCNSState initializes cns state
func (client *Client) InitCNSState(ipConfigs []*cns.ContainerIPConfigState) error {
	client.RestService.Lock()
	client.RestService.ReadyToIPAM = true
	client.RestService.Unlock()
	return client.RestService.AddIPConfigsToState(ipConfigs)
}

// ReadyToIPAM tells the caller if CNS is act as an IPAM for CNI
func (client *Client) ReadyToIPAM() bool {
	return client.RestService.ReadyToIPAM
}
