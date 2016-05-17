// Copyright Microsoft Corp.
// All rights reserved.

package core

import (
	"crypto/rand"
	"fmt"
	"net"

	"github.com/Azure/Aqua/netfilter"
	"github.com/Azure/Aqua/netlink"
)

const (
	// Prefix for bridge names.
	bridgePrefix = "aqua"

	// Prefix for host network interface names.
	hostInterfacePrefix = "veth"

	// Prefix for container network interface names.
	containerInterfacePrefix = "eth"
)

// Network is the bridge and the underlying external interface.
type Network struct {
	id    string
	extIf *externalInterface
}

// Container interface
type Endpoint struct {
	IPv4Address net.IPNet
	IPv6Address net.IPNet
	MacAddress  net.HardwareAddr
	SrcName     string
	DstPrefix   string
	GatewayIPv4 net.IP
}

// External interface is a host network interface that forwards traffic between
// containers and external networks.
type externalInterface struct {
	name       string
	bridgeName string

	// Number of networks using this external interface.
	networkCount int

	originalHwAddress net.HardwareAddr
	assignedHwAddress net.HardwareAddr

	addresses map[string]string
}

var externalInterfaces map[string]*externalInterface = make(map[string]*externalInterface)

// Creates a container network.
func CreateNetwork(networkId string, ipv4Pool string, ipv6Pool string) (*Network, error) {
	// Find the external interface for this subnet.
	extIfName := "eth1"

	// Check whether the external interface is already configured.
	extIf := externalInterfaces[extIfName]
	if extIf == nil {
		var err error
		extIf, err = acquireExternalInterface(extIfName)
		if err != nil {
			return nil, err
		}
	}

	extIf.networkCount++

	nw := &Network{
		id:    networkId,
		extIf: extIf,
	}

	return nw, nil
}

// Deletes a container network.
func DeleteNetwork(nw *Network) error {
	return releaseExternalInterface(nw.extIf.name)
}

// Enslaves an interface and connects it to a bridge.
func acquireExternalInterface(ifName string) (*externalInterface, error) {
	// Find the external interface.
	hostIf, err := net.InterfaceByName(ifName)
	if err != nil {
		return nil, err
	}

	// Create bridge.
	bridgeName := bridgePrefix + "0"
	_, err = net.InterfaceByName(bridgeName)
	if err != nil {
		if err := netlink.AddLink(bridgeName, "bridge"); err != nil {
			return nil, err
		}
	}

	// Bridge up.
	err = netlink.SetLinkState(bridgeName, true)
	if err != nil {
		return nil, err
	}

	// Setup MAC address translation rules.
	err = ebtables.SetupSnatForOutgoingPackets(hostIf.Name, hostIf.HardwareAddr.String())
	if err != nil {
		return nil, err
	}

	err = ebtables.SetupDnatForArpReplies(hostIf.Name)
	if err != nil {
		return nil, err
	}

	// External interface down.
	err = netlink.SetLinkState(hostIf.Name, false)
	if err != nil {
		return nil, err
	}

	// Get a new link address for the external interface.
	macAddress, err := generateMacAddress()
	if err != nil {
		return nil, err
	}

	// Save state.
	extIf := externalInterface{
		name:              hostIf.Name,
		bridgeName:        bridgeName,
		originalHwAddress: hostIf.HardwareAddr,
		assignedHwAddress: macAddress,
		addresses:         make(map[string]string),
	}

	// Save IP addresses on the host interface, to be
	// restored when the interface is released.
	addrs, _ := hostIf.Addrs()
	for _, addr := range addrs {
		extIf.addresses[addr.String()] = addr.String()
	}

	// Set the new link address on external interface.
	err = netlink.SetLinkAddress(hostIf.Name, macAddress)
	if err != nil {
		return nil, err
	}

	// External interface up.
	err = netlink.SetLinkState(hostIf.Name, true)
	if err != nil {
		return nil, err
	}

	// Connect the external interface to the bridge.
	err = netlink.SetLinkMaster(hostIf.Name, bridgeName)
	if err != nil {
		return nil, err
	}

	externalInterfaces[hostIf.Name] = &extIf

	return &extIf, nil
}

