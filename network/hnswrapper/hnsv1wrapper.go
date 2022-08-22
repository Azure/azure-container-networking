//go:build windows
// +build windows

package hnswrapper

import (
	"encoding/json"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Microsoft/hcsshim"
)

type Hnsv1wrapper struct{}

func (Hnsv1wrapper) CreateEndpoint(endpoint *hcsshim.HNSEndpoint, path string) (*hcsshim.HNSEndpoint, error) {
	// Marshal the request.
	buffer, err := json.Marshal(endpoint)
	if err != nil {
		return nil, err
	}
	hnsRequest := string(buffer)

	// Create the HNS endpoint.
	log.Printf("[net] HNSEndpointRequest POST request:%+v", hnsRequest)
	hnsResponse, err := hcsshim.HNSEndpointRequest("POST", path, hnsRequest)
	log.Printf("[net] HNSEndpointRequest POST response:%+v err:%v.", hnsResponse, err)

	if err != nil {
		return nil, err
	}

	return hnsResponse, err
}

func (Hnsv1wrapper) DeleteEndpoint(endpointId string) (*hcsshim.HNSEndpoint, error) {
	hnsResponse, err := hcsshim.HNSEndpointRequest("DELETE", endpointId, "")
	if err != nil {
		return nil, err
	}

	return hnsResponse, err
}

func (Hnsv1wrapper) CreateNetwork(network *hcsshim.HNSNetwork, path string) (*hcsshim.HNSNetwork, error) {
	// Marshal the request.
	buffer, err := json.Marshal(network)
	if err != nil {
		return nil, err
	}
	hnsRequest := string(buffer)

	// Create the HNS network.
	log.Printf("[net] HNSNetworkRequest POST request:%+v", hnsRequest)
	hnsResponse, err := hcsshim.HNSNetworkRequest("POST", path, hnsRequest)
	log.Printf("[net] HNSNetworkRequest POST response:%+v err:%v.", hnsResponse, err)

	if err != nil {
		return nil, err
	}

	return hnsResponse, nil
}

func (Hnsv1wrapper) DeleteNetwork(networkId string) (*hcsshim.HNSNetwork, error) {
	hnsResponse, err := hcsshim.HNSNetworkRequest("DELETE", networkId, "")
	if err != nil {
		return nil, err
	}

	return hnsResponse, err
}

func (Hnsv1wrapper) GetHNSEndpointByName(endpointName string) (*hcsshim.HNSEndpoint, error) {
	return hcsshim.GetHNSEndpointByName(endpointName)
}

func (Hnsv1wrapper) GetHNSEndpointByID(id string) (*hcsshim.HNSEndpoint, error) {
	return hcsshim.GetHNSEndpointByID(id)
}

func (Hnsv1wrapper) HotAttachEndpoint(containerID, endpointID string) error {
	return hcsshim.HotAttachEndpoint(containerID, endpointID)
}

func (Hnsv1wrapper) IsAttached(endpoint *hcsshim.HNSEndpoint, containerID string) (bool, error) {
	return endpoint.IsAttached(containerID)
}

func (w Hnsv1wrapper) GetHNSGlobals() (*hcsshim.HNSGlobals, error) {
	return hcsshim.GetHNSGlobals()
}
