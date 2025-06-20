package network

import (
	"context"
	"net"
)

type dhcpClient interface {
	DiscoverRequest(context.Context, net.HardwareAddr, string) error
	DHCPRehydrationFeatureOnHost(context.Context) (bool, error)
}

type mockDHCP struct{}

func (d *mockDHCP) DiscoverRequest(context.Context, net.HardwareAddr, string) error {
	return nil
}

func (d *mockDHCP) DHCPRehydrationFeatureOnHost(context.Context) (bool, error) {
	return false, nil
}
