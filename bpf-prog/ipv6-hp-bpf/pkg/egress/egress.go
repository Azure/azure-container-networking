package egress

import (
	"syscall"

	"github.com/vishvananda/netlink"
	"go.uber.org/zap"
)

// SetupEgressFilter sets up the egress filter
func SetupEgressFilter(ifaceIndex int, objs *EgressObjects, logger *zap.Logger) error {
	link, err := netlink.LinkByIndex(ifaceIndex)
	if err != nil {
		logger.Error("Failed to get link", zap.Error(err))
		return err
	}

	// Get the list of filters on the link
	filters, err := netlink.FilterList(link, netlink.HANDLE_MIN_EGRESS)
	if err != nil {
		logger.Error("Failed to get filter list", zap.Error(err))
		return err
	}

	// Check if egress filter exists and delete it. Filter is identified by its name.
	// this is to avoid duplicate filters after restarting the daemonset
	for _, filter := range filters {
		if filter, ok := filter.(*netlink.BpfFilter); ok && filter.Name == "ipv6_hp_egress" {
			if err := netlink.FilterDel(filter); err != nil {
				logger.Error("Failed to delete filter", zap.Error(err))
				return err
			}
			break
		}
	}

	egressFilter := &netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: ifaceIndex,
			Parent:    netlink.HANDLE_MIN_EGRESS,
			Protocol:  syscall.ETH_P_ALL,
			Priority:  1,
		},
		Fd:           objs.GuaToLinklocal.FD(),
		Name:         "ipv6_hp_egress",
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
