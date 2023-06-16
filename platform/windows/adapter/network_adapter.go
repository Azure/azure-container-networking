// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package adapter

type NetworkAdapter interface {
	// GetAdapter returns name of adapter if found
	// Must return error if adapter is not found or adapter name empty
	GetAdapterName() (string, error)

	// Get PriorityVLANTag returns PriorityVLANTag value for Adapter
	GetPriorityVLANTag() (int, error)

	// Set adapter's PriorityVLANTag value to desired value if adapter exists
	SetPriorityVLANTag(int) error
}
