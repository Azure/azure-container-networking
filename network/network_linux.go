// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/epcommon"
	"github.com/Azure/azure-container-networking/platform"
	"golang.org/x/sys/unix"
)

const (
	// Prefix for bridge names.
	bridgePrefix = "azure"
	// Virtual MAC address used by Azure VNET.
	virtualMacAddress     = "12:34:56:78:9a:bc"
	versionID             = "VERSION_ID"
	distroID              = "ID"
	ubuntuStr             = "ubuntu"
	dnsServersStr         = "DNS Servers"
	dnsDomainStr          = "DNS Domain"
	ubuntuVersion17       = 17
	defaultDnsServerIP    = "168.63.129.16"
	systemdResolvConfFile = "/run/systemd/resolve/resolv.conf"
	SnatBridgeIPKey       = "snatBridgeIP"
	LocalIPKey            = "localIP"
	InfraVnetIPKey        = "infraVnetIP"
	OptVethName           = "vethname"
)

const (
	lineDelimiter  = "\n"
	colonDelimiter = ":"
	dotDelimiter   = "."
)

// Linux implementation of route.
type route netlink.Route

// NewNetworkImpl creates a new container network.
func (nm *networkManager) newNetworkImpl(nwInfo *NetworkInfo, extIf *externalInterface) (*network, error) {
	// Connect the external interface.
	var vlanid int
	opt, _ := nwInfo.Options[genericData].(map[string]interface{})
	log.Printf("opt %+v options %+v", opt, nwInfo.Options)

	switch nwInfo.Mode {
	case opModeTunnel:
		handleCommonOptions(extIf.Name, nwInfo)
		fallthrough
	case opModeBridge:
		log.Printf("create bridge")
		if err := nm.connectExternalInterface(extIf, nwInfo); err != nil {
			return nil, err
		}

		if opt != nil && opt[VlanIDKey] != nil {
			vlanid, _ = strconv.Atoi(opt[VlanIDKey].(string))
		}
		handleCommonOptions(extIf.BridgeName, nwInfo)
	case opModeTransparent:
		handleCommonOptions(extIf.Name, nwInfo)
	default:
		return nil, errNetworkModeInvalid
	}

	// Create the network object.
	nw := &network{
		Id:               nwInfo.Id,
		Mode:             nwInfo.Mode,
		Endpoints:        make(map[string]*endpoint),
		extIf:            extIf,
		VlanId:           vlanid,
		DNS:              nwInfo.DNS,
		EnableSnatOnHost: nwInfo.EnableSnatOnHost,
	}

	return nw, nil
}

func handleCommonOptions(ifname string, nwInfo *NetworkInfo) error {
	var err error
	if routes, exists := nwInfo.Options[RoutesKey]; exists {
		err = AddRoutes(ifname, routes.([]RouteInfo))
		if err != nil {
			return err
		}
	}

	if iptcmds, exists := nwInfo.Options[IPTablesKey]; exists {
		err = AddToIptables(iptcmds.([]iptables.IPTableEntry))
		if err != nil {
			return err
		}
	}

	return nil
}

// DeleteNetworkImpl deletes an existing container network.
func (nm *networkManager) deleteNetworkImpl(nw *network) error {
	var networkClient NetworkClient

	if nw.VlanId != 0 {
		networkClient = NewOVSClient(nw.extIf.BridgeName, nw.extIf.Name)
	} else {
		networkClient = NewLinuxBridgeClient(nw.extIf.BridgeName, nw.extIf.Name, NetworkInfo{})
	}

	// Disconnect the interface if this was the last network using it.
	if len(nw.extIf.Networks) == 1 {
		nm.disconnectExternalInterface(nw.extIf, networkClient)
	}

	return nil
}

