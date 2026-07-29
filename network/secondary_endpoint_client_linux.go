package network

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/netns"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/pkg/errors"
	vishnetlink "github.com/vishvananda/netlink"
	"go.uber.org/zap"
)

const (
	NetworkNotReadyErrorMsg = "network is not ready"

	// dhcpDiscoverTimeout is the per-attempt deadline for a single DHCP Discover.
	dhcpDiscoverTimeout = 3 * time.Second
	// dhcpDiscoverAttempts is the number of DHCP Discover attempts. On accelerated
	// networking (MANA) nodes the freshly moved interface may not have carrier yet
	// and a single Discover can race datapath readiness or be lost, so we retransmit
	// like a standard DHCP client instead of failing the whole CNI ADD.
	dhcpDiscoverAttempts = 3
)

// dhcpDiscoverRetryDelay is the backoff between DHCP Discover attempts. It is a var
// so tests can shorten it.
var dhcpDiscoverRetryDelay = 1 * time.Second

var errorSecondaryEndpointClient = errors.New("SecondaryEndpointClient Error")

func newErrorSecondaryEndpointClient(err error) error {
	return errors.Wrapf(err, "%s", errorSecondaryEndpointClient)
}

type SecondaryEndpointClient struct {
	netlink        netlink.NetlinkInterface
	netioshim      netio.NetIOInterface
	plClient       platform.ExecClient
	netUtilsClient networkutils.NetworkUtils
	nsClient       NamespaceClientInterface
	dhcpClient     dhcpClient
	ep             *endpoint
}

func NewSecondaryEndpointClient(
	nl netlink.NetlinkInterface,
	nioc netio.NetIOInterface,
	plc platform.ExecClient,
	nsc NamespaceClientInterface,
	dhcpClient dhcpClient,
	endpoint *endpoint,
) *SecondaryEndpointClient {
	client := &SecondaryEndpointClient{
		netlink:        nl,
		netioshim:      nioc,
		plClient:       plc,
		netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
		nsClient:       nsc,
		dhcpClient:     dhcpClient,
		ep:             endpoint,
	}

	return client
}

// linkResolver abstracts the vishvananda/netlink link lookups used by
// resolveMasterInterface so it can be unit tested without a real netlink socket.
type linkResolver interface {
	LinkByName(name string) (vishnetlink.Link, error)
	LinkByIndex(index int) (vishnetlink.Link, error)
}

type vishLinkResolver struct{}

func (vishLinkResolver) LinkByName(name string) (vishnetlink.Link, error) {
	link, err := vishnetlink.LinkByName(name)
	return link, errors.Wrapf(err, "netlink LinkByName %q", name)
}

func (vishLinkResolver) LinkByIndex(index int) (vishnetlink.Link, error) {
	link, err := vishnetlink.LinkByIndex(index)
	return link, errors.Wrapf(err, "netlink LinkByIndex %d", index)
}

// masterNl is the netlink client used by resolveMasterInterface. Overridable in tests.
var masterNl linkResolver = vishLinkResolver{}

// resolveMasterInterface returns the netvsc upper (master) device name for the given
// interface. On accelerated-networking nodes the SR-IOV VF and its netvsc master share
// a MAC; the VF has a non-zero MasterIndex while the master does not. Only the master may
// be moved into a pod netns, so a VF name is resolved up to its master. A name that is
// already the master (or has no master) is returned unchanged.
func resolveMasterInterface(name string) (string, error) {
	link, err := masterNl.LinkByName(name)
	if err != nil {
		return "", errors.Wrapf(err, "get link %q", name)
	}
	masterIndex := link.Attrs().MasterIndex
	if masterIndex == 0 {
		return name, nil
	}
	master, err := masterNl.LinkByIndex(masterIndex)
	if err != nil {
		return "", errors.Wrapf(err, "get master link by index %d", masterIndex)
	}
	return master.Attrs().Name, nil
}

