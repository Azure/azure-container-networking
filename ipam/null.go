// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

import (
	"net"
)

// Null IPAM configuration source.
type nullSource struct {
	name        string
	sink        addressConfigSink
	initialized bool
}

// Creates the null source.
func newNullSource() (*nullSource, error) {
	return &nullSource{
		name: "Null",
	}, nil
}

// Starts the null source.
func (s *nullSource) start(sink addressConfigSink) error {
	s.sink = sink
	return nil
}

// Stops the null source.
func (s *nullSource) stop() {
	s.sink = nil
}

// Refreshes configuration.
func (s *nullSource) refresh() error {

	// Initialize once.
	if s.initialized {
		return nil
	}
	s.initialized = true

	// Configure the local default address space.
	local, err := s.sink.newAddressSpace(LocalDefaultAddressSpaceId, LocalScope)
	if err != nil {
		return err
	}

	subnet := net.IPNet{
		IP:   net.IPv4(0, 0, 0, 0),
		Mask: net.IPv4Mask(0, 0, 0, 0),
	}

	_, err = local.newAddressPool("", 0, &subnet)
	if err != nil {
		return err
	}

	// Set the local address space as active.
	return s.sink.setAddressSpace(local)
}
