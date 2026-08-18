package network

import (
	"net"

	"github.com/pkg/errors"
	vishnetlink "github.com/vishvananda/netlink"
	"go.uber.org/zap"
)

// errNoAddressesInNetns is returned when the container interface exists but has
// no usable addresses, leaving nothing to reconstruct the endpoint from.
var errNoAddressesInNetns = errors.New("no addresses found on container interface")

// netnsLinkClient abstracts the netlink lookups performed while reconstructing an
// endpoint from a pod network namespace, so tests can run without a real netns.
type netnsLinkClient interface {
	LinkByName(name string) (vishnetlink.Link, error)
	LinkByIndex(index int) (vishnetlink.Link, error)
	AddrList(link vishnetlink.Link, family int) ([]vishnetlink.Addr, error)
}

// defaultNetnsLinkClient delegates to the real vishvananda/netlink package.
type defaultNetnsLinkClient struct{}

func (defaultNetnsLinkClient) LinkByName(name string) (vishnetlink.Link, error) {
	link, err := vishnetlink.LinkByName(name)
	return link, errors.Wrapf(err, "link by name %s", name)
}

func (defaultNetnsLinkClient) LinkByIndex(index int) (vishnetlink.Link, error) {
	link, err := vishnetlink.LinkByIndex(index)
	return link, errors.Wrapf(err, "link by index %d", index)
}

func (defaultNetnsLinkClient) AddrList(link vishnetlink.Link, family int) ([]vishnetlink.Addr, error) {
	addrs, err := vishnetlink.AddrList(link, family)
	return addrs, errors.Wrap(err, "list addresses")
}

// GetEndpointInfoFromNetns reconstructs a minimal EndpointInfo by inspecting the
// pod network namespace directly, for use when CNS cannot be reached during DEL
// and there is therefore no stored endpoint state to delete against.
//
// Without this, a stateless DEL that cannot reach CNS returns zero endpoints and
// skips endpoint deletion entirely. Modes whose per-pod host state is owned by
// the veth (routes, for example) still recover, because the kernel drops that
// state when the runtime tears the netns down. Modes that additionally program
// node-scoped state keyed by pod IP or host veth name do not: nothing references
// the departing interface, so the entries are orphaned on the node permanently.
func (nm *networkManager) GetEndpointInfoFromNetns(endpointID, netnsPath, ifName string) (*EndpointInfo, error) {
	return getEndpointInfoFromNetns(nm.nsClient, defaultNetnsLinkClient{}, endpointID, netnsPath, ifName)
}

// getEndpointInfoFromNetns performs the namespace entry and the two-sided link
// lookup. The container interface's addresses and its peer index must be read
// inside the namespace, while resolving that peer index to a host interface name
// must happen back on the host, so the work is deliberately split around Exit.
func getEndpointInfoFromNetns(nsc NamespaceClientInterface, linkClient netnsLinkClient,
	endpointID, netnsPath, ifName string,
) (*EndpointInfo, error) {
	if netnsPath == "" {
		return nil, errors.New("no network namespace path provided")
	}
	if ifName == "" {
		ifName = InfraInterfaceName
	}

	var (
		ipAddresses []net.IPNet
		peerIndex   int
	)

	if err := executeInNetnsPath(nsc, netnsPath, func() error {
		link, linkErr := linkClient.LinkByName(ifName)
		if linkErr != nil {
			return errors.Wrapf(linkErr, "look up interface %s", ifName)
		}

		addrs, addrErr := linkClient.AddrList(link, vishnetlink.FAMILY_ALL)
		if addrErr != nil {
			return errors.Wrapf(addrErr, "list addresses on %s", ifName)
		}

		for i := range addrs {
			if addrs[i].IPNet == nil || addrs[i].IP.IsLinkLocalUnicast() {
				continue
			}
			ipAddresses = append(ipAddresses, *addrs[i].IPNet)
		}

		// For a veth, IFLA_LINK carries the peer's index as seen from the peer's
		// namespace, which is the host side we need to name the interface.
		peerIndex = link.Attrs().ParentIndex
		return nil
	}); err != nil {
		return nil, err
	}

	if len(ipAddresses) == 0 {
		return nil, errors.Wrapf(errNoAddressesInNetns, "interface %s in %s", ifName, netnsPath)
	}

	epInfo := &EndpointInfo{
		EndpointID:  endpointID,
		IfName:      ifName,
		NetNsPath:   netnsPath,
		IPAddresses: ipAddresses,
	}

	// A missing peer index is not fatal: the addresses alone still let IP-keyed
	// state be cleaned up, which is better than abandoning the delete entirely.
	if peerIndex == 0 {
		logger.Warn("no peer index on container interface, host interface name unresolved",
			zap.String("ifName", ifName), zap.String("netns", netnsPath))
		return epInfo, nil
	}

	hostLink, err := linkClient.LinkByIndex(peerIndex)
	if err != nil {
		logger.Warn("failed to resolve host interface from peer index",
			zap.Int("peerIndex", peerIndex), zap.Error(err))
		return epInfo, nil
	}
	epInfo.HostIfName = hostLink.Attrs().Name

	logger.Info("reconstructed endpoint from netns",
		zap.String("ifName", ifName), zap.String("hostIfName", epInfo.HostIfName),
		zap.Any("ipAddresses", ipAddresses))

	return epInfo, nil
}

// executeInNetnsPath runs f inside the namespace at the given path. It differs
// from ExecuteInNS, which resolves a bind-mounted name under /var/run/netns,
// because the CNI runtime hands us an absolute path instead.
func executeInNetnsPath(nsc NamespaceClientInterface, netnsPath string, f func() error) error {
	ns, err := nsc.OpenNamespace(netnsPath)
	if err != nil {
		return errors.Wrapf(err, "open netns %s", netnsPath)
	}
	defer ns.Close()

	if err := ns.Enter(); err != nil {
		return errors.Wrapf(err, "enter netns %s", netnsPath)
	}
	defer func() {
		if exitErr := ns.Exit(); exitErr != nil {
			logger.Error("failed to exit netns", zap.String("netns", netnsPath), zap.Error(exitErr))
		}
	}()

	return f()
}
