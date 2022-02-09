// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"errors"
	"fmt"
)

var (
	// Error responses returned by NetworkManager.
	errSubnetNotFound         = fmt.Errorf("Subnet not found")
	errNetworkModeInvalid     = fmt.Errorf("Network mode is invalid")
	errNetworkExists          = fmt.Errorf("Network already exists")
	errNetworkNotFound        = &networkNotFoundError{}
	errEndpointExists         = fmt.Errorf("Endpoint already exists")
	errEndpointNotFound       = fmt.Errorf("Endpoint not found")
	errNamespaceNotFound      = fmt.Errorf("Namespace not found")
	errMultipleEndpointsFound = fmt.Errorf("Multiple endpoints found")
	errEndpointInUse          = fmt.Errorf("Endpoint is already joined to a sandbox")
	errEndpointNotInUse       = fmt.Errorf("Endpoint is not joined to a sandbox")
)

type networkNotFoundError struct{}

func (n *networkNotFoundError) Error() string {
	return "Network not found"
}

func IsNetworkNotFoundError(err error) bool {
	return errors.Is(err, errNetworkNotFound)
}
