// Copyright 2017 Microsoft. All rights reserved.
// MIT License

//go:build windows
// +build windows

package hnswrapper

import (
	"github.com/Microsoft/hcsshim"
)

type HnsV1WrapperInterface interface {
	CreateEndpoint(endpoint *hcsshim.HNSEndpoint, path string) (*hcsshim.HNSEndpoint, error)
	DeleteEndpoint(endpointId string) (*hcsshim.HNSEndpoint, error)
	CreateNetwork(network *hcsshim.HNSNetwork, path string) (*hcsshim.HNSNetwork, error)
	DeleteNetwork(networkId string) (*hcsshim.HNSNetwork, error)
	GetHNSEndpointByName(endpointName string) (*hcsshim.HNSEndpoint, error)
	GetHNSEndpointByID(endpointID string) (*hcsshim.HNSEndpoint, error)
	HotAttachEndpoint(containerID string, endpointID string) error
	IsAttached(hnsep *hcsshim.HNSEndpoint, containerID string) (bool, error)
	GetHNSGlobals() (*hcsshim.HNSGlobals, error)
}
