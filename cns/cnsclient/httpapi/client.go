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
func (client *Client) CreateOrUpdateNC(ncRequest cns.CreateNetworkContainerRequest) error {
	returnCode := client.RestService.CreateOrUpdateNetworkContainerInternal(ncRequest)

	if returnCode != 0 {
		return fmt.Errorf("Failed to Create NC request: %+v, errorCode: %d", ncRequest, returnCode)
	}

	return nil
}

// ReconcileNCState initializes cns state
func (client *Client) ReconcileNCState(ncRequest *cns.CreateNetworkContainerRequest, podInfoByIP map[string]cns.KubernetesPodInfo, batchSize int64, requestThreshold, releaseThreshold float64) error {
	returnCode := client.RestService.ReconcileNCState(ncRequest, podInfoByIP, batchSize, requestThreshold, releaseThreshold)

	if returnCode != 0 {
		return fmt.Errorf("Failed to Reconcile ncState: ncRequest %+v, podInfoMap: %+v, errorCode: %d", *ncRequest, podInfoByIP, returnCode)
	}

	return nil
}
