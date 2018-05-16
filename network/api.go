// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"fmt"
)

var (
	// Error responses returned by NetworkManager.
	errSubnetNotFound     = fmt.Errorf("Subnet not found")
	errNetworkModeInvalid = fmt.Errorf("Network mode is invalid")
	errNetworkExists      = fmt.Errorf("Network already exists")
	errNetworkNotFound    = fmt.Errorf("Network not found")
	errEndpointExists     = fmt.Errorf("Endpoint already exists")
	errEndpointNotFound   = fmt.Errorf("Endpoint not found")
	errEndpointInUse      = fmt.Errorf("Endpoint is already joined to a sandbox")
	errEndpointNotInUse   = fmt.Errorf("Endpoint is not joined to a sandbox")

	OptVethName = "vethname"
)
