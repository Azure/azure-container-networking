// Copyright 2017 Microsoft. All rights reserved.
// MIT License

//go:build windows
// +build windows

package hnswrapper

import (
	"github.com/Microsoft/hcsshim/hcn"
)

type Hnsv2wrapper struct{}

func (Hnsv2wrapper) CreateEndpoint(endpoint *hcn.HostComputeEndpoint) (*hcn.HostComputeEndpoint, error) {
	return endpoint.Create() // nolint:wrapcheck // no need to wrap check for this wrapper
}

func (Hnsv2wrapper) DeleteEndpoint(endpoint *hcn.HostComputeEndpoint) error {
	return endpoint.Delete() // nolint:wrapcheck // no need to wrap check for this wrapper
}

func (Hnsv2wrapper) CreateNetwork(network *hcn.HostComputeNetwork) (*hcn.HostComputeNetwork, error) {
	return network.Create() // nolint:wrapcheck // no need to wrap check for this wrapper
}

func (Hnsv2wrapper) DeleteNetwork(network *hcn.HostComputeNetwork) error {
	return network.Delete() // nolint:wrapcheck // no need to wrap check for this wrapper
}

func (Hnsv2wrapper) ModifyNetworkSettings(network *hcn.HostComputeNetwork, request *hcn.ModifyNetworkSettingRequest) error {
	return network.ModifyNetworkSettings(request) // nolint:wrapcheck // no need to wrap check for this wrapper
}

func (Hnsv2wrapper) AddNetworkPolicy(network *hcn.HostComputeNetwork, networkPolicy hcn.PolicyNetworkRequest) error {
	return network.AddPolicy(networkPolicy) // nolint:wrapcheck // no need to wrap check for this wrapper
}

func (Hnsv2wrapper) RemoveNetworkPolicy(network *hcn.HostComputeNetwork, networkPolicy hcn.PolicyNetworkRequest) error {
	return network.RemovePolicy(networkPolicy) // nolint:wrapcheck // no need to wrap check for this wrapper
}

func (Hnsv2wrapper) GetNamespaceByID(netNamespacePath string) (*hcn.HostComputeNamespace, error) {
	return hcn.GetNamespaceByID(netNamespacePath) // nolint:wrapcheck // no need to wrap check for this wrapper
}

func (Hnsv2wrapper) AddNamespaceEndpoint(namespaceID, endpointID string) error {
	return hcn.AddNamespaceEndpoint(namespaceID, endpointID) // nolint:wrapcheck // no need to wrap check for this wrapper
}

func (Hnsv2wrapper) RemoveNamespaceEndpoint(namespaceID, endpointID string) error {
	return hcn.RemoveNamespaceEndpoint(namespaceID, endpointID) // nolint:wrapcheck // no need to wrap check for this wrapper
}

func (Hnsv2wrapper) GetNetworkByName(networkName string) (*hcn.HostComputeNetwork, error) {
	return hcn.GetNetworkByName(networkName) // nolint:wrapcheck // no need to wrap check for this wrapper
}

func (Hnsv2wrapper) GetNetworkByID(networkID string) (*hcn.HostComputeNetwork, error) {
	return hcn.GetNetworkByID(networkID) // nolint:wrapcheck // no need to wrap check for this wrapper
}

func (Hnsv2wrapper) GetEndpointByID(endpointID string) (*hcn.HostComputeEndpoint, error) {
	return hcn.GetEndpointByID(endpointID) // nolint:wrapcheck // no need to wrap check for this wrapper
}

func (Hnsv2wrapper) ListEndpointsOfNetwork(networkID string) ([]hcn.HostComputeEndpoint, error) {
	return hcn.ListEndpointsOfNetwork(networkID) // nolint:wrapcheck // no need to wrap check for this wrapper
}

func (Hnsv2wrapper) ListEndpointsQuery(query hcn.HostComputeQuery) ([]hcn.HostComputeEndpoint, error) {
	return hcn.ListEndpointsQuery(query) // nolint:wrapcheck // no need to wrap check for this wrapper
}

func (Hnsv2wrapper) ApplyEndpointPolicy(endpoint *hcn.HostComputeEndpoint, requestType hcn.RequestType, endpointPolicy hcn.PolicyEndpointRequest) error {
	return endpoint.ApplyPolicy(requestType, endpointPolicy) // nolint:wrapcheck // no need to wrap check for this wrapper
}

func (Hnsv2wrapper) GetEndpointByName(endpointName string) (*hcn.HostComputeEndpoint, error) {
	return hcn.GetEndpointByName(endpointName) // nolint:wrapcheck // no need to wrap check for this wrapper
}
