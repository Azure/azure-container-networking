package network

import (
	"context"
	"net"
)

type dhcpClient interface {
	DiscoverRequest(context.Context, net.HardwareAddr, string) error
}

type mockDHCP struct{}

func (netns *mockDHCP) DiscoverRequest(context.Context, net.HardwareAddr, string) error {
	return nil
}
