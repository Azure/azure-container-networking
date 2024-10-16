package network

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/netns"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/network/snat"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/pkg/errors"
	vishnetlink "github.com/vishvananda/netlink"
	"go.uber.org/zap"
)

const (
	virtualGwIPVlanString = "169.254.2.1/32"
	azureMac              = "12:34:56:78:9a:bc"                       // Packets leaving the VM should have this MAC
	loopbackIf            = "lo"                                      // The name of the loopback interface
	numDefaultRoutes      = 2                                         // VNET NS, when no containers use it, has this many routes
	tunnelingTable        = 2                                         // Packets not entering on the vlan interface go to this routing table
	tunnelingMark         = 333                                       // The packets that are to tunnel will be marked with this number
	DisableRPFilterCmd    = "sysctl -w net.ipv4.conf.all.rp_filter=0" // Command to disable the rp filter for tunneling
	numRetries            = 5
	sleepInMs             = 100
)

var errNamespaceCreation = fmt.Errorf("network namespace creation error")

type netnsClient interface {
	Get() (fileDescriptor int, err error)
	GetFromName(name string) (fileDescriptor int, err error)
	Set(fileDescriptor int) (err error)
	NewNamed(name string) (fileDescriptor int, err error)
	DeleteNamed(name string) (err error)
	IsNamespaceEqual(fd1, fd2 int) bool
	NamespaceUniqueID(fd int) string
}
type TransparentVlanEndpointClient struct {
	primaryHostIfName string // So like eth0
	vlanIfName        string // So like eth0.1
	vnetVethName      string // Peer is containerVethName
	containerVethName string // Peer is vnetVethName

	vnetMac        net.HardwareAddr
	containerMac   net.HardwareAddr
	hostPrimaryMac net.HardwareAddr

	vnetNSName           string
	vnetNSFileDescriptor int

	snatClient               snat.Client
	vlanID                   int
	enableSnatOnHost         bool
	allowInboundFromHostToNC bool
	allowInboundFromNCToHost bool
	enableSnatForDNS         bool
	netnsClient              netnsClient
	netlink                  netlink.NetlinkInterface
	netioshim                netio.NetIOInterface
	plClient                 platform.ExecClient
	netUtilsClient           networkutils.NetworkUtils
	nsClient                 NamespaceClientInterface
	iptablesClient           ipTablesClient
}

func NewTransparentVlanEndpointClient(
	nw *network,
	ep *EndpointInfo,
	vnetVethName string,
	containerVethName string,
	vlanid int,
	localIP string,
	nl netlink.NetlinkInterface,
	plc platform.ExecClient,
	nsc NamespaceClientInterface,
	iptc ipTablesClient,
) *TransparentVlanEndpointClient {
	vlanVethName := fmt.Sprintf("%s_%d", nw.extIf.Name, vlanid)
	vnetNSName := fmt.Sprintf("az_ns_%d", vlanid)

	client := &TransparentVlanEndpointClient{
		primaryHostIfName:        nw.extIf.Name,
		vlanIfName:               vlanVethName,
		vnetVethName:             vnetVethName,
		hostPrimaryMac:           nw.extIf.MacAddress,
		containerVethName:        containerVethName,
		vnetNSName:               vnetNSName,
		vlanID:                   vlanid,
		enableSnatOnHost:         ep.EnableSnatOnHost,
		allowInboundFromHostToNC: ep.AllowInboundFromHostToNC,
		allowInboundFromNCToHost: ep.AllowInboundFromNCToHost,
		enableSnatForDNS:         ep.EnableSnatForDns,
		netnsClient:              netns.New(),
		netlink:                  nl,
		netioshim:                &netio.NetIO{},
		plClient:                 plc,
		netUtilsClient:           networkutils.NewNetworkUtils(nl, plc),
		nsClient:                 nsc,
		iptablesClient:           iptc,
	}

	client.NewSnatClient(nw.SnatBridgeIP, localIP, ep)

	return client
}