//  SaveIPConfig saves the IP configuration of an interface.
func (nm *networkManager) saveIPConfig(hostIf *net.Interface, extIf *externalInterface) error {
	// Save the default routes on the interface.
	routes, err := netlink.GetIpRoute(&netlink.Route{Dst: &net.IPNet{}, LinkIndex: hostIf.Index})
	if err != nil {
		log.Printf("[net] Failed to query routes: %v.", err)
		return err
	}

	for _, r := range routes {
		if r.Dst == nil {
			if r.Family == unix.AF_INET {
				extIf.IPv4Gateway = r.Gw
			} else if r.Family == unix.AF_INET6 {
				extIf.IPv6Gateway = r.Gw
			}
		}

		extIf.Routes = append(extIf.Routes, (*route)(r))
	}

	// Save global unicast IP addresses on the interface.
	addrs, err := hostIf.Addrs()
	for _, addr := range addrs {
		ipAddr, ipNet, err := net.ParseCIDR(addr.String())
		ipNet.IP = ipAddr
		if err != nil {
			continue
		}

		if !ipAddr.IsGlobalUnicast() {
			continue
		}

		extIf.IPAddresses = append(extIf.IPAddresses, ipNet)

		log.Printf("[net] Deleting IP address %v from interface %v.", ipNet, hostIf.Name)

		err = netlink.DeleteIpAddress(hostIf.Name, ipAddr, ipNet)
		if err != nil {
			break
		}
	}

	log.Printf("[net] Saved interface IP configuration %+v.", extIf)

	return err
}

func getMajorVersion(version string) (int, error) {
	versionSplit := strings.Split(version, dotDelimiter)
	if len(versionSplit) > 0 {
		retrieved_version, err := strconv.Atoi(versionSplit[0])
		if err != nil {
			return 0, err
		}

		return retrieved_version, err
	}

	return 0, fmt.Errorf("[net] Error getting major version")
}

func isGreaterOrEqaulUbuntuVersion(versionToMatch int) bool {
	osInfo, err := platform.GetOSDetails()
	if err != nil {
		log.Printf("[net] Unable to get OS Details: %v", err)
		return false
	}

	log.Printf("[net] OSInfo: %+v", osInfo)

	version := osInfo[versionID]
	distro := osInfo[distroID]

	if strings.EqualFold(distro, ubuntuStr) {
		version = strings.Trim(version, "\"")
		retrieved_version, err := getMajorVersion(version)
		if err != nil {
			log.Printf("[net] Not setting dns. Unable to retrieve major version: %v", err)
			return false
		}

		if retrieved_version >= versionToMatch {
			return true
		}
	}

	return false
}

func readDnsInfo(ifName string) (DNSInfo, error) {
	var dnsInfo DNSInfo

	cmd := fmt.Sprintf("systemd-resolve --status %s", ifName)
	out, err := platform.ExecuteCommand(cmd)
	if err != nil {
		return dnsInfo, err
	}

	log.Printf("[net] console output for above cmd: %s", out)

	lineArr := strings.Split(out, lineDelimiter)
	if len(lineArr) <= 0 {
		return dnsInfo, fmt.Errorf("[net] Console output doesn't have any lines")
	}

	dnsServerFound := false
	for _, line := range lineArr {
		if strings.Contains(line, dnsServersStr) {
			dnsServerSplit := strings.Split(line, colonDelimiter)
			if len(dnsServerSplit) > 1 {
				dnsServerFound = true
				dnsServerSplit[1] = strings.TrimSpace(dnsServerSplit[1])
				dnsInfo.Servers = append(dnsInfo.Servers, dnsServerSplit[1])
			}
		} else if !strings.Contains(line, colonDelimiter) && dnsServerFound {
			dnsServer := strings.TrimSpace(line)
			dnsInfo.Servers = append(dnsInfo.Servers, dnsServer)
		} else {
			dnsServerFound = false
		}
	}

	for _, line := range lineArr {
		if strings.Contains(line, dnsDomainStr) {
			dnsDomainSplit := strings.Split(line, colonDelimiter)
			if len(dnsDomainSplit) > 1 {
				dnsInfo.Suffix = strings.TrimSpace(dnsDomainSplit[1])
			}
		}
	}

	return dnsInfo, nil
}

