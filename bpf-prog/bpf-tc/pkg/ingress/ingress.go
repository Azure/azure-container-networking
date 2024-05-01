package ingress

import (
	"log"
	"syscall"

	"github.com/vishvananda/netlink"
)

// SetupIngressFilter sets up the ingress filter
func SetupIngressFilter(ifaceIndex int, objs *IngressObjects) error {
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
		log.Printf("failed setting ingress filter: %v", err)
		return err
	} else {
		log.Printf("Successfully set ingress filter on %d..", ifaceIndex)
	}

	return nil
}
