// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package cns

import (
	"context"
	"net"
)

// CNIEndpointState is one live interface owned by stateful Azure CNI.
// IPAddresses retain the host addresses and masks returned by CNI.
type CNIEndpointState struct {
	InfraContainerID string
	PodEndpointID    string
	PodName          string
	PodNamespace     string
	InterfaceKey     string
	InterfaceName    string
	IPAddresses      []net.IPNet
}

// CNIEndpointStateProvider reads the complete live stateful CNI endpoint set.
type CNIEndpointStateProvider func(context.Context) ([]CNIEndpointState, error)