func saveDnsConfig(extIf *externalInterface) error {
	dnsInfo, err := readDnsInfo(extIf.Name)
	if err != nil || len(dnsInfo.Servers) == 0 || dnsInfo.Suffix == "" {
		log.Printf("[net] Failed to read dns info %+v from interface %v: %v", dnsInfo, extIf.Name, err)
		return err
	}

	extIf.DNSInfo = dnsInfo
	log.Printf("[net] Saved DNS Info %v from %v", extIf.DNSInfo, extIf.Name)

	return nil
}

// ApplyIPConfig applies a previously saved IP configuration to an interface.
func (nm *networkManager) applyIPConfig(extIf *externalInterface, targetIf *net.Interface) error {
	// Add IP addresses.
	for _, addr := range extIf.IPAddresses {
		log.Printf("[net] Adding IP address %v to interface %v.", addr, targetIf.Name)

		err := netlink.AddIpAddress(targetIf.Name, addr.IP, addr)
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "file exists") {
			log.Printf("[net] Failed to add IP address %v: %v.", addr, err)
			return err
		}
	}

	// Add IP routes.
	for _, route := range extIf.Routes {
		route.LinkIndex = targetIf.Index

		log.Printf("[net] Adding IP route %+v.", route)

		err := netlink.AddIpRoute((*netlink.Route)(route))
		if err != nil {
			log.Printf("[net] Failed to add IP route %v: %v.", route, err)
			return err
		}
	}

	return nil
}

func applyDnsConfig(extIf *externalInterface, ifName string) error {
	var (
		setDnsList string
		err        error
	)

	if extIf != nil {
		for _, server := range extIf.DNSInfo.Servers {
			if net.ParseIP(server).To4() == nil {
				log.Errorf("[net] Invalid dns ip %s.", server)
				continue
			}

			buf := fmt.Sprintf("--set-dns=%s", server)
			setDnsList = setDnsList + " " + buf
		}

		if setDnsList != "" {
			cmd := fmt.Sprintf("systemd-resolve --interface=%s%s", ifName, setDnsList)
			_, err = platform.ExecuteCommand(cmd)
			if err != nil {
				return err
			}
		}

		if extIf.DNSInfo.Suffix != "" {
			cmd := fmt.Sprintf("systemd-resolve --interface=%s --set-domain=%s", ifName, extIf.DNSInfo.Suffix)
			_, err = platform.ExecuteCommand(cmd)
		}

	}

	return err
}

