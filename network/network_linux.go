// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/ovsctl"
	"github.com/Azure/azure-container-networking/platform"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

const (
	// Prefix for bridge names.
	bridgePrefix = "azure"
	// Virtual MAC address used by Azure VNET.
	virtualMacAddress = "12:34:56:78:9a:bc"
	versionID         = "VERSION_ID"
	distroID          = "ID"
	ubuntuStr         = "ubuntu"
	dnsServersStr     = "DNS Servers"
	dnsDomainStr      = "DNS Domain"
	ubuntuVersion17   = 17
	// OptVethName key for veth name option
	OptVethName = "vethname"
	// SnatBridgeIPKey key for the SNAT bridge
	SnatBridgeIPKey = "snatBridgeIP"
	// LocalIPKey key for local IP
	LocalIPKey = "localIP"
	// InfraVnetIPKey key for infra vnet
	InfraVnetIPKey = "infraVnetIP"
)

const (
	lineDelimiter  = "\n"
	colonDelimiter = ":"
	dotDelimiter   = "."
)

var errorNetworkManager = errors.New("Network_linux pkg error")

func newErrorNetworkManager(errStr string) error {
	return fmt.Errorf("%w : %s", errorNetworkManager, errStr)
}

// Linux implementation of route.
type route netlink.Route

