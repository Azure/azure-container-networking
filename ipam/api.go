// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

import (
	"fmt"
)

var (
	// Error responses returned by AddressManager.
	errInvalidAddressSpace     = fmt.Errorf("Invalid address space")
	errInvalidPoolId           = fmt.Errorf("Invalid address pool")
	errInvalidAddress          = fmt.Errorf("Invalid address")
	errInvalidScope            = fmt.Errorf("Invalid scope")
	errInvalidConfiguration    = fmt.Errorf("Invalid configuration")
	errAddressPoolExists       = fmt.Errorf("Address pool already exists")
	errAddressPoolNotFound     = fmt.Errorf("Address pool not found")
	errAddressPoolInUse        = fmt.Errorf("Address pool already in use")
	errAddressPoolNotInUse     = fmt.Errorf("Address pool not in use")
	errNoAvailableAddressPools = fmt.Errorf("No available address pools")
	errAddressExists           = fmt.Errorf("Address already exists")
	errAddressNotFound         = fmt.Errorf("Address not found")
	errAddressInUse            = fmt.Errorf("Address already in use")
	errAddressNotInUse         = fmt.Errorf("Address not in use")
	errNoAvailableAddresses    = fmt.Errorf("No available addresses")

	// Options used by AddressManager.
	OptInterfaceName      = "azure.interface.name"
	OptAddressID          = "azure.address.id"
	OptAddressType        = "azure.address.type"
	OptAddressTypeGateway = "gateway"
)
