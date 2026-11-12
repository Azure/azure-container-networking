package nodenetworkconfig

import (
	"net"
	"net/netip"

	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

type routeAPI interface {
	RouteAdd(*netlink.Route) error
}

type linuxRouteClient struct {
	client routeAPI
}

type netlinkRoutes struct{}

func (netlinkRoutes) RouteAdd(route *netlink.Route) error {
	return errors.Wrap(netlink.RouteAdd(route), "netlink route add")
}

// NewRouteClient creates a client for the CNS process's network namespace.
func NewRouteClient() RouteClient {
	return &linuxRouteClient{client: netlinkRoutes{}}
}

func (c *linuxRouteClient) EnsureBlackholeRoute(prefix netip.Prefix) error {
	if !prefix.IsValid() || prefix.Bits() == 0 {
		return errors.Errorf("invalid overlay prefix for blackhole route: %s", prefix)
	}
	prefix = prefix.Masked()
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
	if err := c.client.RouteAdd(route); err != nil && !errors.Is(err, unix.EEXIST) {
		return errors.Wrapf(err, "failed to add blackhole route for %s", prefix)
	}
	return nil
}

func (r *Reconciler) initializeRoutes(nnc *v1alpha.NodeNetworkConfig) error {
	prefixes := make([]netip.Prefix, 0, len(nnc.Status.NetworkContainers))
	// Dual-stack overlay uses separate IPv4 and IPv6 network containers.
	for i := range nnc.Status.NetworkContainers {
		nc := &nnc.Status.NetworkContainers[i]
		if nc.Type != v1alpha.Overlay || nc.AssignmentMode != v1alpha.Static {
			continue
		}
		if r.nodeIP != "" && nc.NodeIP != r.nodeIP {
			continue
		}
		prefix, err := netip.ParsePrefix(nc.PrimaryIP)
		if err != nil {
			return errors.Wrapf(err, "invalid overlay prefix on network container %s", nc.ID)
		}
		if prefix.Bits() == 0 {
			return errors.Errorf("invalid default-route prefix on overlay network container %s", nc.ID)
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	for _, prefix := range prefixes {
		if err := r.routes.EnsureBlackholeRoute(prefix); err != nil {
			return errors.Wrapf(err, "failed to ensure blackhole route for %s", prefix)
		}
	}
	return nil
}
