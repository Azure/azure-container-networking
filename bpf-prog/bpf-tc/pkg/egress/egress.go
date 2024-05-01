package egress

import (
	"log"
	"syscall"

	"github.com/vishvananda/netlink"
)

// SetupEgressFilter sets up the egress filter
func SetupEgressFilter(ifaceIndex int, objs *EgressObjects) error {
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
		log.Printf("failed setting egress filter: %v", err)
		return err
	} else {
		log.Printf("Successfully set egress filter on %d..", ifaceIndex)
	}

	return nil
}