// NewNetworkImpl creates a new container network.
func (nm *networkManager) newNetworkImpl(nwInfo *NetworkInfo, extIf *externalInterface) (*network, error) {
	// Connect the external interface.
	var (
		vlanid int
		ifName string
	)
	opt, _ := nwInfo.Options[genericData].(map[string]interface{})
	logger.Info("opt options", zap.Any("opt", opt), zap.Any("options", nwInfo.Options))

	switch nwInfo.Mode {
	case opModeTunnel:
		fallthrough
	case opModeBridge:
		logger.Info("create bridge")
		ifName = extIf.BridgeName
		if err := nm.connectExternalInterface(extIf, nwInfo); err != nil {
			return nil, err
		}

		if opt != nil && opt[VlanIDKey] != nil {
			vlanid, _ = strconv.Atoi(opt[VlanIDKey].(string))
		}
	case opModeTransparent:
		logger.Info("Transparent mode")
		ifName = extIf.Name
		if nwInfo.IPV6Mode != "" {
			nu := networkutils.NewNetworkUtils(nm.netlink, nm.plClient)
			if err := nu.EnableIPV6Forwarding(); err != nil {
				return nil, fmt.Errorf("Ipv6 forwarding failed: %w", err)
			}
		}
	case opModeTransparentVlan:
		logger.Info("Transparent vlan mode")
		ifName = extIf.Name
	default:
		return nil, errNetworkModeInvalid
	}

	err := nm.handleCommonOptions(ifName, nwInfo)
	if err != nil {
		logger.Error("handleCommonOptions failed with", zap.Error(err))
		return nil, err
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

func (nm *networkManager) handleCommonOptions(ifName string, nwInfo *NetworkInfo) error {
	var err error
	if routes, exists := nwInfo.Options[RoutesKey]; exists {
		err = addRoutes(nm.netlink, nm.netio, ifName, routes.([]RouteInfo))
		if err != nil {
			return err
		}
	}

	if iptcmds, exists := nwInfo.Options[IPTablesKey]; exists {
		err = nm.addToIptables(iptcmds.([]iptables.IPTableEntry))
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
		networkClient = NewOVSClient(nw.extIf.BridgeName, nw.extIf.Name, ovsctl.NewOvsctl(), nm.netlink, nm.plClient)
	} else {
		networkClient = NewLinuxBridgeClient(nw.extIf.BridgeName, nw.extIf.Name, NetworkInfo{}, nm.netlink, nm.plClient)
	}

	// Disconnect the interface if this was the last network using it.
	if len(nw.extIf.Networks) == 1 {
		nm.disconnectExternalInterface(nw.extIf, networkClient)
	}

	return nil
}

// SaveIPConfig saves the IP configuration of an interface.
func (nm *networkManager) saveIPConfig(hostIf *net.Interface, extIf *externalInterface) error {
	// Save the default routes on the interface.
	routes, err := nm.netlink.GetIPRoute(&netlink.Route{Dst: &net.IPNet{}, LinkIndex: hostIf.Index})
	if err != nil {
		logger.Error("Failed to query routes", zap.Error(err))
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

		logger.Info("Deleting IP address from interface", zap.Any("ipNet", ipNet), zap.String("hostInfName", hostIf.Name))

		err = nm.netlink.DeleteIPAddress(hostIf.Name, ipAddr, ipNet)
		if err != nil {
			break
		}
	}

	logger.Info("Saved interface IP configuration", zap.Any("extIf", extIf))

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

	return 0, fmt.Errorf("Error getting major version") //nolint
}

func isGreaterOrEqaulUbuntuVersion(versionToMatch int) bool {
	osInfo, err := platform.GetOSDetails()
	if err != nil {
		logger.Error("Unable to get OS Details", zap.Error(err))
		return false
	}

	logger.Info("OSInfo", zap.Any("osInfo", osInfo))

	version := osInfo[versionID]
	distro := osInfo[distroID]

	if strings.EqualFold(distro, ubuntuStr) {
		version = strings.Trim(version, "\"")
		retrieved_version, err := getMajorVersion(version)
		if err != nil {
			logger.Error("Not setting dns. Unable to retrieve major version", zap.Error(err))
			return false
		}

		if retrieved_version >= versionToMatch {
			return true
		}
	}

	return false
}

func (nm *networkManager) readDNSInfo(ifName string) (DNSInfo, error) {
	var dnsInfo DNSInfo

	cmd := fmt.Sprintf("systemd-resolve --status %s", ifName)
	out, err := nm.plClient.ExecuteCommand(cmd)
	if err != nil {
		return dnsInfo, err
	}

	logger.Info("console output for above cmd", zap.Any("out", out))

	lineArr := strings.Split(out, lineDelimiter)
	if len(lineArr) <= 0 {
		return dnsInfo, fmt.Errorf("Console output doesn't have any lines") //nolint
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

func (nm *networkManager) saveDNSConfig(extIf *externalInterface) error {
	dnsInfo, err := nm.readDNSInfo(extIf.Name)
	if err != nil || len(dnsInfo.Servers) == 0 || dnsInfo.Suffix == "" {
		logger.Info("Failed to read dns info from interface", zap.Any("dnsInfo", dnsInfo), zap.String("extIfName", extIf.Name),
			zap.Error(err))
		return err
	}

	extIf.DNSInfo = dnsInfo
	logger.Info("Saved DNS Info", zap.Any("DNSInfo", extIf.DNSInfo), zap.String("extIfName", extIf.Name))

	return nil
}

// ApplyIPConfig applies a previously saved IP configuration to an interface.
func (nm *networkManager) applyIPConfig(extIf *externalInterface, targetIf *net.Interface) error {
	// Add IP addresses.
	for _, addr := range extIf.IPAddresses {
		logger.Info("Adding IP address to interface", zap.Any("addr", addr), zap.String("Name", targetIf.Name))

		err := nm.netlink.AddIPAddress(targetIf.Name, addr.IP, addr)
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "file exists") {
			logger.Info("Failed to add IP address", zap.Any("addr", addr), zap.Error(err))
			return err
		}
	}

	// Add IP routes.
	for _, route := range extIf.Routes {
		route.LinkIndex = targetIf.Index

		logger.Info("Adding IP route", zap.Any("route", route))

		err := nm.netlink.AddIPRoute((*netlink.Route)(route))
		if err != nil {
			logger.Error("Failed to add IP route", zap.Any("route", route), zap.Error(err))
			return err
		}
	}

	return nil
}

func (nm *networkManager) applyDNSConfig(extIf *externalInterface, ifName string) error {
	var (
		setDnsList string
		err        error
	)

	if extIf != nil {
		for _, server := range extIf.DNSInfo.Servers {
			if net.ParseIP(server).To4() == nil {
				logger.Error("Invalid dns ip", zap.String("server", server))
				continue
			}

			buf := fmt.Sprintf("--set-dns=%s", server)
			setDnsList = setDnsList + " " + buf
		}

		if setDnsList != "" {
			cmd := fmt.Sprintf("systemd-resolve --interface=%s%s", ifName, setDnsList)
			_, err = nm.plClient.ExecuteCommand(cmd)
			if err != nil {
				return err
			}
		}

		if extIf.DNSInfo.Suffix != "" {
			cmd := fmt.Sprintf("systemd-resolve --interface=%s --set-domain=%s", ifName, extIf.DNSInfo.Suffix)
			_, err = nm.plClient.ExecuteCommand(cmd)
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

	logger.Info("Connecting interface", zap.String("Name", extIf.Name))
	defer func() {
		logger.Info("Connecting interface completed", zap.String("Name", extIf.Name), zap.Error(err))
	}()
	// Check whether this interface is already connected.
	if extIf.BridgeName != "" {
		logger.Info("Interface is already connected to bridge", zap.String("BridgeName", extIf.BridgeName))
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
		networkClient = NewOVSClient(bridgeName, extIf.Name, ovsctl.NewOvsctl(), nm.netlink, nm.plClient)
	} else {
		networkClient = NewLinuxBridgeClient(bridgeName, extIf.Name, *nwInfo, nm.netlink, nm.plClient)
	}

	// Check if the bridge already exists.
	bridge, err := net.InterfaceByName(bridgeName)

	if err != nil {
		// Create the bridge.
		if err = networkClient.CreateBridge(); err != nil {
			logger.Error("Error while creating bridge", zap.Error(err))
			return err
		}

		bridge, err = net.InterfaceByName(bridgeName)
		if err != nil {
			return err
		}
	} else {
		// Use the existing bridge.
		logger.Info("Found existing bridge", zap.String("bridgeName", bridgeName))
	}

	defer func() {
		if err != nil {
			logger.Info("cleanup network")
			nm.disconnectExternalInterface(extIf, networkClient)
		}
	}()

	// Save host IP configuration.
	err = nm.saveIPConfig(hostIf, extIf)
	if err != nil {
		logger.Error("Failed to save IP configuration for interface",
			zap.String("Name", hostIf.Name), zap.Error(err))
	}

	/*
		If custom dns server is updated, VM needs reboot for the change to take effect.
	*/
	isGreaterOrEqualUbuntu17 := isGreaterOrEqaulUbuntuVersion(ubuntuVersion17)
	isSystemdResolvedActive := false
	if isGreaterOrEqualUbuntu17 {
		// Don't copy dns servers if systemd-resolved isn't available
		if _, cmderr := nm.plClient.ExecuteCommand("systemctl status systemd-resolved"); cmderr == nil {
			isSystemdResolvedActive = true
			logger.Info("Saving dns config from", zap.String("Name", hostIf.Name))
			if err = nm.saveDNSConfig(extIf); err != nil {
				logger.Error("Failed to save dns config", zap.Error(err))
				return err
			}
		}
	}

	// External interface down.
	logger.Info("Setting link state down", zap.String("Name", hostIf.Name))
	err = nm.netlink.SetLinkState(hostIf.Name, false)
	if err != nil {
		return err
	}

	// Connect the external interface to the bridge.
	logger.Info("Setting link master", zap.String("Name", hostIf.Name), zap.String("bridgeName", bridgeName))
	if err = networkClient.SetBridgeMasterToHostInterface(); err != nil {
		return err
	}

	// External interface up.
	logger.Info("Setting link state up", zap.String("Name", hostIf.Name))
	err = nm.netlink.SetLinkState(hostIf.Name, true)
	if err != nil {
		return err
	}

	// Bridge up.
	logger.Info("Setting link state up", zap.String("bridgeName", bridgeName))
	err = nm.netlink.SetLinkState(bridgeName, true)
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
		logger.Info("Setting link hairpin on", zap.String("Name", hostIf.Name))
		if err = networkClient.SetHairpinOnHostInterface(true); err != nil {
			return err
		}
	}

	// Apply IP configuration to the bridge for host traffic.
	err = nm.applyIPConfig(extIf, bridge)
	if err != nil {
		logger.Error("Failed to apply interface IP configuration", zap.Error(err))
		return err
	}

	if isGreaterOrEqualUbuntu17 && isSystemdResolvedActive {
		logger.Info("Applying dns config on", zap.String("bridgeName", bridgeName))

		if err = nm.applyDNSConfig(extIf, bridgeName); err != nil {
			logger.Error("Failed to apply DNS configuration with", zap.Error(err))
			return err
		}

		logger.Info("Applied dns config on", zap.Any("DNSInfo", extIf.DNSInfo), zap.String("bridgeName", bridgeName))
	}

	if nwInfo.IPV6Mode == IPV6Nat {
		// adds pod cidr gateway ip to bridge
		if err = nm.addIpv6NatGateway(nwInfo); err != nil {
			logger.Error("Adding IPv6 Nat Gateway failed with", zap.Error(err))
			return err
		}

		if err = nm.addIpv6SnatRule(extIf, nwInfo); err != nil {
			logger.Error("Adding IPv6 Snat Rule failed with", zap.Error(err))
			return err
		}

		// unmark packet if set by kube-proxy to skip kube-postrouting rule and processed
		// by cni snat rule
		if err = iptables.InsertIptableRule(iptables.V6, iptables.Mangle, iptables.Postrouting, "", "MARK --set-mark 0x0"); err != nil {
			logger.Error("Adding Iptable mangle rule failed", zap.Error(err))
			return err
		}
	}

	extIf.BridgeName = bridgeName
	logger.Info("Connected interface to bridge", zap.String("Name", extIf.Name), zap.String("BridgeName", extIf.BridgeName))

	return nil
}

// DisconnectExternalInterface disconnects a host interface from its bridge.
func (nm *networkManager) disconnectExternalInterface(extIf *externalInterface, networkClient NetworkClient) {
	logger.Info("Disconnecting interface", zap.String("Name", extIf.Name))

	logger.Info("Deleting bridge rules")
	// Delete bridge rules set on the external interface.
	networkClient.DeleteL2Rules(extIf)

	logger.Info("Deleting bridge")
	// Delete Bridge
	networkClient.DeleteBridge()

	extIf.BridgeName = ""
	logger.Info("Restoring ipconfig with primary interface", zap.String("Name", extIf.Name))

	// Restore IP configuration.
	hostIf, _ := net.InterfaceByName(extIf.Name)
	err := nm.applyIPConfig(extIf, hostIf)
	if err != nil {
		logger.Error("Failed to apply IP configuration", zap.Error(err))
	}

	extIf.IPAddresses = nil
	extIf.Routes = nil

	logger.Info("Disconnected interface", zap.String("Name", extIf.Name))
}

func (*networkManager) addToIptables(cmds []iptables.IPTableEntry) error {
	logger.Info("Adding additional iptable rules...")
	for _, cmd := range cmds {
		err := iptables.RunCmd(cmd.Version, cmd.Params)
		if err != nil {
			return err
		}
		logger.Info("Successfully run iptables rule", zap.Any("cmd", cmd))
	}
	return nil
}

// Add ipv6 nat gateway IP on bridge
func (nm *networkManager) addIpv6NatGateway(nwInfo *NetworkInfo) error {
	logger.Info("Adding ipv6 nat gateway on azure bridge")
	for _, subnetInfo := range nwInfo.Subnets {
		if subnetInfo.Family == platform.AfINET6 {
			ipAddr := []net.IPNet{{
				IP:   subnetInfo.Gateway,
				Mask: subnetInfo.Prefix.Mask,
			}}
			nuc := networkutils.NewNetworkUtils(nm.netlink, nm.plClient)
			err := nuc.AssignIPToInterface(nwInfo.BridgeName, ipAddr)
			if err != nil {
				return newErrorNetworkManager(err.Error())
			}
		}
	}

	return nil
}

// snat ipv6 traffic to secondary ipv6 ip before leaving VM
func (*networkManager) addIpv6SnatRule(extIf *externalInterface, nwInfo *NetworkInfo) error {
	var (
		ipv6SnatRuleSet  bool
		ipv6SubnetPrefix net.IPNet
	)

	for _, subnet := range nwInfo.Subnets {
		if subnet.Family == platform.AfINET6 {
			ipv6SubnetPrefix = subnet.Prefix
			break
		}
	}

	if len(ipv6SubnetPrefix.IP) == 0 {
		return errSubnetV6NotFound
	}

	for _, ipAddr := range extIf.IPAddresses {
		if ipAddr.IP.To4() == nil {
			logger.Info("Adding ipv6 snat rule")
			matchSrcPrefix := fmt.Sprintf("-s %s", ipv6SubnetPrefix.String())
			if err := networkutils.AddSnatRule(matchSrcPrefix, ipAddr.IP); err != nil {
				return fmt.Errorf("Adding iptable snat rule failed:%w", err)
			}
			ipv6SnatRuleSet = true
		}
	}

	if !ipv6SnatRuleSet {
		return errV6SnatRuleNotSet
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

// AddStaticRoute adds a static route to the interface.
func AddStaticRoute(nl netlink.NetlinkInterface, netioshim netio.NetIOInterface, ip, interfaceName string) error {
	logger.Info("Adding static route", zap.String("ip", ip))
	var routes []RouteInfo
	_, ipNet, _ := net.ParseCIDR(ip)
	gwIP := net.ParseIP("0.0.0.0")
	route := RouteInfo{Dst: *ipNet, Gw: gwIP}
	routes = append(routes, route)
	if err := addRoutes(nl, netioshim, interfaceName, routes); err != nil {
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "file exists") {
			logger.Error("addroutes failed with error", zap.Error(err))
			return err
		}
	}

	return nil
}
