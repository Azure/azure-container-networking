package netio

import (
	"net"

	"github.com/pkg/errors"
	vishnetlink "github.com/vishvananda/netlink"
)

// linkResolver abstracts the netlink link lookups used by ResolveMasterInterface
// so callers can be unit tested without a real netlink socket.
type linkResolver interface {
	LinkByName(name string) (vishnetlink.Link, error)
	LinkByIndex(index int) (vishnetlink.Link, error)
}

type defaultLinkResolver struct{}

func (defaultLinkResolver) LinkByName(name string) (vishnetlink.Link, error) {
	link, err := vishnetlink.LinkByName(name)
	return link, errors.Wrap(err, "netlink LinkByName")
}

func (defaultLinkResolver) LinkByIndex(index int) (vishnetlink.Link, error) {
	link, err := vishnetlink.LinkByIndex(index)
	return link, errors.Wrap(err, "netlink LinkByIndex")
}

// masterLinkResolver is used by ResolveMasterInterface. Overridable in tests.
var masterLinkResolver linkResolver = defaultLinkResolver{}

// ResolveMasterInterface returns the upper (master) device for iface, routing to the
// Linux netlink-based resolver.
func (ns *NetIO) ResolveMasterInterface(iface *net.Interface) (*net.Interface, error) {
	return resolveMasterInterface(iface)
}

// resolveMasterInterface returns the upper (master) device for iface.
//
// On accelerated-networking nodes the SR-IOV VF and its netvsc upper device share a
// MAC address, so a MAC lookup can return either one depending on kernel enumeration
// order. That order is the RTM_GETLINK dump order, which walks the kernel's
// net_device index hash table (NETDEV_HASHENTRIES = 256) and therefore yields devices
// ordered by ifindex%256 rather than by ifindex - so a VF with a high ifindex can be
// returned before its own low-ifindex master.
//
// Only the netvsc master may be moved into a pod network namespace; moving the bare VF
// breaks the netvsc bond. A VF has a non-zero MasterIndex, the master does not, so an
// interface that already is the master (or has no master) is returned unchanged.
//
// The returned *net.Interface carries both the resolved name and its ifindex. Callers
// that subsequently act on the device should use the index: the name can be reassigned
// to a different device between resolution and use when a NIC is replaced and its freed
// name is reused.
func resolveMasterInterface(iface *net.Interface) (*net.Interface, error) {
	if iface == nil {
		return nil, ErrInterfaceNil
	}

	link, err := masterLinkResolver.LinkByName(iface.Name)
	if err != nil {
		return nil, errors.Wrapf(err, "get link %q", iface.Name)
	}

	masterIndex := link.Attrs().MasterIndex
	if masterIndex == 0 {
		return iface, nil
	}

	master, err := masterLinkResolver.LinkByIndex(masterIndex)
	if err != nil {
		return nil, errors.Wrapf(err, "get master link by index %d", masterIndex)
	}

	return &net.Interface{
		Index:        master.Attrs().Index,
		Name:         master.Attrs().Name,
		MTU:          master.Attrs().MTU,
		HardwareAddr: master.Attrs().HardwareAddr,
	}, nil
}
