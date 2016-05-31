// Copyright Microsoft Corp.
// All rights reserved.

package ipam

import (
	"net"
)

// Null IPAM configuration source.
type nullSource struct {
	name        string
	sink        configSink
	initialized bool
}

// Creates the null source.
func newNullSource(sink configSink) (*nullSource, error) {
	return &nullSource{
		name: "Null",
		sink: sink,
	}, nil
}

// Starts the null source.
func (s *nullSource) start() error {
	return nil
}

// Stops the null source.
func (s *nullSource) stop() {
	return
}

// Refreshes configuration.
func (s *nullSource) refresh() error {

	// Initialize once.
	if s.initialized == true {
		return nil
	}
	s.initialized = true

	// Configure the local default address space.
	local, err := newAddressSpace(localDefaultAddressSpaceId, localScope)
	if err != nil {
		return err
	}

	subnet := net.IPNet{
		IP:   net.IPv4(0, 0, 0, 0),
		Mask: net.IPv4Mask(0, 0, 0, 0),
	}

	_, err = local.newAddressPool("", &subnet)
	if err != nil {
		return err
	}

	// Set the local address space as active.
	s.sink.setAddressSpace(local)

	return nil
}