// Releases an enslaved interface and disconnects it from its bridge.
func releaseExternalInterface(ifName string) error {
	//
	extIf := externalInterfaces[ifName]

	// Disconnect external interface from its bridge.
	err := netlink.SetLinkMaster(ifName, "")
	if err != nil {
		return err
	}

	// External interface down.
	err = netlink.SetLinkState(ifName, false)
	if err != nil {
		return err
	}

	// Restore the original link address.
	err = netlink.SetLinkAddress(ifName, extIf.originalHwAddress)
	if err != nil {
		return err
	}

	// External interface up.
	err = netlink.SetLinkState(ifName, true)
	if err != nil {
		return err
	}

	// Cleanup MAC address translation rules.
	ebtables.CleanupDnatForArpReplies(ifName)
	ebtables.CleanupSnatForOutgoingPackets(ifName, extIf.originalHwAddress.String())

	// Restore IP addresses.
	for _, addr := range extIf.addresses {
		ip, ipNet, err := net.ParseCIDR(addr)
		if err != nil {
			netlink.AddIpAddress(ifName, ip, ipNet)
		}
	}

	// Delete the bridge if this was the last network using it.
	extIf.networkCount--
	if extIf.networkCount == 0 {
		err := netlink.DeleteLink(extIf.bridgeName)
		if err != nil {
			return err
		}
	}

	return nil
}

// Generates a random MAC address.
func generateMacAddress() (net.HardwareAddr, error) {
	buf := make([]byte, 6)
	_, err := rand.Read(buf)
	if err != nil {
		return nil, err
	}

	// Clear the multicast bit.
	buf[0] &= 0xFE

	// Set the locally administered bit.
	buf[0] |= 2

	macAddr := fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", buf[0], buf[1], buf[2], buf[3], buf[4], buf[5])
	hwAddr, err := net.ParseMAC(macAddr)
	if err != nil {
		return nil, err
	}

	return hwAddr, nil
}

// Creates a new endpoint.
func CreateEndpoint(nw *Network, endpointId string, ipAddress string) (*Endpoint, error) {
	var containerIf *net.Interface
	var ep *Endpoint

	// Parse IP address.
	ipAddr, ipNet, err := net.ParseCIDR(ipAddress)
	ipNet.IP = ipAddr
	if err != nil {
		return nil, err
	}

	// Create a veth pair.
	contIfName := fmt.Sprintf("%s%s-2", hostInterfacePrefix, endpointId[:7])
	hostIfName := fmt.Sprintf("%s%s", hostInterfacePrefix, endpointId[:7])

	err = netlink.AddVethPair(contIfName, hostIfName)
	if err != nil {
		return nil, err
	}

	// Assign IP address to container network interface.
	err = netlink.AddIpAddress(contIfName, ipAddr, ipNet)
	if err != nil {
		goto cleanup
	}

	// Host interface up.
	err = netlink.SetLinkState(hostIfName, true)
	if err != nil {
		goto cleanup
	}

	// Connect host interface to the bridge.
	err = netlink.SetLinkMaster(hostIfName, nw.extIf.bridgeName)
	if err != nil {
		goto cleanup
	}

	// Query container network interface info.
	containerIf, err = net.InterfaceByName(contIfName)
	if err != nil {
		goto cleanup
	}

	// Setup NAT.
	err = ebtables.SetupDnatBasedOnIPV4Address(ipAddr.String(), containerIf.HardwareAddr.String())
	if err != nil {
		goto cleanup
	}

	ep = &Endpoint{
		IPv4Address: *ipNet,
		IPv6Address: net.IPNet{},
		MacAddress:  containerIf.HardwareAddr,
		SrcName:     contIfName,
		DstPrefix:   containerInterfacePrefix,
		GatewayIPv4: net.IPv4(0, 0, 0, 0),
	}

	return ep, nil

cleanup:
	// Roll back the changes for the endpoint.
	netlink.DeleteLink(contIfName)

	return nil, err
}

// Deletes an existing endpoint.
func DeleteEndpoint(ep *Endpoint) error {
	// Delete veth pair.
	netlink.DeleteLink(ep.SrcName)

	// Remove NAT.
	err := ebtables.RemoveDnatBasedOnIPV4Address(ep.IPv4Address.IP.String(), ep.MacAddress.String())

	return err
}
