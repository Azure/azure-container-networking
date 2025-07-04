package dhcp

import (
	"context"
	"net"

	"go.uber.org/zap"
)

type DHCP struct {
	logger *zap.Logger
}

func New(logger *zap.Logger) *DHCP {
	return &DHCP{
		logger: logger,
	}
}

func (c *DHCP) DiscoverRequest(_ context.Context, _ net.HardwareAddr, _ string) error {
	return nil
}

func (c *DHCP) DHCPRehydrationFeatureOnHost(_ context.Context) (bool, error) {
	return false, nil
}
