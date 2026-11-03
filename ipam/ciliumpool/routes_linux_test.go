package ciliumpool

import (
	"net"
	"net/netip"
	"strings"
	"testing"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

func TestHostRoutesDetectLocalPodConflict(t *testing.T) {
	gw := net.ParseIP("10.0.0.1")
	routes := make([]netlink.Route, 0, 3)
	routes = append(routes, netlink.Route{Table: unix.RT_TABLE_MAIN, LinkIndex: 2, Gw: gw},
		netlink.Route{Table: 101, LinkIndex: 2, Gw: gw})
	rules := []netlink.Rule{
		{Table: unix.RT_TABLE_LOCAL, Priority: 0},
		{Table: 101, Mark: 3, Priority: 100},
	}
	addresses := []netip.Addr{netip.MustParseAddr("10.0.0.10")}
	if err := validateHostRoutes(2, addresses, routes, rules); err != nil {
		t.Fatal(err)
	}
	_, dst, err := net.ParseCIDR("10.0.0.10/32")
	if err != nil {
		t.Fatal(err)
	}
	routes = append(routes, netlink.Route{Table: unix.RT_TABLE_LOCAL, Type: unix.RTN_LOCAL, Dst: dst})
	if err = validateHostRoutes(2, addresses, routes, rules); err == nil || !strings.Contains(err.Error(), "local route") {
		t.Fatalf("host-local pod address was not diagnosed: %v", err)
	}
	// A host address outside the operator pool is not a pod-route conflict.
	if err = validateHostRoutes(2, []netip.Addr{netip.MustParseAddr("10.0.0.11")}, routes, rules); err != nil {
		t.Fatal(err)
	}
	if err = validateHostRoutes(2, addresses, routes[:1], rules); err == nil {
		t.Fatal("missing tunnel gateway was accepted")
	}
	if err = validateHostRoutes(2, addresses, routes[:2], rules[:1]); err == nil {
		t.Fatal("missing mark rule was accepted")
	}
}