// Adds interfaces to the vnet (created if not existing) and vm namespace
func (client *TransparentVlanEndpointClient) AddEndpoints(epInfo *EndpointInfo) error {
	// VM Namespace
	if err := client.ensureCleanPopulateVM(); err != nil {
		return errors.Wrap(err, "failed to ensure both network namespace and vlan veth were present or both absent")
	}
	if err := client.PopulateVM(epInfo); err != nil {
		return err
	}
	if err := client.AddSnatEndpoint(); err != nil {
		return errors.Wrap(err, "failed to add snat endpoint")
	}
	// VNET Namespace
	return ExecuteInNS(client.nsClient, client.vnetNSName, func() error {
		return client.PopulateVnet(epInfo)
	})
}

// Called from AddEndpoints, Namespace: VM and Vnet
func (client *TransparentVlanEndpointClient) ensureCleanPopulateVM() error {
	// Clean up vlan interface in the VM namespace and ensure the network namespace (if it exists) has a vlan interface
	logger.Info("Checking if NS and vlan veth exists...")
	var existingErr error
	client.vnetNSFileDescriptor, existingErr = client.netnsClient.GetFromName(client.vnetNSName)
	if existingErr == nil {
		// The namespace exists
		vlanIfErr := ExecuteInNS(client.nsClient, client.vnetNSName, func() error {
			// Ensure the vlan interface exists in the namespace
			_, err := client.netioshim.GetNetworkInterfaceByName(client.vlanIfName)
			return errors.Wrap(err, "failed to get vlan interface in namespace")
		})
		if vlanIfErr != nil {
			// Assume any error is the vlan interface not found
			logger.Info("vlan interface doesn't exist even though network namespace exists, deleting network namespace...", zap.String("message", vlanIfErr.Error()))
			delErr := client.netnsClient.DeleteNamed(client.vnetNSName)
			if delErr != nil {
				return errors.Wrap(delErr, "failed to cleanup/delete ns after noticing vlan veth does not exist")
			}
		}
	}
	// Delete the vlan interface in the VM namespace if it exists
	_, vlanIfInVMErr := client.netioshim.GetNetworkInterfaceByName(client.vlanIfName)
	if vlanIfInVMErr == nil {
		// The vlan interface exists in the VM ns because it failed to move into the network ns previously and needs to be cleaned up
		logger.Info("vlan interface exists on the VM namespace, deleting", zap.String("vlanIfName", client.vlanIfName))
		if delErr := client.netlink.DeleteLink(client.vlanIfName); delErr != nil {
			return errors.Wrap(delErr, "failed to clean up vlan interface")
		}
	}
	return nil
}

// Called from PopulateVM, Namespace: VM
func (client *TransparentVlanEndpointClient) createNetworkNamespace(vmNS int) error {
	// If this call succeeds, the namespace is set to the new namespace
	vnetNS, err := client.netnsClient.NewNamed(client.vnetNSName)
	if err != nil {
		return errors.Wrap(err, "failed to create vnet ns")
	}
	logger.Info("Vnet Namespace created", zap.String("vnetNS", client.netnsClient.NamespaceUniqueID(vnetNS)), zap.String("vmNS", client.netnsClient.NamespaceUniqueID(vmNS)))
	if !client.netnsClient.IsNamespaceEqual(vnetNS, vmNS) {
		client.vnetNSFileDescriptor = vnetNS
		return nil
	}
	// the vnet and vm namespace are the same by this point
	logger.Info("Vnet Namespace is the same as VM namespace. Deleting...")
	delErr := client.netnsClient.DeleteNamed(client.vnetNSName)
	if delErr != nil {
		logger.Error("failed to cleanup/delete ns after noticing vnet ns is the same as vm ns", zap.Any("error:", delErr.Error()))
	}
	return errors.Wrap(errNamespaceCreation, "vnet and vm namespace are the same")
}

// Called from PopulateVM, Namespace: VM and namespace represented by fd
func (client *TransparentVlanEndpointClient) setLinkNetNSAndConfirm(name string, fd uintptr) error {
	logger.Info("Move link to NS", zap.String("ifName", name), zap.Any("NSFileDescriptor", fd))
	err := client.netlink.SetLinkNetNs(name, fd)
	if err != nil {
		return errors.Wrapf(err, "failed to set %v inside namespace %v", name, fd)
	}

	// confirm veth was moved successfully
	err = RunWithRetries(func() error {
		// retry checking in the namespace if the interface is not detected
		return ExecuteInNS(client.nsClient, client.vnetNSName, func() error {
			_, ifDetectedErr := client.netioshim.GetNetworkInterfaceByName(client.vlanIfName)
			return errors.Wrap(ifDetectedErr, "failed to get vlan veth in namespace")
		})
	}, numRetries, sleepInMs)
	if err != nil {
		return errors.Wrapf(err, "failed to detect %v inside namespace %v", name, fd)
	}
	return nil
}

