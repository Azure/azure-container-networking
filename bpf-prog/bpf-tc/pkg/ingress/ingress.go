package ingress

import (
	"syscall"

	"github.com/vishvananda/netlink"
	"go.uber.org/zap"
)

// SetupIngressFilter sets up the ingress filter
func SetupIngressFilter(ifaceIndex int, objs *IngressObjects, logger *zap.Logger) error {
	ingressFilter := &netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: ifaceIndex,
			Parent:    netlink.HANDLE_MIN_INGRESS,
			Protocol:  syscall.ETH_P_ALL,
			Priority:  1,
		},
		Fd:           objs.LinklocalToGua.FD(),
		Name:         "ingress_filter",
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