func (client *SecondaryEndpointClient) AddEndpoints(epInfo *EndpointInfo) error {
	iface, err := client.netioshim.GetNetworkInterfaceByMac(epInfo.MacAddress)
	if err != nil {
		return newErrorSecondaryEndpointClient(err)
	}

	// On accelerated-networking nodes the MAC is shared by the SR-IOV VF and its
	// netvsc upper (master) device, and GetNetworkInterfaceByMac may return either
	// one depending on kernel enumeration order. Only the upper device may be moved
	// into the pod network namespace; moving the bare VF breaks the bond and fails
	// later with "no such network interface". Resolve to the upper device here so
	// the rest of the flow operates on the correct interface regardless of order.
	ifName, err := resolveMasterInterface(iface.Name)
	if err != nil {
		return newErrorSecondaryEndpointClient(err)
	}

	epInfo.IfName = ifName
	if _, exists := client.ep.SecondaryInterfaces[ifName]; exists {
		return newErrorSecondaryEndpointClient(errors.New(ifName + " already exists"))
	}

	ipconfigs := make([]*IPConfig, len(epInfo.IPAddresses))
	for i, ipconfig := range epInfo.IPAddresses {
		ipconfigs[i] = &IPConfig{Address: ipconfig}
	}

	client.ep.SecondaryInterfaces[ifName] = &InterfaceInfo{
		Name:              ifName,
		MacAddress:        epInfo.MacAddress,
		IPConfigs:         ipconfigs,
		NICType:           epInfo.NICType,
		SkipDefaultRoutes: epInfo.SkipDefaultRoutes,
	}

	return nil
}

func (client *SecondaryEndpointClient) AddEndpointRules(_ *EndpointInfo) error {
	return nil
}

func (client *SecondaryEndpointClient) DeleteEndpointRules(_ *endpoint) {
}

func (client *SecondaryEndpointClient) MoveEndpointsToContainerNS(epInfo *EndpointInfo, nsID uintptr) error {
	// Move the container interface to container's network namespace.
	logger.Info("[net] Setting link %v netns %v.", zap.String("IfName", epInfo.IfName), zap.String("NetNsPath", epInfo.NetNsPath))
	if err := client.netlink.SetLinkNetNs(epInfo.IfName, nsID); err != nil {
		return newErrorSecondaryEndpointClient(err)
	}

	return nil
}

func (client *SecondaryEndpointClient) SetupContainerInterfaces(epInfo *EndpointInfo) error {
	logger.Info("[net] Setting link state up.", zap.String("IfName", epInfo.IfName))
	if err := client.netlink.SetLinkState(epInfo.IfName, true); err != nil {
		return newErrorSecondaryEndpointClient(err)
	}

	return nil
}