// Called from AddEndpoints, Namespace: VM
func (client *TransparentVlanEndpointClient) PopulateVM(epInfo *EndpointInfo) error {
	vmNS, err := client.netnsClient.Get()
	if err != nil {
		return errors.Wrap(err, "failed to get vm ns handle")
	}

	var existingErr error
	client.vnetNSFileDescriptor, existingErr = client.netnsClient.GetFromName(client.vnetNSName)
	// If the ns does not exist, the below code will trigger to create it
	// This will also (we assume) mean the vlan veth does not exist
	if existingErr != nil {
		// We assume the only possible error is that the namespace doesn't exist
		logger.Info("No existing NS detected. Creating the vnet namespace and switching to it", zap.String("message", existingErr.Error()))

		err = RunWithRetries(func() error {
			return client.createNetworkNamespace(vmNS)
		}, numRetries, sleepInMs)
		if err != nil {
			return errors.Wrap(err, "failed to create network namespace")
		}

		deleteNSIfNotNilErr := client.netnsClient.Set(vmNS)
		// Any failure will trigger removing the namespace created
		defer func() {
			if deleteNSIfNotNilErr != nil {
				logger.Info("[transparent vlan] removing vnet ns due to failure...")
				err = client.netnsClient.DeleteNamed(client.vnetNSName)
				if err != nil {
					logger.Error("failed to cleanup/delete ns after failing to create vlan veth")
				}
			}
		}()
		if deleteNSIfNotNilErr != nil {
			return errors.Wrap(deleteNSIfNotNilErr, "failed to set current ns to vm")
		}

		// Now create vlan veth
		logger.Info("Create the host vlan link after getting eth0", zap.String("primaryHostIfName", client.primaryHostIfName))
		// Get parent interface index. Index is consistent across libraries.
		eth0, deleteNSIfNotNilErr := client.netioshim.GetNetworkInterfaceByName(client.primaryHostIfName)
		if deleteNSIfNotNilErr != nil {
			return errors.Wrap(deleteNSIfNotNilErr, "failed to get eth0 interface")
		}
		linkAttrs := vishnetlink.NewLinkAttrs()
		linkAttrs.Name = client.vlanIfName
		// Set the peer
		linkAttrs.ParentIndex = eth0.Index
		link := &vishnetlink.Vlan{
			LinkAttrs: linkAttrs,
			VlanId:    client.vlanID,
		}
		logger.Info("Attempting to create link in VM NS", zap.String("vlanIfName", client.vlanIfName))
		// Create vlan veth
		deleteNSIfNotNilErr = vishnetlink.LinkAdd(link)
		if deleteNSIfNotNilErr != nil {
			// ignore link already exists error
			if !strings.Contains(deleteNSIfNotNilErr.Error(), "file exists") {
				// Any failure to add the link should error (auto delete NS)
				return errors.Wrap(deleteNSIfNotNilErr, "failed to create vlan vnet link after making new ns")
			}
			// Prevent accidentally deleting NS and vlan interface since we ignore this error
			deleteNSIfNotNilErr = nil
		}
		defer func() {
			if deleteNSIfNotNilErr != nil {
				logger.Info("removing vlan veth due to failure...")
				if delErr := client.netlink.DeleteLink(client.vlanIfName); delErr != nil {
					logger.Error("deleting vlan veth failed on addendpoint failure with", zap.Any("error:", delErr.Error()))
				}
			}
		}()

		// sometimes there is slight delay in interface creation. check if it exists
		err = RunWithRetries(func() error {
			_, err = client.netioshim.GetNetworkInterfaceByName(client.vlanIfName)
			return errors.Wrap(err, "failed to get vlan veth")
		}, numRetries, sleepInMs)

		if err != nil {
			deleteNSIfNotNilErr = errors.Wrapf(err, "failed to get vlan veth interface:%s", client.vlanIfName)
			return deleteNSIfNotNilErr
		}

		deleteNSIfNotNilErr = client.netUtilsClient.DisableRAForInterface(client.vlanIfName)
		if deleteNSIfNotNilErr != nil {
			return errors.Wrap(deleteNSIfNotNilErr, "failed to disable router advertisements for vlan vnet link")
		}
		// vlan veth was created successfully, so move the vlan veth you created
		logger.Info("Move vlan link to vnet NS", zap.String("vlanIfName", client.vlanIfName), zap.Any("vnetNSFileDescriptor", uintptr(client.vnetNSFileDescriptor)))
		deleteNSIfNotNilErr = client.setLinkNetNSAndConfirm(client.vlanIfName, uintptr(client.vnetNSFileDescriptor))
		if deleteNSIfNotNilErr != nil {
			return errors.Wrap(deleteNSIfNotNilErr, "failed to move or detect vlan veth inside vnet namespace")
		}
		logger.Info("Moving vlan veth into namespace confirmed")
	} else {
		logger.Info("Existing NS detected. vlan interface should exist or namespace would've been deleted.", zap.String("vnetNSName", client.vnetNSName), zap.String("vlanIfName", client.vlanIfName))
	}

	// Get the default constant host veth mac
	mac, err := net.ParseMAC(defaultHostVethHwAddr)
	if err != nil {
		logger.Info("Failed to parse the mac address", zap.String("defaultHostVethHwAddr", defaultHostVethHwAddr))
	}

	// Create veth pair
	if err = client.netUtilsClient.CreateEndpoint(client.vnetVethName, client.containerVethName, mac); err != nil {
		return errors.Wrap(err, "failed to create veth pair")
	}

	// Ensure vnet veth is created, as there may be a slight delay
	err = RunWithRetries(func() error {
		_, getErr := client.netioshim.GetNetworkInterfaceByName(client.vnetVethName)
		return errors.Wrap(getErr, "failed to get vnet veth")
	}, numRetries, sleepInMs)
	if err != nil {
		return errors.Wrap(err, "vnet veth does not exist")
	}

	// Ensure container veth is created, as there may be a slight delay
	var containerIf *net.Interface
	err = RunWithRetries(func() error {
		var getErr error
		containerIf, getErr = client.netioshim.GetNetworkInterfaceByName(client.containerVethName)
		return errors.Wrap(getErr, "failed to get container veth")
	}, numRetries, sleepInMs)
	if err != nil {
		return errors.Wrap(err, "container veth does not exist")
	}

	// Disable RA for veth pair, and delete if any failure
	if err = client.netUtilsClient.DisableRAForInterface(client.vnetVethName); err != nil {
		if delErr := client.netlink.DeleteLink(client.vnetVethName); delErr != nil {
			logger.Error("Deleting vnet veth failed on addendpoint failure with", zap.Error(delErr))
		}
		return errors.Wrap(err, "failed to disable RA on vnet veth, deleting")
	}
	if err = client.netUtilsClient.DisableRAForInterface(client.containerVethName); err != nil {
		if delErr := client.netlink.DeleteLink(client.containerVethName); delErr != nil {
			logger.Error("Deleting container veth failed on addendpoint failure with", zap.Error(delErr))
		}
		return errors.Wrap(err, "failed to disable RA on container veth, deleting")
	}

	if err = client.setLinkNetNSAndConfirm(client.vnetVethName, uintptr(client.vnetNSFileDescriptor)); err != nil {
		if delErr := client.netlink.DeleteLink(client.vnetVethName); delErr != nil {
			logger.Error("Deleting vnet veth failed on addendpoint failure with", zap.Error(delErr))
		}
		return errors.Wrap(err, "failed to move or detect vnetVethName in vnet ns, deleting")
	}
	client.containerMac = containerIf.HardwareAddr
	return nil
}

