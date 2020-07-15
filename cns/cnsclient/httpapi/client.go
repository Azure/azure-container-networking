package httpapi

import (
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
)

// Client implements APIClient interface. Used to update CNS state
type Client struct {
	RestService *restserver.HTTPRestService
}

// CreateOrUpdateNC updates cns state
func (client *Client) CreateOrUpdateNC(ncRequest *cns.CreateNetworkContainerRequest) error {
	// var (
	// 	ipConfigsToAdd []*cns.ContainerIPConfigState
	// )

	// //Lock to read ipconfigs
	// client.RestService.Lock()

	// //Only add ipconfigs that don't exist in cns state already
	// for _, ipConfig := range ipConfigs {
	// 	if _, ok := client.RestService.PodIPConfigState[ipConfig.ID]; !ok {
	// 		ipConfig.State = cns.Available
	// 		ipConfigsToAdd = append(ipConfigsToAdd, ipConfig)
	// 	}
	// }

	// client.RestService.Unlock()
	// leave empty
	return nil //client.RestService.AddIPConfigsToState(ipConfigsToAdd)
}

// InitCNSState initializes cns state
func (client *Client) InitCNSState(ncRequest *cns.CreateNetworkContainerRequest, podInfoByIP map[string]*cns.KubernetesPodInfo) error {
	// client.RestService.Lock()
	// client.RestService.ReadyToIPAM = true
	// client.RestService.Unlock()

	// return client.RestService.AddIPConfigsToState(ipConfigs)
	return nil
}
