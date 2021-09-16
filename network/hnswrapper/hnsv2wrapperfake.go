// Copyright 2017 Microsoft. All rights reserved.
// MIT License

// +build windows

package hnswrapper

import (
	"github.com/Microsoft/hcsshim/hcn"
)

type Hnsv2wrapperFake struct {
}

func (f Hnsv2wrapperFake) CreateNetwork(network *hcn.HostComputeNetwork) (*hcn.HostComputeNetwork, error) {
	return network,nil
}

func (f Hnsv2wrapperFake) DeleteNetwork(network *hcn.HostComputeNetwork) error {
	return nil
}

func (f Hnsv2wrapperFake) GetNetworkByID(networkId string) (*hcn.HostComputeNetwork, error) {
	network := &hcn.HostComputeNetwork{Id: "c84257e3-3d60-40c4-8c47-d740a1c260d3"}
	return network,nil
}

func (f Hnsv2wrapperFake) GetEndpointByID(endpointId string) (*hcn.HostComputeEndpoint, error) {
	endpoint := &hcn.HostComputeEndpoint{Id: "7a2ae98a-0c84-4b35-9684-1c02a2bf7e03"}
	return endpoint,nil
}

func (Hnsv2wrapperFake) CreateEndpoint(endpoint *hcn.HostComputeEndpoint)  (*hcn.HostComputeEndpoint, error)  {
	return endpoint, nil
}

func (Hnsv2wrapperFake) DeleteEndpoint(endpoint *hcn.HostComputeEndpoint) error {
	return nil
}

func (Hnsv2wrapperFake) GetNamespaceByID(netNamespacePath string) (*hcn.HostComputeNamespace, error) {
	nameSpace := &hcn.HostComputeNamespace{Id: "ea37ac15-119e-477b-863b-cc23d6eeaa4d", NamespaceId: 1000}
	return nameSpace, nil
}

func (Hnsv2wrapperFake) AddNamespaceEndpoint(namespaceId string, endpointId string) error {
	return nil
}

func (Hnsv2wrapperFake) RemoveNamespaceEndpoint(namespaceId string, endpointId string) error {
	return nil
}