// Called from AddEndpoints, Namespace: Vnet
func (client *TransparentVlanEndpointClient) PopulateVnet(epInfo *EndpointInfo) error {
	_, err := client.netioshim.GetNetworkInterfaceByName(client.vlanIfName)
	if err != nil {
		return errors.Wrap(err, "vlan veth doesn't exist")
	}
	vnetVethIf, err := client.netioshim.GetNetworkInterfaceByName(client.vnetVethName)
	if err != nil {
		return errors.Wrap(err, "vnet veth doesn't exist")
	}
	client.vnetMac = vnetVethIf.HardwareAddr
	// Disable rp filter again to allow asymmetric routing for tunneling packets
	_, err = client.plClient.ExecuteRawCommand(DisableRPFilterCmd)
	if err != nil {
		return errors.Wrap(err, "transparent vlan failed to disable rp filter in vnet")
	}
	disableRPFilterVlanIfCmd := strings.Replace(DisableRPFilterCmd, "all", client.vlanIfName, 1)
	_, err = client.plClient.ExecuteRawCommand(disableRPFilterVlanIfCmd)
	if err != nil {
		return errors.Wrap(err, "transparent vlan failed to disable rp filter vlan interface in vnet")
	}
	return nil
}

func (client *TransparentVlanEndpointClient) AddEndpointRules(epInfo *EndpointInfo) error {
	if err := client.AddSnatEndpointRules(); err != nil {
		return errors.Wrap(err, "failed to add snat endpoint rules")
	}
	logger.Info("[transparent-vlan] Adding tunneling rules in vnet namespace")
	err := ExecuteInNS(client.nsClient, client.vnetNSName, func() error {
		return client.AddVnetRules(epInfo)
	})
	return err
}