func (client *SecondaryEndpointClient) ConfigureContainerInterfacesAndRoutes(epInfo *EndpointInfo) error {
	if err := client.netUtilsClient.AssignIPToInterface(epInfo.IfName, epInfo.IPAddresses); err != nil {
		return newErrorSecondaryEndpointClient(err)
	}

	ifInfo, exists := client.ep.SecondaryInterfaces[epInfo.IfName]
	if !exists {
		return newErrorSecondaryEndpointClient(errors.New(epInfo.IfName + " does not exist"))
	}

	if len(epInfo.Routes) < 1 {
		return newErrorSecondaryEndpointClient(errors.New("routes expected for " + epInfo.IfName))
	}

	// virtual gw route needs to be scope link
	for i := range epInfo.Routes {
		if epInfo.Routes[i].Gw == nil {
			epInfo.Routes[i].Scope = netlink.RT_SCOPE_LINK
		}
	}

	if err := addRoutes(client.netlink, client.netioshim, epInfo.IfName, epInfo.Routes); err != nil {
		return newErrorSecondaryEndpointClient(err)
	}

	ifInfo.Routes = append(ifInfo.Routes, epInfo.Routes...)

	// issue dhcp discover packet to ensure mapping created for dns via wireserver to work
	// we do not use the response for anything.
	// Retry with backoff: on accelerated networking (MANA) nodes the moved interface
	// may not have carrier immediately, so a single Discover can race datapath readiness
	// or be dropped. Retransmitting avoids failing the CNI ADD with a spurious timeout.
	logger.Info("Sending DHCP packet", zap.Stringer("macAddress", epInfo.MacAddress), zap.String("ifName", epInfo.IfName))
	var err error
	for attempt := 1; attempt <= dhcpDiscoverAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), dhcpDiscoverTimeout)
		err = client.dhcpClient.DiscoverRequest(ctx, epInfo.MacAddress, epInfo.IfName)
		cancel()
		if err == nil {
			break
		}
		// Transient: a Discover can be lost before the interface has carrier. Log at
		// Warn since the retry usually succeeds; the final failure is returned below.
		logger.Warn("DHCP discover attempt failed, will retry",
			zap.Int("attempt", attempt), zap.Int("maxAttempts", dhcpDiscoverAttempts),
			zap.String("ifName", epInfo.IfName), zap.Error(err))
		if attempt < dhcpDiscoverAttempts {
			time.Sleep(dhcpDiscoverRetryDelay)
		}
	}
	if err != nil {
		return errors.Wrap(err, NetworkNotReadyErrorMsg+" - failed to issue dhcp discover packet to create mapping in host")
	}
	logger.Info("Finished configuring container interfaces and routes for secondary endpoint client")

	return nil
}

func (client *SecondaryEndpointClient) DeleteEndpoints(ep *endpoint) error {
	// Get VM namespace
	vmns, err := netns.New().Get()
	if err != nil {
		return newErrorSecondaryEndpointClient(err)
	}

	// Open the network namespace.
	logger.Info("Opening netns", zap.Any("NetNsPath", ep.NetworkNameSpace))
	ns, err := client.nsClient.OpenNamespace(ep.NetworkNameSpace)
	if err != nil {
		if strings.Contains(err.Error(), errFileNotExist.Error()) {
			// clear SecondaryInterfaces map since network namespace doesn't exist anymore
			ep.SecondaryInterfaces = make(map[string]*InterfaceInfo)
			return nil
		}

		return newErrorSecondaryEndpointClient(err)
	}
	defer ns.Close()

	// Enter the container network namespace.
	logger.Info("Entering netns", zap.Any("NetNsPath", ep.NetworkNameSpace))
	if err := ns.Enter(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			ep.SecondaryInterfaces = make(map[string]*InterfaceInfo)
			return nil
		}

		return newErrorSecondaryEndpointClient(err)
	}

	// Return to host network namespace.
	defer func() {
		logger.Info("Exiting netns", zap.Any("NetNsPath", ep.NetworkNameSpace))
		if err := ns.Exit(); err != nil {
			logger.Error("Failed to exit netns with", zap.Error(newErrorSecondaryEndpointClient(err)))
		}
	}()
	// For stateless cni linux, check if delegated vmnic type, and if so, move the interface back to host network namespace using this *endpoint* struct's ifname
	if ep.NICType == cns.NodeNetworkInterfaceFrontendNIC {
		if err := client.netlink.SetLinkNetNs(ep.IfName, uintptr(vmns)); err != nil {
			wrappedErr := newErrorSecondaryEndpointClient(err)
			logger.Error("Failed to move interface", zap.String("IfName", ep.IfName), zap.Error(wrappedErr))
			return wrappedErr
		}
	}
	for iface := range ep.SecondaryInterfaces {
		if err := client.netlink.SetLinkNetNs(iface, uintptr(vmns)); err != nil {
			logger.Error("Failed to move interface", zap.String("IfName", iface), zap.Error(newErrorSecondaryEndpointClient(err)))
			continue
		}

		delete(ep.SecondaryInterfaces, iface)
	}

	return nil
}
