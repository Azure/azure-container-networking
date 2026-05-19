// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
)

// CNSClient defines the interface for CNS client operations used by networkManager
// in stateless CNI mode. This interface allows for dependency injection and unit testing.
//
// The concrete implementation is *cnsclient.Client which makes HTTP calls to CNS.
// For testing, use MockCNSEndpointClient which implements this interface.
type CNSClient interface {
	// GetEndpoint retrieves endpoint state from CNS
	GetEndpoint(ctx context.Context, endpointID string) (*restserver.GetEndpointResponse, error)

	// UpdateEndpoint updates endpoint state in CNS with HNS/veth information
	UpdateEndpoint(ctx context.Context, endpointID string, ifnameToIPInfoMap map[string]*restserver.IPInfo) (*cns.Response, error)

	// DeleteEndpointState removes endpoint state from CNS
	DeleteEndpointState(ctx context.Context, endpointID string) (*cns.Response, error)
}