// Add rules related to tunneling the packet outside of the VM, assumes all calls are idempotent. Namespace: vnet
func (client *TransparentVlanEndpointClient) AddVnetRules(epInfo *EndpointInfo) error {
	// iptables -t mangle -I PREROUTING -j MARK --set-mark <TUNNELING MARK>
	markOption := fmt.Sprintf("MARK --set-mark %d", tunnelingMark)
	if err := client.iptablesClient.InsertIptableRule(iptables.V4, "mangle", "PREROUTING", "", markOption); err != nil {
		return errors.Wrap(err, "unable to insert iptables rule mark all packets not entering on vlan interface")
	}
	// iptables -t mangle -I PREROUTING -j ACCEPT -i <VLAN IF>
	match := fmt.Sprintf("-i %s", client.vlanIfName)
	if err := client.iptablesClient.InsertIptableRule(iptables.V4, "mangle", "PREROUTING", match, "ACCEPT"); err != nil {
		return errors.Wrap(err, "unable to insert iptables rule accept all incoming from vlan interface")
	}
	// Blocks wireserver traffic from customer vnet nic
	if err := client.netUtilsClient.BlockEgressTrafficFromContainer(client.iptablesClient, iptables.V4, networkutils.AzureDNS, iptables.TCP, iptables.HTTPPort); err != nil {
		return errors.Wrap(err, "unable to insert iptables rule to drop wireserver packets")
	}

	// Packets that are marked should go to the tunneling table
	newRule := vishnetlink.NewRule()
	newRule.Mark = tunnelingMark
	newRule.Table = tunnelingTable
	rules, err := vishnetlink.RuleList(vishnetlink.FAMILY_V4)
	if err != nil {
		return errors.Wrap(err, "unable to get existing ip rule list")
	}
	// Check if rule exists already
	ruleExists := false
	for index := range rules {
		if rules[index].Mark == newRule.Mark {
			ruleExists = true
		}
	}
	if !ruleExists {
		if err := vishnetlink.RuleAdd(newRule); err != nil {
			return errors.Wrap(err, "failed to add rule that forwards packet with mark to tunneling routing table")
		}
	}

	return nil
}

func (client *TransparentVlanEndpointClient) DeleteEndpointRules(ep *endpoint) {
	client.DeleteSnatEndpointRules()
}

func (client *TransparentVlanEndpointClient) MoveEndpointsToContainerNS(epInfo *EndpointInfo, nsID uintptr) error {
	if err := client.netlink.SetLinkNetNs(client.containerVethName, nsID); err != nil {
		return errors.Wrap(err, "failed to move endpoint to container ns")
	}
	if err := client.MoveSnatEndpointToContainerNS(epInfo.NetNsPath, nsID); err != nil {
		return errors.Wrap(err, "failed to move snat endpoint to container ns")
	}
	return nil
}