// ConnectExternalInterface connects the given host interface to a bridge.
func (nm *networkManager) connectExternalInterface(extIf *externalInterface, nwInfo *NetworkInfo) error {
	var (
		err           error
		networkClient NetworkClient
	)

	log.Printf("[net] Connecting interface %v.", extIf.Name)
	defer func() { log.Printf("[net] Connecting interface %v completed with err:%v.", extIf.Name, err) }()

	// Check whether this interface is already connected.
	if extIf.BridgeName != "" {
		log.Printf("[net] Interface is already connected to bridge %v.", extIf.BridgeName)
		return nil
	}

	// Find the external interface.
	hostIf, err := net.InterfaceByName(extIf.Name)
	if err != nil {
		return err
	}

	// If a bridge name is not specified, generate one based on the external interface index.
	bridgeName := nwInfo.BridgeName
	if bridgeName == "" {
		bridgeName = fmt.Sprintf("%s%d", bridgePrefix, hostIf.Index)
	}

	opt, _ := nwInfo.Options[genericData].(map[string]interface{})
	if opt != nil && opt[VlanIDKey] != nil {
		networkClient = NewOVSClient(bridgeName, extIf.Name)
	} else {
		networkClient = NewLinuxBridgeClient(bridgeName, extIf.Name, *nwInfo)
	}

	// Check if the bridge already exists.
	bridge, err := net.InterfaceByName(bridgeName)

	if err != nil {
		// Create the bridge.
		if err = networkClient.CreateBridge(); err != nil {
			log.Printf("Error while creating bridge %+v", err)
			return err
		}

		bridge, err = net.InterfaceByName(bridgeName)
		if err != nil {
			return err
		}
	} else {
		// Use the existing bridge.
		log.Printf("[net] Found existing bridge %v.", bridgeName)
	}

	defer func() {
		if err != nil {
			log.Printf("[net] cleanup network")
			nm.disconnectExternalInterface(extIf, networkClient)
		}
	}()

	// Save host IP configuration.
	err = nm.saveIPConfig(hostIf, extIf)
	if err != nil {
		log.Printf("[net] Failed to save IP configuration for interface %v: %v.", hostIf.Name, err)
	}

	/*
		If custom dns server is updated, VM needs reboot for the change to take effect.
	*/
	isGreaterOrEqualUbuntu17 := isGreaterOrEqaulUbuntuVersion(ubuntuVersion17)
	if isGreaterOrEqualUbuntu17 {
		log.Printf("[net] Saving dns config from %v", extIf.Name)
		if err = saveDnsConfig(extIf); err != nil {
			log.Printf("[net] Failed to save dns config: %v", err)
			return err
		}
	}

	// External interface down.
	log.Printf("[net] Setting link %v state down.", hostIf.Name)
	err = netlink.SetLinkState(hostIf.Name, false)
	if err != nil {
		return err
	}

	// Connect the external interface to the bridge.
	log.Printf("[net] Setting link %v master %v.", hostIf.Name, bridgeName)
	if err = networkClient.SetBridgeMasterToHostInterface(); err != nil {
		return err
	}

	// External interface up.
	log.Printf("[net] Setting link %v state up.", hostIf.Name)
	err = netlink.SetLinkState(hostIf.Name, true)
	if err != nil {
		return err
	}

	// Bridge up.
	log.Printf("[net] Setting link %v state up.", bridgeName)
	err = netlink.SetLinkState(bridgeName, true)
	if err != nil {
		return err
	}

	// Add the bridge rules.
	err = networkClient.AddL2Rules(extIf)
	if err != nil {
		return err
	}

	// External interface hairpin on.
	if !nwInfo.DisableHairpinOnHostInterface {
		log.Printf("[net] Setting link %v hairpin on.", hostIf.Name)
		if err = networkClient.SetHairpinOnHostInterface(true); err != nil {
			return err
		}
	}

	// Apply IP configuration to the bridge for host traffic.
	err = nm.applyIPConfig(extIf, bridge)
	if err != nil {
		log.Printf("[net] Failed to apply interface IP configuration: %v.", err)
		return err
	}

	if isGreaterOrEqualUbuntu17 {
		log.Printf("[net] Applying dns config on %v", bridgeName)

		if err = applyDnsConfig(extIf, bridgeName); err != nil {
			log.Printf("[net] Failed to apply DNS configuration: %v.", err)
			return err
		}

		log.Printf("[net] Applied dns config %v on %v", extIf.DNSInfo, bridgeName)
	}

	if nwInfo.IPV6Mode == IPV6Nat {
		// adds pod cidr gateway ip to bridge
		if err = addIpv6NatGateway(nwInfo); err != nil {
			log.Errorf("[net] Adding IPv6 Nat Gateway failed:%v", err)
			return err
		}

		if err = addIpv6SnatRule(extIf, nwInfo); err != nil {
			log.Errorf("[net] Adding IPv6 Snat Rule failed:%v", err)
			return err
		}

		// unmark packet if set by kube-proxy to skip kube-postrouting rule and processed
		// by cni snat rule
		if err = iptables.InsertIptableRule(iptables.V6, iptables.Mangle, iptables.Postrouting, "", "MARK --set-mark 0x0"); err != nil {
			log.Errorf("[net] Adding Iptable mangle rule failed:%v", err)
			return err
		}
	}

	extIf.BridgeName = bridgeName
	log.Printf("[net] Connected interface %v to bridge %v.", extIf.Name, extIf.BridgeName)

	return nil
}

