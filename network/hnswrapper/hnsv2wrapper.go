// Copyright 2017 Microsoft. All rights reserved.
// MIT License

//go:build windows
// +build windows

package hnswrapper

import (
	"github.com/Microsoft/hcsshim/hcn"
)

type Hnsv2wrapper struct {
}

func (Hnsv2wrapper) CreateEndpoint(endpoint *hcn.HostComputeEndpoint) (*hcn.HostComputeEndpoint, error) {
	return endpoint.Create()
}

func (Hnsv2wrapper) DeleteEndpoint(endpoint *hcn.HostComputeEndpoint) error {
	return endpoint.Delete()
}

func (Hnsv2wrapper) CreateNetwork(network *hcn.HostComputeNetwork) (*hcn.HostComputeNetwork, error) {
	return network.Create()
}

func (Hnsv2wrapper) DeleteNetwork(network *hcn.HostComputeNetwork) error {
	return network.Delete()
}

func (Hnsv2wrapper) ModifyNetworkSettings(network *hcn.HostComputeNetwork, request *hcn.ModifyNetworkSettingRequest) error {
	return network.ModifyNetworkSettings(request)
}

func (Hnsv2wrapper) AddNetworkPolicy(network *hcn.HostComputeNetwork, networkPolicy hcn.PolicyNetworkRequest) error {
	return network.AddPolicy(networkPolicy)
}

func (Hnsv2wrapper) RemoveNetworkPolicy(network *hcn.HostComputeNetwork, networkPolicy hcn.PolicyNetworkRequest) error {
	return network.RemovePolicy(networkPolicy)
}

func (w Hnsv2wrapper) GetNamespaceByID(netNamespacePath string) (*hcn.HostComputeNamespace, error) {
	return hcn.GetNamespaceByID(netNamespacePath)
}

func (w Hnsv2wrapper) AddNamespaceEndpoint(namespaceId string, endpointId string) error {
	return hcn.AddNamespaceEndpoint(namespaceId, endpointId)
}

func (w Hnsv2wrapper) RemoveNamespaceEndpoint(namespaceId string, endpointId string) error {
	return hcn.RemoveNamespaceEndpoint(namespaceId, endpointId)
}

func (w Hnsv2wrapper) GetNetworkByName(networkName string) (*hcn.HostComputeNetwork, error) {
	return hcn.GetNetworkByName(networkName)
}

func (w Hnsv2wrapper) GetNetworkByID(networkId string) (*hcn.HostComputeNetwork, error) {
	return hcn.GetNetworkByID(networkId)
}

func (f Hnsv2wrapper) GetEndpointByID(endpointId string) (*hcn.HostComputeEndpoint, error) {
	return hcn.GetEndpointByID(endpointId)
}

func (f Hnsv2wrapper) ListEndpointsOfNetwork(networkId string) ([]hcn.HostComputeEndpoint, error) {
	return hcn.ListEndpointsOfNetwork(networkId)
}

func (f Hnsv2wrapper) ApplyEndpointPolicy(endpoint *hcn.HostComputeEndpoint, requestType hcn.RequestType, endpointPolicy hcn.PolicyEndpointRequest) error {
	return endpoint.ApplyPolicy(requestType, endpointPolicy)
}