func (client *TransparentVlanEndpointClient) SetupContainerInterfaces(epInfo *EndpointInfo) error {
	if err := client.netUtilsClient.SetupContainerInterface(client.containerVethName, epInfo.IfName); err != nil {
		return errors.Wrap(err, "failed to setup container interface")
	}
	client.containerVethName = epInfo.IfName

	if err := client.SetupSnatContainerInterface(); err != nil {
		return errors.Wrap(err, "failed to setup snat container interface")
	}
	return nil
}

// Adds routes, arp entries, etc. to the vnet and container namespaces
func (client *TransparentVlanEndpointClient) ConfigureContainerInterfacesAndRoutes(epInfo *EndpointInfo) error {
	// Container NS
	err := client.ConfigureContainerInterfacesAndRoutesImpl(epInfo)
	if err != nil {
		return err
	}

	// Switch to vnet NS and call ConfigureVnetInterfacesAndRoutes
	err = ExecuteInNS(client.nsClient, client.vnetNSName, func() error {
		return client.ConfigureVnetInterfacesAndRoutesImpl(epInfo)
	})
	if err != nil {
		return err
	}

	// Container NS
	if err = client.ConfigureSnatContainerInterface(); err != nil {
		return errors.Wrap(err, "failed to configure snat container interface")
	}
	return nil
}

// Called from ConfigureContainerInterfacesAndRoutes, Namespace: Container
func (client *TransparentVlanEndpointClient) ConfigureContainerInterfacesAndRoutesImpl(epInfo *EndpointInfo) error {
	if err := client.netUtilsClient.AssignIPToInterface(client.containerVethName, epInfo.IPAddresses); err != nil {
		return errors.Wrap(err, "failed to assign ips to container veth interface")
	}
	// kernel subnet route auto added by above call must be removed
	for _, ipAddr := range epInfo.IPAddresses {
		_, ipnet, _ := net.ParseCIDR(ipAddr.String())
		routeInfo := RouteInfo{
			Dst:      *ipnet,
			Scope:    netlink.RT_SCOPE_LINK,
			Protocol: netlink.RTPROT_KERNEL,
		}
		if err := deleteRoutes(client.netlink, client.netioshim, client.containerVethName, []RouteInfo{routeInfo}); err != nil {
			return errors.Wrap(err, "failed to remove kernel subnet route")
		}
	}

	if err := client.addDefaultRoutes(client.containerVethName, 0); err != nil {
		return errors.Wrap(err, "failed container ns add default routes")
	}
	if err := client.AddDefaultArp(client.containerVethName, client.vnetMac.String()); err != nil {
		return errors.Wrap(err, "failed container ns add default arp")
	}
	return nil
}

// Called from ConfigureContainerInterfacesAndRoutes, Namespace: Vnet
func (client *TransparentVlanEndpointClient) ConfigureVnetInterfacesAndRoutesImpl(epInfo *EndpointInfo) error {
	err := client.netlink.SetLinkState(loopbackIf, true)
	if err != nil {
		return errors.Wrap(err, "failed to set loopback link state to up")
	}

	// Add route specifying which device the pod ip(s) are on
	routeInfoList := client.GetVnetRoutes(epInfo.IPAddresses)
	if err = client.addDefaultRoutes(client.vlanIfName, 0); err != nil {
		return errors.Wrap(err, "failed vnet ns add default/gateway routes (idempotent)")
	}
	if err = client.AddDefaultArp(client.vlanIfName, azureMac); err != nil {
		return errors.Wrap(err, "failed vnet ns add default arp entry (idempotent)")
	}

	// Delete old route if any for this IP
	err = deleteRoutes(client.netlink, client.netioshim, "", routeInfoList)
	logger.Info("[transparent-vlan] Deleting old routes returned", zap.Error(err))

	if err = addRoutes(client.netlink, client.netioshim, client.vnetVethName, routeInfoList); err != nil {
		return errors.Wrap(err, "failed adding routes to vnet specific to this container")
	}
	if err = client.addDefaultRoutes(client.vlanIfName, tunnelingTable); err != nil {
		return errors.Wrap(err, "failed vnet ns add outbound routing table routes for tunneling (idempotent)")
	}
	// Return to ConfigureContainerInterfacesAndRoutes
	return err
}

