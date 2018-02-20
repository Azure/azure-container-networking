// Copyright 2017 Microsoft. All rights reserved.
// MIT License

// +build linux

package network

import (
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/ebtables"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netlink"
	"golang.org/x/sys/unix"
)

const (
	// Prefix for bridge names.
	bridgePrefix = "azure"

	// Virtual MAC address used by Azure VNET.
	virtualMacAddress = "12:34:56:78:9a:bc"
)

// Linux implementation of route.
type route netlink.Route

// NewNetworkImpl creates a new container network.
func (nm *networkManager) newNetworkImpl(nwInfo *NetworkInfo, extIf *externalInterface) (*network, error) {
	// Connect the external interface.
	switch nwInfo.Mode {
	case opModeTunnel:
		fallthrough
	case opModeBridge:
		err := nm.connectExternalInterface(extIf, nwInfo)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errNetworkModeInvalid
	}

	// Create the network object.
	nw := &network{
		Id:        nwInfo.Id,
		Mode:      nwInfo.Mode,
		Endpoints: make(map[string]*endpoint),
		extIf:     extIf,
	}

	return nw, nil
}

// DeleteNetworkImpl deletes an existing container network.
func (nm *networkManager) deleteNetworkImpl(nw *network) error {
	// Disconnect the interface if this was the last network using it.
	if len(nw.extIf.Networks) == 1 {
		nm.disconnectExternalInterface(nw.extIf)
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

// AddBridgeRules adds bridge frame table rules for container traffic.
func (nm *networkManager) addBridgeRules(extIf *externalInterface, hostIf *net.Interface, bridgeName string, opMode string) error {
	// Add SNAT rule to translate container egress traffic.
	log.Printf("[net] Adding SNAT rule for egress traffic on %v.", hostIf.Name)
	err := ebtables.SetSnatForInterface(hostIf.Name, hostIf.HardwareAddr, ebtables.Append)
	if err != nil {
		return err
	}

	// Add ARP reply rule for host primary IP address.
	// ARP requests for all IP addresses are forwarded to the SDN fabric, but fabric
	// doesn't respond to ARP requests from the VM for its own primary IP address.
	primary := extIf.IPAddresses[0].IP
	log.Printf("[net] Adding ARP reply rule for primary IP address %v.", primary)
	err = ebtables.SetArpReply(primary, hostIf.HardwareAddr, ebtables.Append)
	if err != nil {
		return err
	}

	// Add DNAT rule to forward ARP replies to container interfaces.
	log.Printf("[net] Adding DNAT rule for ingress ARP traffic on interface %v.", hostIf.Name)
	err = ebtables.SetDnatForArpReplies(hostIf.Name, ebtables.Append)
	if err != nil {
		return err
	}

	// Enable VEPA for host policy enforcement if necessary.
	if opMode == opModeTunnel {
		log.Printf("[net] Enabling VEPA mode for %v.", hostIf.Name)
		err = ebtables.SetVepaMode(bridgeName, commonInterfacePrefix, virtualMacAddress, ebtables.Append)
		if err != nil {
			return err
		}
	}

	return nil
}

// DeleteBridgeRules deletes bridge rules for container traffic.
func (nm *networkManager) deleteBridgeRules(extIf *externalInterface) {
	ebtables.SetVepaMode(extIf.BridgeName, commonInterfacePrefix, virtualMacAddress, ebtables.Delete)
	ebtables.SetDnatForArpReplies(extIf.Name, ebtables.Delete)
	ebtables.SetArpReply(extIf.IPAddresses[0].IP, extIf.MacAddress, ebtables.Delete)
	ebtables.SetSnatForInterface(extIf.Name, extIf.MacAddress, ebtables.Delete)
}

// ConnectExternalInterface connects the given host interface to a bridge.
func (nm *networkManager) connectExternalInterface(extIf *externalInterface, nwInfo *NetworkInfo) error {
	var err error

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

	// Check if the bridge already exists.
	bridge, err := net.InterfaceByName(bridgeName)
	if err != nil {
		// Create the bridge.
		log.Printf("[net] Creating bridge %v.", bridgeName)

		link := netlink.BridgeLink{
			LinkInfo: netlink.LinkInfo{
				Type: netlink.LINK_TYPE_BRIDGE,
				Name: bridgeName,
			},
		}

		err = netlink.AddLink(&link)
		if err != nil {
			return err
		}

		// On failure, delete the bridge.
		defer func() {
			if err != nil {
				netlink.DeleteLink(bridgeName)
			}
		}()

		bridge, err = net.InterfaceByName(bridgeName)
		if err != nil {
			return err
		}
	} else {
		// Use the existing bridge.
		log.Printf("[net] Found existing bridge %v.", bridgeName)
	}

	// Save host IP configuration.
	err = nm.saveIPConfig(hostIf, extIf)
	if err != nil {
		log.Printf("[net] Failed to save IP configuration for interface %v: %v.", hostIf.Name, err)
	}

	// Add the bridge rules.
	err = nm.addBridgeRules(extIf, hostIf, bridgeName, nwInfo.Mode)
	if err != nil {
		return err
	}

	// External interface down.
	log.Printf("[net] Setting link %v state down.", hostIf.Name)
	err = netlink.SetLinkState(hostIf.Name, false)
	if err != nil {
		return err
	}

	// Connect the external interface to the bridge.
	log.Printf("[net] Setting link %v master %v.", hostIf.Name, bridgeName)
	err = netlink.SetLinkMaster(hostIf.Name, bridgeName)
	if err != nil {
		return err
	}

	// External interface up.
	log.Printf("[net] Setting link %v state up.", hostIf.Name)
	err = netlink.SetLinkState(hostIf.Name, true)
	if err != nil {
		return err
	}

	// External interface hairpin on.
	log.Printf("[net] Setting link %v hairpin on.", hostIf.Name)
	err = netlink.SetLinkHairpin(hostIf.Name, true)
	if err != nil {
		return err
	}

	// Bridge up.
	log.Printf("[net] Setting link %v state up.", bridgeName)
	err = netlink.SetLinkState(bridgeName, true)
	if err != nil {
		return err
	}

	// Apply IP configuration to the bridge for host traffic.
	err = nm.applyIPConfig(extIf, bridge)
	if err != nil {
		log.Printf("[net] Failed to apply interface IP configuration: %v.", err)
	}

	extIf.BridgeName = bridgeName
	err = nil

	log.Printf("[net] Connected interface %v to bridge %v.", extIf.Name, extIf.BridgeName)

	return nil
}

// DisconnectExternalInterface disconnects a host interface from its bridge.
func (nm *networkManager) disconnectExternalInterface(extIf *externalInterface) error {
	log.Printf("[net] Disconnecting interface %v.", extIf.Name)

	// Delete bridge rules set on the external interface.
	nm.deleteBridgeRules(extIf)

	// Disconnect external interface from its bridge.
	err := netlink.SetLinkMaster(extIf.Name, "")
	if err != nil {
		log.Printf("[net] Failed to disconnect interface %v from bridge, err:%v.", extIf.Name, err)
	}

	// Delete the bridge.
	err = netlink.DeleteLink(extIf.BridgeName)
	if err != nil {
		log.Printf("[net] Failed to delete bridge %v, err:%v.", extIf.BridgeName, err)
	}

	extIf.BridgeName = ""

	// Restore IP configuration.
	hostIf, _ := net.InterfaceByName(extIf.Name)
	err = nm.applyIPConfig(extIf, hostIf)
	if err != nil {
		log.Printf("[net] Failed to apply IP configuration: %v.", err)
	}

	extIf.IPAddresses = nil
	extIf.Routes = nil

	log.Printf("[net] Disconnected interface %v.", extIf.Name)

	return nil
}
