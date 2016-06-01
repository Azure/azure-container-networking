// Copyright Microsoft Corp.
// All rights reserved.

package core

import (
	"fmt"
	"net"
	"time"

	"github.com/Azure/Aqua/netfilter"
	"github.com/Azure/Aqua/netlink"
)

const (
	// Prefix for bridge names.
	bridgePrefix = "aqua"

	// Prefix for host virtual network interface names.
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
	macAddress net.HardwareAddr

	// Number of networks using this external interface.
	networkCount int
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
		extIf, err = connectExternalInterface(extIfName)
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
	return disconnectExternalInterface(nw.extIf.name)
}

// Connects a host interface to a bridge.
func connectExternalInterface(ifName string) (*externalInterface, error) {
	// Find the external interface.
	hostIf, err := net.InterfaceByName(ifName)
	if err != nil {
		return nil, err
	}

	// Create the bridge.
	bridgeName := bridgePrefix + "0"
	_, err = net.InterfaceByName(bridgeName)
	if err != nil {
		if err := netlink.AddLink(bridgeName, "bridge"); err != nil {
			return nil, err
		}
	}

	// Assign external interface's IP addresses to the bridge for host traffic.
	addrs, _ := hostIf.Addrs()
	for _, addr := range addrs {
		ipAddr, ipNet, err := net.ParseCIDR(addr.String())
		ipNet.IP = ipAddr
		if err != nil {
			return nil, err
		}

		err = netlink.DeleteIpAddress(ifName, ipAddr, ipNet)
		if err != nil {
			return nil, err
		}

		err = netlink.AddIpAddress(bridgeName, ipAddr, ipNet)
		if err != nil {
			return nil, err
		}
	}

	// Setup MAC address translation rules for external interface.
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

	// Connect the external interface to the bridge.
	err = netlink.SetLinkMaster(hostIf.Name, bridgeName)
	if err != nil {
		return nil, err
	}

	// External interface up.
	err = netlink.SetLinkState(hostIf.Name, true)
	if err != nil {
		return nil, err
	}

	// Bridge up.
	err = netlink.SetLinkState(bridgeName, true)
	if err != nil {
		return nil, err
	}

	// Save external interface's state.
	extIf := externalInterface{
		name:       hostIf.Name,
		bridgeName: bridgeName,
		macAddress: hostIf.HardwareAddr,
	}

	externalInterfaces[hostIf.Name] = &extIf

	return &extIf, nil
}

// Disconnects a host interface from its bridge.
func disconnectExternalInterface(ifName string) error {
	// Find the external interface.
	extIf := externalInterfaces[ifName]
	if extIf == nil {
		return nil
	}

	// Disconnect the interface if this was the last network using it.
	extIf.networkCount--
	if extIf.networkCount > 0 {
		return nil
	}

	// Cleanup MAC address translation rules.
	ebtables.CleanupDnatForArpReplies(ifName)
	ebtables.CleanupSnatForOutgoingPackets(ifName, extIf.macAddress.String())

	// Disconnect external interface from its bridge.
	err := netlink.SetLinkMaster(ifName, "")
	if err != nil {
		return err
	}

	// Delete the bridge.
	err = netlink.DeleteLink(extIf.bridgeName)
	if err != nil {
		return err
	}

	// Restart external interface to trigger DHCP/SLAAC and reset its configuration.
	restartInterface(ifName)

	delete(externalInterfaces, ifName)

	return nil
}

// Restarts an interface by setting its operational state down and back up.
func restartInterface(ifName string) error {
	err := netlink.SetLinkState(ifName, false)
	if err != nil {
		return err
	}

	// Delay for the state to settle.
	time.Sleep(2 * time.Second)

	err = netlink.SetLinkState(ifName, true)
	return err
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
