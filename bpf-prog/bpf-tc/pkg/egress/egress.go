package egress

import (
	"syscall"

	"github.com/vishvananda/netlink"
	"go.uber.org/zap"
)

// SetupEgressFilter sets up the egress filter
func SetupEgressFilter(ifaceIndex int, objs *EgressObjects, logger *zap.Logger) error {
	egressFilter := &netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: ifaceIndex,
			Parent:    netlink.HANDLE_MIN_EGRESS,
			Protocol:  syscall.ETH_P_ALL,
			Priority:  1,
		},
		Fd:           objs.GuaToLinklocal.FD(),
		Name:         "egress_filter",
		DirectAction: true,
	}

	if err := netlink.FilterReplace(egressFilter); err != nil {
		logger.Error("failed setting egress filter", zap.Error(err))
		return err
	} else {
		logger.Info("Successfully set egress filter on", zap.Int("ifaceIndex", ifaceIndex))
	}

	return nil
}