// DisconnectExternalInterface disconnects a host interface from its bridge.
func (nm *networkManager) disconnectExternalInterface(extIf *externalInterface, networkClient NetworkClient) {
	log.Printf("[net] Disconnecting interface %v.", extIf.Name)

	log.Printf("[net] Deleting bridge rules")
	// Delete bridge rules set on the external interface.
	networkClient.DeleteL2Rules(extIf)

	log.Printf("[net] Deleting bridge")
	// Delete Bridge
	networkClient.DeleteBridge()

	extIf.BridgeName = ""
	log.Printf("Restoring ipconfig with primary interface %v", extIf.Name)

	// Restore IP configuration.
	hostIf, _ := net.InterfaceByName(extIf.Name)
	err := nm.applyIPConfig(extIf, hostIf)
	if err != nil {
		log.Printf("[net] Failed to apply IP configuration: %v.", err)
	}

	extIf.IPAddresses = nil
	extIf.Routes = nil

	log.Printf("[net] Disconnected interface %v.", extIf.Name)
}

func AddToIptables(cmds []iptables.IPTableEntry) error {
	log.Printf("Adding additional iptable rules...")
	for _, cmd := range cmds {
		err := iptables.RunCmd(cmd.Version, cmd.Params)
		if err != nil {
			return err
		}
		log.Printf("Succesfully run iptables rule %v", cmd)
	}
	return nil
}

func AddRoutes(bridgeName string, routes []RouteInfo) error {
	log.Printf("Adding routes...")
	for _, route := range routes {
		route.DevName = bridgeName
		devIf, _ := net.InterfaceByName(route.DevName)
		ifIndex := devIf.Index
		gwfamily := netlink.GetIpAddressFamily(route.Gw)

		nlRoute := &netlink.Route{
			Family:    gwfamily,
			Dst:       &route.Dst,
			Gw:        route.Gw,
			LinkIndex: ifIndex,
		}

		if err := netlink.AddIpRoute(nlRoute); err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "file exists") {
				return fmt.Errorf("Failed to add %+v to host interface with error: %v", nlRoute, err)
			}
			log.Printf("[cni-net] route already exists: dst %+v, gw %+v, interfaceName %v", nlRoute.Dst, nlRoute.Gw, route.DevName)
		}

		log.Printf("[cni-net] Added route %+v", route)
	}

	return nil
}

// Add ipv6 nat gateway IP on bridge
func addIpv6NatGateway(nwInfo *NetworkInfo) error {
	log.Printf("[net] Adding ipv6 nat gateway on azure bridge")
	for _, subnetInfo := range nwInfo.Subnets {
		if subnetInfo.Family == platform.AfINET6 {
			ipAddr := []net.IPNet{{
				IP:   subnetInfo.Gateway,
				Mask: subnetInfo.Prefix.Mask,
			}}
			return epcommon.AssignIPToInterface(nwInfo.BridgeName, ipAddr)
		}
	}

	return nil
}

// snat ipv6 traffic to secondary ipv6 ip before leaving VM
func addIpv6SnatRule(extIf *externalInterface, nwInfo *NetworkInfo) error {
	log.Printf("[net] Adding ipv6 snat rule")
	for _, ipAddr := range extIf.IPAddresses {
		if ipAddr.IP.To4() == nil {
			if err := epcommon.AddSnatRule("", ipAddr.IP); err != nil {
				return err
			}
		}
	}

	return nil
}

func getNetworkInfoImpl(nwInfo *NetworkInfo, nw *network) {
	if nw.VlanId != 0 {
		vlanMap := make(map[string]interface{})
		vlanMap[VlanIDKey] = strconv.Itoa(nw.VlanId)
		nwInfo.Options[genericData] = vlanMap
	}
}

func AddStaticRoute(ip string, interfaceName string) error {
	log.Printf("[ovs] Adding %v static route", ip)
	var routes []RouteInfo
	_, ipNet, _ := net.ParseCIDR(ip)
	gwIP := net.ParseIP("0.0.0.0")
	route := RouteInfo{Dst: *ipNet, Gw: gwIP}
	routes = append(routes, route)
	if err := addRoutes(interfaceName, routes); err != nil {
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "file exists") {
			log.Printf("addroutes failed with error %v", err)
			return err
		}
	}

	return nil
}
