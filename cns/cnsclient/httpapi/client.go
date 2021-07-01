package httpapi

import (
	"fmt"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
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

// UpdateIPAMPoolMonitor updates IPAM pool monitor.
func (client *Client) UpdateIPAMPoolMonitor(scalar nnc.Scaler, spec nnc.NodeNetworkConfigSpec) error {
	returnCode := client.RestService.UpdateIPAMPoolMonitorInternal(scalar, spec)

	if returnCode != 0 {
		return fmt.Errorf("Failed to update IPAM pool monitor scalar: %+v, spec: %+v, errorCode: %d", scalar, spec, returnCode)
	}

	return nil
}

// ReconcileNCState initializes cns state
func (client *Client) ReconcileNCState(ncRequest *cns.CreateNetworkContainerRequest, podInfoByIP map[string]cns.PodInfo, scalar nnc.Scaler, spec nnc.NodeNetworkConfigSpec) error {
	returnCode := client.RestService.ReconcileNCState(ncRequest, podInfoByIP, scalar, spec)

	if returnCode != 0 {
		return fmt.Errorf("Failed to Reconcile ncState: ncRequest %+v, podInfoMap: %+v, errorCode: %d", *ncRequest, podInfoByIP, returnCode)
	}

	return nil
}

func (client *Client) GetNC(req cns.GetNetworkContainerRequest) (cns.GetNetworkContainerResponse, error) {
	response, returnCode := client.RestService.GetNetworkContainerInternal(req)
	if returnCode != 0 {
		if returnCode == restserver.UnknownContainerID {
			return response, fmt.Errorf("NotFound")
		}
		return response, fmt.Errorf("Failed to get NC, request: %+v, errorCode: %d", req, returnCode)
	}

	return response, nil
}

func (client *Client) DeleteNC(req cns.DeleteNetworkContainerRequest) error {
	returnCode := client.RestService.DeleteNetworkContainerInternal(req)
	if returnCode != 0 {
		return fmt.Errorf("Failed to delete NC, request: %+v, errorCode: %d", req, returnCode)
	}

	return nil
}