// Helper that gets the routes in the vnet NS for a particular list of IP addresses
// Example: 192.168.0.4 dev <device which connects to NS with that IP> proto static
func (client *TransparentVlanEndpointClient) GetVnetRoutes(ipAddresses []net.IPNet) []RouteInfo {
	routeInfoList := make([]RouteInfo, 0, len(ipAddresses))
	// Add route specifying which device the pod ip(s) are on
	for _, ipAddr := range ipAddresses {
		var (
			routeInfo RouteInfo
			ipNet     net.IPNet
		)

		if ipAddr.IP.To4() != nil {
			ipNet = net.IPNet{IP: ipAddr.IP, Mask: net.CIDRMask(ipv4FullMask, ipv4Bits)}
		} else {
			ipNet = net.IPNet{IP: ipAddr.IP, Mask: net.CIDRMask(ipv6FullMask, ipv6Bits)}
		}
		logger.Info("Getting route for this", zap.String("ip", ipNet.String()))
		routeInfo.Dst = ipNet
		routeInfoList = append(routeInfoList, routeInfo)

	}
	return routeInfoList
}

// Helper that creates routing rules for the current NS which direct packets
// to the virtual gateway ip on linkToName device interface
// Route 1: 169.254.2.1 dev <linkToName>
// Route 2: default via 169.254.2.1 dev <linkToName>
func (client *TransparentVlanEndpointClient) addDefaultRoutes(linkToName string, table int) error {
	// Add route for virtualgwip (ip route add 169.254.2.1/32 dev eth0)
	virtualGwIP, virtualGwNet, _ := net.ParseCIDR(virtualGwIPVlanString)
	routeInfo := RouteInfo{
		Dst:   *virtualGwNet,
		Scope: netlink.RT_SCOPE_LINK,
		Table: table,
	}
	// Difference between interface name in addRoutes and DevName: in RouteInfo?
	if err := addRoutes(client.netlink, client.netioshim, linkToName, []RouteInfo{routeInfo}); err != nil {
		return err
	}

	// Add default route (ip route add default via 169.254.2.1 dev eth0)
	_, defaultIPNet, _ := net.ParseCIDR(defaultGwCidr)
	dstIP := net.IPNet{IP: net.ParseIP(defaultGw), Mask: defaultIPNet.Mask}
	routeInfo = RouteInfo{
		Dst:   dstIP,
		Gw:    virtualGwIP,
		Table: table,
	}

	if err := addRoutes(client.netlink, client.netioshim, linkToName, []RouteInfo{routeInfo}); err != nil {
		return err
	}
	return nil
}

// Helper that creates arp entry for the current NS which maps the virtual
// gateway (169.254.2.1) to destMac on a particular interfaceName
// Example: (169.254.2.1) at 12:34:56:78:9a:bc [ether] PERM on <interfaceName>
func (client *TransparentVlanEndpointClient) AddDefaultArp(interfaceName, destMac string) error {
	_, virtualGwNet, _ := net.ParseCIDR(virtualGwIPVlanString)
	logger.Info("Adding static arp for",
		zap.String("IP", virtualGwNet.String()), zap.String("MAC", destMac))
	hardwareAddr, err := net.ParseMAC(destMac)
	if err != nil {
		return errors.Wrap(err, "unable to parse mac")
	}
	linkInfo := netlink.LinkInfo{
		Name:       interfaceName,
		IPAddr:     virtualGwNet.IP,
		MacAddress: hardwareAddr,
	}

	if err := client.netlink.SetOrRemoveLinkAddress(linkInfo, netlink.ADD, netlink.NUD_PERMANENT); err != nil {
		return fmt.Errorf("adding arp entry failed: %w", err)
	}
	return nil
}

