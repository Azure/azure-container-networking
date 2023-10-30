// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/Azure/azure-container-networking/platform"
	"go.uber.org/zap"
)

const (
	// Operational modes.
	opModeBridge          = "bridge"
	opModeTunnel          = "tunnel"
	opModeTransparent     = "transparent"
	opModeTransparentVlan = "transparent-vlan"
	opModeDefault         = opModeTunnel
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
	IsIPv6Enabled                 bool
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

func (nwInfo *NetworkInfo) PrettyString() string {
	return fmt.Sprintf("Id:%s MasterIfName:%s AdapterName:%s Mode:%s Subnets:%v podsubnet:%v Enablesnatonhost:%t", nwInfo.Id, nwInfo.MasterIfName,
		nwInfo.AdapterName, nwInfo.Mode, nwInfo.Subnets, nwInfo.PodSubnet, nwInfo.EnableSnatOnHost)
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

	logger.Info("Added ExternalInterface for subnet", zap.String("ifName", ifName), zap.String("subnet", subnet))

	return nil
}

// DeleteExternalInterface removes an interface from the list of available external interfaces.
func (nm *networkManager) deleteExternalInterface(ifName string) error {
	delete(nm.ExternalInterfaces, ifName)

	logger.Info("Deleted ExternalInterface", zap.String("ifName", ifName))

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

	logger.Info("Creating", zap.String("network", nwInfo.PrettyString()))
	defer func() {
		if err != nil {
			logger.Error("Failed to create network", zap.String("id", nwInfo.Id), zap.Error(err))
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

	logger.Info("Created network on interface", zap.String("id", nwInfo.Id), zap.String("Name", extIf.Name))
	return nw, nil
}

// DeleteNetwork deletes an existing container network.
func (nm *networkManager) deleteNetwork(networkID string) error {
	var err error

	logger.Info("Deleting network", zap.String("networkID", networkID))
	defer func() {
		if err != nil {
			logger.Error("Failed to delete network", zap.String("networkID", networkID), zap.Error(err))
		}
	}()

	// Find the network.
	nw, err := nm.getNetwork(networkID)
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
		delete(nw.extIf.Networks, networkID)
	}

	logger.Info("Deleted network", zap.Any("nw", nw))
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

// getNetworkIDForNetNs finds the network that contains the endpoint that was created for this netNs. Returns
// and errNetworkNotFound if the netNs is not found in any network
func (nm *networkManager) FindNetworkIDFromNetNs(netNs string) (string, error) {
	logger.Info("Querying state for network for NetNs", zap.String("netNs", netNs))

	// Look through the external interfaces
	for _, iface := range nm.ExternalInterfaces {
		// Look through the networks
		for _, network := range iface.Networks {
			// Network may have multiple endpoints, so look through all of them
			for _, endpoint := range network.Endpoints {
				// If the netNs matches for this endpoint, return the network ID (which is the name)
				if endpoint.NetNs == netNs {
					logger.Info("Found network for NetNS", zap.String("id", network.Id), zap.String("netNs", netNs))
					return network.Id, nil
				}
			}
		}
	}

	return "", errNetworkNotFound
}

// GetNumEndpointsInNetNs returns number of endpoints
func (nm *networkManager) GetNumEndpointsByContainerID(containerID string) int {
	numEndpoints := 0
	// Look through the external interfaces
	for _, iface := range nm.ExternalInterfaces {
		// Look through the networks
		for _, network := range iface.Networks {
			// Network may have multiple endpoints, so look through all of them
			for _, endpoint := range network.Endpoints {
				// If the netNs matches for this endpoint, return the network ID (which is the name)
				if endpoint.ContainerID == containerID {
					logger.Info("Found endpoint for containerID", zap.String("id", endpoint.Id), zap.String("containerID", containerID))
					numEndpoints++
				}
			}
		}
	}

	return numEndpoints
}
