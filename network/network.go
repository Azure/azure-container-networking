// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/Azure/azure-container-networking/platform"
)

const (
	// Operational modes.
	opModeBridge      = "bridge"
	opModeTunnel      = "tunnel"
	opModeTransparent = "transparent"
	opModeDefault     = opModeTunnel
)

const (
	// ipv6 modes
	IPV6Nat = "ipv6nat"
)

// externalInterface is a host network interface that bridges containers to external networks.
type externalInterface struct {
	Name        string
	Networks    map[string]*network
	Subnets     []string
	BridgeName  string
	DNSInfo     DNSInfo
	MacAddress  net.HardwareAddr
	IPAddresses []*net.IPNet
	Routes      []*route
	IPv4Gateway net.IP
	IPv6Gateway net.IP
}

// A container network is a set of endpoints allowed to communicate with each other.
type network struct {
	Id               string
	HnsId            string `json:",omitempty"`
	Mode             string
	VlanId           int
	Subnets          []SubnetInfo
	Endpoints        map[string]*endpoint
	extIf            *externalInterface
	DNS              DNSInfo
	EnableSnatOnHost bool
	NetNs            string
	SnatBridgeIP     string
}

// NetworkInfo contains read-only information about a container network.
type NetworkInfo struct {
	MasterIfName                  string
	AdapterName                   string
	Id                            string
	Mode                          string
	Subnets                       []SubnetInfo
	PodSubnet                     SubnetInfo
	DNS                           DNSInfo
	Policies                      []policy.Policy
	BridgeName                    string
	EnableSnatOnHost              bool
	NetNs                         string
	Options                       map[string]interface{}
	DisableHairpinOnHostInterface bool
	IPV6Mode                      string
	IPAMType                      string
	ServiceCidrs                  string
}

// SubnetInfo contains subnet information for a container network.
type SubnetInfo struct {
	Family    platform.AddressFamily
	Prefix    net.IPNet
	Gateway   net.IP
	PrimaryIP net.IP
}

// DNSInfo contains DNS information for a container network or endpoint.
type DNSInfo struct {
	Suffix  string
	Servers []string
	Options []string
}

// NewExternalInterface adds a host interface to the list of available external interfaces.
func (nm *networkManager) newExternalInterface(ifName string, subnet string) error {
	// Check whether the external interface is already configured.
	if nm.ExternalInterfaces[ifName] != nil {
		return nil
	}

	// Find the host interface.
	hostIf, err := net.InterfaceByName(ifName)
	if err != nil {
		return err
	}

	extIf := externalInterface{
		Name:        ifName,
		Networks:    make(map[string]*network),
		MacAddress:  hostIf.HardwareAddr,
		IPv4Gateway: net.IPv4zero,
		IPv6Gateway: net.IPv6unspecified,
	}

	extIf.Subnets = append(extIf.Subnets, subnet)

	nm.ExternalInterfaces[ifName] = &extIf

	log.Printf("[net] Added ExternalInterface %v for subnet %v.", ifName, subnet)

	return nil
}

// DeleteExternalInterface removes an interface from the list of available external interfaces.
func (nm *networkManager) deleteExternalInterface(ifName string) error {
	delete(nm.ExternalInterfaces, ifName)

	log.Printf("[net] Deleted ExternalInterface %v.", ifName)

	return nil
}

// FindExternalInterfaceBySubnet finds an external interface connected to the given subnet.
func (nm *networkManager) findExternalInterfaceBySubnet(subnet string) *externalInterface {
	for _, extIf := range nm.ExternalInterfaces {
		for _, s := range extIf.Subnets {
			if s == subnet {
				return extIf
			}
		}
	}

	return nil
}

// FindExternalInterfaceByName finds an external interface by name.
func (nm *networkManager) findExternalInterfaceByName(ifName string) *externalInterface {
	extIf, exists := nm.ExternalInterfaces[ifName]
	if exists && extIf != nil {
		return extIf
	}

	return nil
}

// NewNetwork creates a new container network.
func (nm *networkManager) newNetwork(nwInfo *NetworkInfo) (*network, error) {
	var nw *network
	var err error

	log.Printf("[net] Creating network %+v.", nwInfo)
	defer func() {
		if err != nil {
			log.Printf("[net] Failed to create network %v, err:%v.", nwInfo.Id, err)
		}
	}()

	// Set defaults.
	if nwInfo.Mode == "" {
		nwInfo.Mode = opModeDefault
	}

	// If the master interface name is provided, find the external interface by name
	// else use subnet to to find the interface
	var extIf *externalInterface
	if len(strings.TrimSpace(nwInfo.MasterIfName)) > 0 {
		extIf = nm.findExternalInterfaceByName(nwInfo.MasterIfName)
	} else {
		extIf = nm.findExternalInterfaceBySubnet(nwInfo.Subnets[0].Prefix.String())
	}
	if extIf == nil {
		err = errSubnetNotFound
		return nil, err
	}

	// Make sure this network does not already exist.
	if extIf.Networks[nwInfo.Id] != nil {
		err = errNetworkExists
		return nil, err
	}

	// Call the OS-specific implementation.
	nw, err = nm.newNetworkImpl(nwInfo, extIf)
	if err != nil {
		return nil, err
	}

	// Add the network object.
	nw.Subnets = nwInfo.Subnets
	extIf.Networks[nwInfo.Id] = nw

	log.Printf("[net] Created network %v on interface %v.", nwInfo.Id, extIf.Name)
	return nw, nil
}

// DeleteNetwork deletes an existing container network.
func (nm *networkManager) deleteNetwork(networkId string) error {
	var err error

	log.Printf("[net] Deleting network %v.", networkId)
	defer func() {
		if err != nil {
			log.Printf("[net] Failed to delete network %v, err:%v.", networkId, err)
		}
	}()

	// Find the network.
	nw, err := nm.getNetwork(networkId)
	if err != nil {
		return err
	}

	// Call the OS-specific implementation.
	err = nm.deleteNetworkImpl(nw)
	if err != nil {
		return err
	}

	// Remove the network object.
	if nw.extIf != nil {
		delete(nw.extIf.Networks, networkId)
	}

	log.Printf("[net] Deleted network %+v.", nw)
	return nil
}

// GetNetwork returns the network with the given ID.
func (nm *networkManager) getNetwork(networkId string) (*network, error) {
	for _, extIf := range nm.ExternalInterfaces {
		nw, ok := extIf.Networks[networkId]
		if ok {
			return nw, nil
		}
	}

	return nil, errNetworkNotFound
}