func (client *TransparentVlanEndpointClient) DeleteEndpoints(ep *endpoint) error {
	// Vnet NS
	err := ExecuteInNS(client.nsClient, client.vnetNSName, func() error {
		// Passing in functionality to get number of routes after deletion
		getNumRoutesLeft := func() (int, error) {
			routes, err := vishnetlink.RouteList(nil, vishnetlink.FAMILY_V4)
			if err != nil {
				return 0, errors.Wrap(err, "failed to get num routes left")
			}
			return len(routes), nil
		}

		return client.DeleteEndpointsImpl(ep, getNumRoutesLeft)
	})
	if err != nil {
		return err
	}

	// VM NS
	if err := client.DeleteSnatEndpoint(); err != nil {
		return errors.Wrap(err, "failed to delete snat endpoint")
	}
	return nil
}

// getNumRoutesLeft is a function which gets the current number of routes in the namespace. Namespace: Vnet
func (client *TransparentVlanEndpointClient) DeleteEndpointsImpl(ep *endpoint, _ func() (int, error)) error {
	routeInfoList := client.GetVnetRoutes(ep.IPAddresses)
	if err := deleteRoutes(client.netlink, client.netioshim, client.vnetVethName, routeInfoList); err != nil {
		return errors.Wrap(err, "failed to remove routes")
	}

	logger.Info("Deleting host veth", zap.String("vnetVethName", client.vnetVethName))
	// Delete Host Veth
	if err := client.netlink.DeleteLink(client.vnetVethName); err != nil {
		return errors.Wrapf(err, "deleteLink for %v failed", client.vnetVethName)
	}

	// TODO: revist if this require in future.
	//nolint gocritic
	/*	if routesLeft <= numDefaultRoutes {
			// Deletes default arp, default routes, vlan veth; there are two default routes
			// so when we have <= numDefaultRoutes routes left, no containers use this namespace
			log.Printf("[transparent vlan] Deleting namespace %s as no containers occupy it", client.vnetNSName)
			delErr := client.netnsClient.DeleteNamed(client.vnetNSName)
			if delErr != nil {
				return errors.Wrap(delErr, "failed to delete namespace")
			}
		}
	*/
	return nil
}

// Helper function that allows executing a function in a VM namespace
// Does not work for process namespaces
func ExecuteInNS(nsc NamespaceClientInterface, nsName string, f func() error) error {
	// Current namespace
	returnedTo, err := nsc.GetCurrentThreadNamespace()
	if err != nil {
		logger.Error("[ExecuteInNS] Could not get NS we are in", zap.Error(err))
	} else {
		logger.Info("[ExecuteInNS] In NS before switch", zap.String("fileName", returnedTo.GetName()))
	}

	// Open the network namespace
	logger.Info("[ExecuteInNS] Opening ns", zap.String("nsName", fmt.Sprintf("/var/run/netns/%s", nsName)))
	ns, err := nsc.OpenNamespace(fmt.Sprintf("/var/run/netns/%s", nsName))
	if err != nil {
		return err
	}
	defer ns.Close()
	// Enter the network namespace
	logger.Info("[ExecuteInNS] Entering ns", zap.String("nsFileName", ns.GetName()))
	if err := ns.Enter(); err != nil {
		return err
	}

	// Exit network namespace
	defer func() {
		logger.Info("[ExecuteInNS] Exiting ns", zap.String("nsFileName", ns.GetName()))
		if err := ns.Exit(); err != nil {
			logger.Error("[ExecuteInNS] Could not exit ns", zap.Error(err))
		}
		returnedTo, err := nsc.GetCurrentThreadNamespace()
		if err != nil {
			logger.Error("[ExecuteInNS] Could not get NS we returned to", zap.Error(err))
		} else {
			logger.Info("[ExecuteInNS] Returned to NS", zap.String("fileName", returnedTo.GetName()))
		}
	}()
	return f()
}

func RunWithRetries(f func() error, maxRuns, sleepMs int) error {
	var err error
	for i := 0; i < maxRuns; i++ {
		err = f()
		if err == nil {
			break
		}
		logger.Info("Retrying after delay...", zap.String("error", err.Error()), zap.Int("retry", i), zap.Int("sleepMs", sleepMs))
		time.Sleep(time.Duration(sleepMs) * time.Millisecond)
	}
	return err
}
