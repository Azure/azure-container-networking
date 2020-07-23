package httpapi

import (
	"fmt"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
)

// Client implements APIClient interface. Used to update CNS state
type Client struct {
	RestService *restserver.HTTPRestService
}

// CreateOrUpdateNC updates cns state
func (client *Client) CreateOrUpdateNC(ncRequest *cns.CreateNetworkContainerRequest) error {
	returnCode := client.RestService.CreateOrUpdateNetworkContainerInternal(*ncRequest)

	if returnCode != 0 {
		return fmt.Errorf("Failed to Create NC request: %+v, errorCode: %d", *ncRequest, returnCode)
	}

	return nil
}

// InitCNSState initializes cns state
func (client *Client) InitCNSState(ncRequest *cns.CreateNetworkContainerRequest, podInfoByIP map[string]*cns.KubernetesPodInfo) error {
	// client.RestService.Lock()
	// client.RestService.ReadyToIPAM = true
	// client.RestService.Unlock()

	// return client.RestService.AddIPConfigsToState(ipConfigs)
	return nil
}
