package ciliumpool

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

var errHostRoutes = errors.New("ciliumpool: invalid host routes")

func checkHostRoutes(cfg *common.CiliumPoolConfig, addresses []netip.Addr) error {
	link, err := net.InterfaceByName(cfg.Interface)
	if err != nil {
		return fmt.Errorf("finding pool interface: %w", err)
	}
	handle, err := netlink.NewHandle(unix.NETLINK_ROUTE)
	if err != nil {
		return fmt.Errorf("opening route diagnostics: %w", err)
	}
	defer handle.Close()
	if err = handle.SetSocketTimeout(time.Second); err != nil {
		return fmt.Errorf("bounding route diagnostics: %w", err)
	}
	routes, err := handle.RouteListFiltered(unix.AF_INET, &netlink.Route{Table: unix.RT_TABLE_UNSPEC}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return fmt.Errorf("reading host routes: %w", err)
	}
	rules, err := handle.RuleList(unix.AF_INET)
	if err != nil {
		return fmt.Errorf("reading host rules: %w", err)
	}
	return validateHostRoutes(link.Index, addresses, routes, rules)
}

func validateHostRoutes(link int, addresses []netip.Addr, routes []netlink.Route, rules []netlink.Rule) error {
	var gateway net.IP
	for i := range routes {
		route := &routes[i]
		if route.Table == unix.RT_TABLE_MAIN && route.LinkIndex == link && defaultRoute(route) &&
			route.Gw.To4() != nil && !route.Gw.IsUnspecified() {
			gateway = route.Gw
			break
		}
	}
	if gateway == nil {
		return fmt.Errorf("%w: primary interface has no IPv4 default gateway", errHostRoutes)
	}
	tunnelRoute := false
	for i := range routes {
		route := &routes[i]
		if route.Table == 101 && defaultRoute(route) && route.LinkIndex == link && route.Gw.Equal(gateway) {
			tunnelRoute = true
		}
	}
	if !tunnelRoute {
		return fmt.Errorf("%w: table 101 does not use the live primary gateway", errHostRoutes)
	}
	priority := -1
	for i := range rules {
		rule := &rules[i]
		if rule.Table == 101 && rule.Mark == 3 && (rule.Mask == nil || *rule.Mask == ^uint32(0)) &&
			rule.Src == nil && rule.Dst == nil && rule.IifName == "" && rule.OifName == "" && !rule.Invert {
			if priority == -1 || rule.Priority < priority {
				priority = rule.Priority
			}
		}
	}
	if priority == -1 {
		return fmt.Errorf("%w: mark 3 rule for table 101 is absent", errHostRoutes)
	}
	return validateRouteRuleOrder(priority, addresses, routes, rules)
}

func validateRouteRuleOrder(priority int, addresses []netip.Addr, routes []netlink.Route, rules []netlink.Rule) error {
	for i := range rules {
		rule := &rules[i]
		if rule.Priority >= priority {
			continue
		}
		if rule.Mark == 3 && rule.Table != 101 {
			return fmt.Errorf("%w: earlier mark 3 rule selects table %d", errHostRoutes, rule.Table)
		}
		if rule.Table != unix.RT_TABLE_LOCAL {
			continue
		}
		for i := range routes {
			route := &routes[i]
			if route.Table != unix.RT_TABLE_LOCAL || route.Type != unix.RTN_LOCAL || route.Dst == nil {
				continue
			}
			for _, address := range addresses {
				if route.Dst.Contains(net.IP(address.AsSlice())) {
					return fmt.Errorf("%w: local route for pod address %s precedes table 101", errHostRoutes, address)
				}
			}
		}
	}
	return nil
}

func defaultRoute(route *netlink.Route) bool {
	if route.Dst == nil {
		return true
	}
	bits, _ := route.Dst.Mask.Size()
	return bits == 0
}
