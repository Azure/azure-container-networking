package ingress

import (
	"syscall"

	"github.com/vishvananda/netlink"
	"go.uber.org/zap"
)

// SetupIngressFilter sets up the ingress filter
func SetupIngressFilter(ifaceIndex int, objs *IngressObjects, logger *zap.Logger) error {
	link, err := netlink.LinkByIndex(ifaceIndex)
	if err != nil {
		logger.Error("Failed to get link", zap.Error(err))
		return err
	}

	// Get the list of filters on the link
	filters, err := netlink.FilterList(link, netlink.HANDLE_MIN_INGRESS)
	if err != nil {
		logger.Error("Failed to get filter list", zap.Error(err))
		return err
	}

	// Check if egress filter exists and delete it. Filter is identified by its name.
	// this is to avoid duplicate filters after restarting the daemonsetS
	for _, filter := range filters {
		if filter, ok := filter.(*netlink.BpfFilter); ok && filter.Name == "ipv6_hp_ingress" {
			if err := netlink.FilterDel(filter); err != nil {
				logger.Error("Failed to delete filter", zap.Error(err))
				return err
			}
			break
		}
	}

	ingressFilter := &netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: ifaceIndex,
			Parent:    netlink.HANDLE_MIN_INGRESS,
			Protocol:  syscall.ETH_P_ALL,
			Priority:  1,
		},
		Fd:           objs.LinklocalToGua.FD(),
		Name:         "ipv6_hp_ingress",
		DirectAction: true,
	}

	if err := netlink.FilterReplace(ingressFilter); err != nil {
		logger.Error("failed setting ingress filter", zap.Error(err))
		return err
	} else {
		logger.Info("Successfully set ingress filter on", zap.Int("ifaceIndex", ifaceIndex))
	}

	return nil
}
