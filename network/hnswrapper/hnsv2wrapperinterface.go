// Copyright 2017 Microsoft. All rights reserved.
// MIT License

//go:build windows
// +build windows

package hnswrapper

import "github.com/Microsoft/hcsshim/hcn"

type HnsV2WrapperInterface interface {
	CreateEndpoint(endpoint *hcn.HostComputeEndpoint) (*hcn.HostComputeEndpoint, error)
	DeleteEndpoint(endpoint *hcn.HostComputeEndpoint) error
	CreateNetwork(network *hcn.HostComputeNetwork) (*hcn.HostComputeNetwork, error)
	DeleteNetwork(network *hcn.HostComputeNetwork) error
	ModifyNetworkSettings(network *hcn.HostComputeNetwork, request *hcn.ModifyNetworkSettingRequest) error
	AddNetworkPolicy(network *hcn.HostComputeNetwork, networkPolicy hcn.PolicyNetworkRequest) error
	RemoveNetworkPolicy(network *hcn.HostComputeNetwork, networkPolicy hcn.PolicyNetworkRequest) error
	GetNamespaceByID(netNamespacePath string) (*hcn.HostComputeNamespace, error)
	AddNamespaceEndpoint(namespaceId string, endpointId string) error
	RemoveNamespaceEndpoint(namespaceId string, endpointId string) error
	GetNetworkByName(networkName string) (*hcn.HostComputeNetwork, error)
	GetNetworkByID(networkId string) (*hcn.HostComputeNetwork, error)
	GetEndpointByID(endpointId string) (*hcn.HostComputeEndpoint, error)
	ListEndpointsOfNetwork(networkId string) ([]hcn.HostComputeEndpoint, error)
	ApplyEndpointPolicy(endpoint *hcn.HostComputeEndpoint, requestType hcn.RequestType, endpointPolicy hcn.PolicyEndpointRequest) error
}
