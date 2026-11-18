package nodenetworkconfig

import (
	"net"
	"net/netip"

	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

func (r *Reconciler) configurePlatformSink(enabled bool) {
	if !enabled {
		return
	}

	r.platformSink = newPlatformSink(netlink.RouteAdd)
}

func newPlatformSink(routeAdd func(*netlink.Route) error) func([]string) error {
	initialized := false
	return func(primaryIPs []string) error {
		if initialized {
			return nil
		}

		prefixes := make([]netip.Prefix, 0, len(primaryIPs))
		for _, primaryIP := range primaryIPs {
			prefix, err := netip.ParsePrefix(primaryIP)
			if err != nil {
				return errors.Wrapf(err, "invalid overlay prefix %q", primaryIP)
			}
			prefixes = append(prefixes, prefix.Masked())
		}
		for _, prefix := range prefixes {
			family := netlink.FAMILY_V4
			if prefix.Addr().Is6() {
				family = netlink.FAMILY_V6
			}
			route := &netlink.Route{
				Dst:      &net.IPNet{IP: net.IP(prefix.Addr().AsSlice()), Mask: net.CIDRMask(prefix.Bits(), prefix.Addr().BitLen())},
				Family:   family,
				Table:    unix.RT_TABLE_MAIN,
				Type:     unix.RTN_BLACKHOLE,
				Protocol: unix.RTPROT_STATIC,
			}
			if err := routeAdd(route); err != nil && !errors.Is(err, unix.EEXIST) {
				return errors.Wrapf(err, "failed to ensure blackhole route for %s", prefix)
			}
		}
		initialized = len(prefixes) > 0
		return nil
	}
}
