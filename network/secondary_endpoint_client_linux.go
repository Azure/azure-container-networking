package network

import (
	"context"
	"net"
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
)

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

func (client *SecondaryEndpointClient) AddEndpoints(epInfo *EndpointInfo) error {
	iface, err := client.netioshim.GetNetworkInterfaceByMac(epInfo.MacAddress)
	if err != nil {
		return newErrorSecondaryEndpointClient(err)
	}

	epInfo.IfName = iface.Name
	if _, exists := client.ep.SecondaryInterfaces[iface.Name]; exists {
		return newErrorSecondaryEndpointClient(errors.New(iface.Name + " already exists"))
	}

	ipconfigs := make([]*IPConfig, len(epInfo.IPAddresses))
	for i, ipconfig := range epInfo.IPAddresses {
		ipconfigs[i] = &IPConfig{Address: ipconfig}
	}

	client.ep.SecondaryInterfaces[iface.Name] = &InterfaceInfo{
		Name:              iface.Name,
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
	// we do not use the response for anything
	numSecs := 3
	timeout := time.Duration(numSecs) * time.Second
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(timeout))
	defer cancel()
	logger.Info("Sending DHCP packet", zap.Any("macAddress", epInfo.MacAddress), zap.String("ifName", epInfo.IfName))
	err := client.dhcpClient.DiscoverRequest(ctx, epInfo.MacAddress, epInfo.IfName)
	if err != nil {
		return errors.Wrap(err, NetworkNotReadyErrorMsg+" - failed to issue dhcp discover packet to create mapping in host")
	}
	logger.Info("Finished configuring container interfaces and routes for secondary endpoint client")

	return nil
}

func (client *SecondaryEndpointClient) DeleteEndpoints(ep *endpoint) error {
	return client.ExecuteInNS(ep.NetworkNameSpace, func(vmns int) error {
		// For stateless cni linux, check if delegated vmnic type, and if so, delete using this *endpoint* struct's ifname
		if ep.NICType == cns.NodeNetworkInterfaceFrontendNIC {
			if err := client.netlink.SetLinkNetNs(ep.IfName, uintptr(vmns)); err != nil {
				logger.Error("Failed to move interface", zap.String("IfName", ep.IfName), zap.Error(newErrorSecondaryEndpointClient(err)))
			}
		}

		// For Stateful cni linux, Use SecondaryInterfaces map to move all interfaces to host netns
		// TODO: SecondaryInterfaces map should be retired and only IfName field and NICType should be used to determine the delegated NIC
		for iface := range ep.SecondaryInterfaces {
			if err := client.netlink.SetLinkNetNs(iface, uintptr(vmns)); err != nil {
				logger.Error("Failed to move interface", zap.String("IfName", iface), zap.Error(newErrorSecondaryEndpointClient(err)))
				continue
			}
			delete(ep.SecondaryInterfaces, iface)
		}

		return nil
	})
}

// FetchInterfacesFromNetnsPath finds all interfaces from the specified netns path except non-eth interfaces,
// and return slice of EndpointInfo that includes interface name, NICType, netnspath and IP addresses.
func (client *SecondaryEndpointClient) FetchInterfacesFromNetnsPath(infraInterfaceName, netnspath string) ([]*EndpointInfo, error) {
	result := []*EndpointInfo{}
	err := client.ExecuteInNS(netnspath, func(vmns int) error {
		// Use the netlink API to list links
		links, err := vishnetlink.LinkList()
		if err != nil {
			return newErrorSecondaryEndpointClient(err)
		}
		ret := []*EndpointInfo{}
		for _, l := range links {
			ifname := l.Attrs().Name
			// Filter out infra interface and non-eth interfaces
			if !strings.HasPrefix(ifname, "eth") {
				continue
			}
			// Now safely get addresses
			addrs, err := vishnetlink.AddrList(l, vishnetlink.FAMILY_ALL)
			if err != nil {
				return newErrorSecondaryEndpointClient(err)
			}
			ipAddresses := make([]net.IPNet, 0, len(addrs))
			for _, addr := range addrs {
				ipAddresses = append(ipAddresses, *addr.IPNet)
				logger.Info("Found address in netns", zap.String("IfName", ifname), zap.String("Address", addr.IPNet.String()))
			}

			epInfo := &EndpointInfo{
				NetNsPath:   netnspath,
				IfName:      ifname,
				IPAddresses: ipAddresses,
			}
			if ifname == infraInterfaceName {
				epInfo.NICType = cns.InfraNIC
			} else {
				epInfo.NICType = cns.NodeNetworkInterfaceFrontendNIC
			}
			ret = append(ret, epInfo)
		}
		logger.Info("Found interfaces in netns", zap.Any("interfaces", ret), zap.Int("vmns", vmns))
		result = ret
		return nil
	})

	return result, err
}

// ExecuteInNS executes a function within the specified network namespace, handling all namespace operations.
func (client *SecondaryEndpointClient) ExecuteInNS(nsName string, f func(v int) error) error {
	// Get VM namespace
	vmns, err := netns.New().Get()
	if err != nil {
		return newErrorSecondaryEndpointClient(err)
	}

	// Open the network namespace.
	logger.Info("Opening netns", zap.Any("NetNsPath", nsName))
	ns, err := client.nsClient.OpenNamespace(nsName)
	if err != nil {
		if strings.Contains(err.Error(), errFileNotExist.Error()) {
			return nil
		}
		return newErrorSecondaryEndpointClient(err)
	}
	defer ns.Close()

	// Enter the container network namespace.
	logger.Info("Entering netns", zap.Any("NetNsPath", nsName))
	if err := ns.Enter(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return newErrorSecondaryEndpointClient(err)
	}

	// Return to host network namespace.
	defer func() {
		logger.Info("Exiting netns", zap.Any("NetNsPath", nsName))
		if exitErr := ns.Exit(); exitErr != nil {
			logger.Error("Failed to exit netns", zap.Error(newErrorSecondaryEndpointClient(exitErr)))
		}
	}()

	// Execute the provided function
	return f(vmns)
}
