//go:build windows
// +build windows

package hnswrapper

import (
	"encoding/json"

	"github.com/Azure/azure-container-networking/cni/log"
	"github.com/Microsoft/hcsshim"
	"go.uber.org/zap"
)

var logger = log.CNILogger.With(zap.String("component", "net-hns"))

type Hnsv1wrapper struct{}

func (Hnsv1wrapper) CreateEndpoint(endpoint *hcsshim.HNSEndpoint, path string) (*hcsshim.HNSEndpoint, error) {
	// Marshal the request.
	buffer, err := json.Marshal(endpoint)
	if err != nil {
		return nil, err
	}
	hnsRequest := string(buffer)

	// Create the HNS endpoint.
	logger.Info("HNSEndpointRequest POST", zap.String("request", hnsRequest))
	hnsResponse, err := hcsshim.HNSEndpointRequest("POST", path, hnsRequest)
	logger.Info("HNSEndpointRequest POST", zap.Any("response", hnsResponse), zap.Error(err))

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
	logger.Info("HNSNetworkRequest POST request", zap.String("request", hnsRequest))
	hnsResponse, err := hcsshim.HNSNetworkRequest("POST", path, hnsRequest)
	logger.Info("HNSNetworkRequest POST response", zap.Any("response", hnsResponse), zap.Error(err))

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
