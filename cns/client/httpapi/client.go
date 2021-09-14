package httpapi

import (
	"fmt"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
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
func (client *Client) UpdateIPAMPoolMonitor(scalar v1alpha.Scaler, spec v1alpha.NodeNetworkConfigSpec) {
	client.RestService.IPAMPoolMonitor.Update(scalar, spec)
}

// ReconcileNCState initializes cns state
func (client *Client) ReconcileNCState(ncRequest *cns.CreateNetworkContainerRequest, podInfoByIP map[string]cns.PodInfo, scalar v1alpha.Scaler, spec v1alpha.NodeNetworkConfigSpec) error {
	returnCode := client.RestService.ReconcileNCState(ncRequest, podInfoByIP, scalar, spec)

	if returnCode != 0 {
		return fmt.Errorf("Failed to Reconcile ncState: ncRequest %+v, podInfoMap: %+v, errorCode: %d", *ncRequest, podInfoByIP, returnCode)
	}

	return nil
}

func (client *Client) GetNC(req cns.GetNetworkContainerRequest) (cns.GetNetworkContainerResponse, error) {
	resp, returnCode := client.RestService.GetNetworkContainerInternal(req)
	if returnCode != 0 {
		if returnCode == types.UnknownContainerID {
			return resp, errors.New(returnCode.String())
		}
		return resp, errors.Errorf("failed to get NC, request: %+v, errorCode: %d", req, returnCode)
	}

	return resp, nil
}

func (client *Client) DeleteNC(req cns.DeleteNetworkContainerRequest) error {
	returnCode := client.RestService.DeleteNetworkContainerInternal(req)
	if returnCode != 0 {
		return fmt.Errorf("Failed to delete NC, request: %+v, errorCode: %d", req, returnCode)
	}

	return nil
}
