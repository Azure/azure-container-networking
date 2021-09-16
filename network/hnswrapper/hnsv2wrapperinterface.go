// Copyright 2017 Microsoft. All rights reserved.
// MIT License

// +build windows

package hnswrapper

import "github.com/Microsoft/hcsshim/hcn"

type HnsV2WrapperInterface interface {
	CreateEndpoint(endpoint *hcn.HostComputeEndpoint) (*hcn.HostComputeEndpoint, error)
	DeleteEndpoint(endpoint *hcn.HostComputeEndpoint) error
	CreateNetwork(network *hcn.HostComputeNetwork) (*hcn.HostComputeNetwork, error)
	DeleteNetwork(network *hcn.HostComputeNetwork)  error
	GetNamespaceByID(netNamespacePath string) (*hcn.HostComputeNamespace, error)
	AddNamespaceEndpoint(namespaceId string, endpointId string) error
	RemoveNamespaceEndpoint(namespaceId string, endpointId string) error
	GetNetworkByID(networkId string) (*hcn.HostComputeNetwork, error)
	GetEndpointByID(endpointId string) (*hcn.HostComputeEndpoint, error)
}