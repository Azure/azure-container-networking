// Copyright 2017 Microsoft. All rights reserved.
// MIT License

// +build linux

package network

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netlink"
)

const (
	// Common prefix for all types of host network interface names.
	commonInterfacePrefix = "az"

	// Prefix for host virtual network interface names.
	hostVEthInterfacePrefix = commonInterfacePrefix + "v"

	// Prefix for container network interface names.
	containerInterfacePrefix = "eth"
)

func generateVethName(key string) string {
	h := sha1.New()
	h.Write([]byte(key))
	return hex.EncodeToString(h.Sum(nil))[:11]
}

func ConstructEndpointID(containerID string, netNsPath string, ifName string) (string, string) {
	if len(containerID) > 8 {
		containerID = containerID[:8]
	} else {
		log.Printf("Container ID is not greater than 8 ID: %v", containerID)
		return "", ""
	}

	infraEpName := containerID + "-" + ifName

	return infraEpName, ""
}

// newEndpointImpl creates a new endpoint in the network.
func (nw *network) newEndpointImpl(epInfo *EndpointInfo) (*endpoint, error) {
	var containerIf *net.Interface
	var ns *Namespace
	var ep *endpoint
	var err error
	var hostIfName string
	var contIfName string
	var epClient EndpointClient
	var vlanid int = 0

	if nw.Endpoints[epInfo.Id] != nil {
		log.Printf("[net] Endpoint alreday exists.")
		err = errEndpointExists
		return nil, err
	}

	if epInfo.Data != nil {
		if _, ok := epInfo.Data[VlanIDKey]; ok {
			vlanid = epInfo.Data[VlanIDKey].(int)
		}
	}

	if _, ok := epInfo.Data[OptVethName]; ok {
		log.Printf("Generate veth name based on the key provided")
		key := epInfo.Data[OptVethName].(string)
		vethname := generateVethName(key)
		hostIfName = fmt.Sprintf("%s%s", hostVEthInterfacePrefix, vethname)
		contIfName = fmt.Sprintf("%s%s2", hostVEthInterfacePrefix, vethname)
	} else {
		// Create a veth pair.
		log.Printf("Generate veth name based on endpoint id")
		hostIfName = fmt.Sprintf("%s%s", hostVEthInterfacePrefix, epInfo.Id[:7])
		contIfName = fmt.Sprintf("%s%s-2", hostVEthInterfacePrefix, epInfo.Id[:7])
	}

	if vlanid != 0 {
		epClient = NewOVSEndpointClient(
			nw.extIf,
			epInfo,
			hostIfName,
			contIfName,
			vlanid)
	} else {
		epClient = NewLinuxBridgeEndpointClient(nw.extIf, hostIfName, contIfName, nw.Mode)
	}

	// Cleanup on failure.
	defer func() {
		if err != nil {
			log.Printf("CNI error. Delete Endpoint %v and rules that are created.", contIfName)
			endpt := &endpoint{
				Id:               epInfo.Id,
				IfName:           contIfName,
				HostIfName:       hostIfName,
				IPAddresses:      epInfo.IPAddresses,
				Gateways:         []net.IP{nw.extIf.IPv4Gateway},
				DNS:              epInfo.DNS,
				VlanID:           vlanid,
				EnableSnatOnHost: epInfo.EnableSnatOnHost,
			}

			if containerIf != nil {
				endpt.MacAddress = containerIf.HardwareAddr
				epClient.DeleteEndpointRules(endpt)
			}

			epClient.DeleteEndpoints(endpt)
		}
	}()

	if err = epClient.AddEndpoints(epInfo); err != nil {
		return nil, err
	}

	containerIf, err = net.InterfaceByName(contIfName)
	if err != nil {
		return nil, err
	}

	// Setup rules for IP addresses on the container interface.
	if err = epClient.AddEndpointRules(epInfo); err != nil {
		return nil, err
	}

	// If a network namespace for the container interface is specified...
	if epInfo.NetNsPath != "" {
		// Open the network namespace.
		log.Printf("[net] Opening netns %v.", epInfo.NetNsPath)
		ns, err = OpenNamespace(epInfo.NetNsPath)
		if err != nil {
			return nil, err
		}
		defer ns.Close()

		if err := epClient.MoveEndpointsToContainerNS(epInfo, ns.GetFd()); err != nil {
			return nil, err
		}

		// Enter the container network namespace.
		log.Printf("[net] Entering netns %v.", epInfo.NetNsPath)
		if err = ns.Enter(); err != nil {
			return nil, err
		}

		// Return to host network namespace.
		defer func() {
			log.Printf("[net] Exiting netns %v.", epInfo.NetNsPath)
			if err := ns.Exit(); err != nil {
				log.Printf("[net] Failed to exit netns, err:%v.", err)
			}
		}()
	}

	// If a name for the container interface is specified...
	if epInfo.IfName != "" {
		if err = epClient.SetupContainerInterfaces(epInfo); err != nil {
			return nil, err
		}
	}

	if err = epClient.ConfigureContainerInterfacesAndRoutes(epInfo); err != nil {
		return nil, err
	}

	// Create the endpoint object.
	ep = &endpoint{
		Id:               epInfo.Id,
		IfName:           epInfo.IfName,
		HostIfName:       hostIfName,
		MacAddress:       containerIf.HardwareAddr,
		InfraVnetIP:      epInfo.InfraVnetIP,
		IPAddresses:      epInfo.IPAddresses,
		Gateways:         []net.IP{nw.extIf.IPv4Gateway},
		DNS:              epInfo.DNS,
		VlanID:           vlanid,
		EnableSnatOnHost: epInfo.EnableSnatOnHost,
		EnableInfraVnet:  epInfo.EnableInfraVnet,
	}

	for _, route := range epInfo.Routes {
		ep.Routes = append(ep.Routes, route)
	}

	return ep, nil
}

// deleteEndpointImpl deletes an existing endpoint from the network.
func (nw *network) deleteEndpointImpl(ep *endpoint) error {
	var epClient EndpointClient

	// Delete the veth pair by deleting one of the peer interfaces.
	// Deleting the host interface is more convenient since it does not require
	// entering the container netns and hence works both for CNI and CNM.
	if ep.VlanID != 0 {
		epInfo := ep.getInfo()
		epClient = NewOVSEndpointClient(nw.extIf, epInfo, ep.HostIfName, "", ep.VlanID)
	} else {
		epClient = NewLinuxBridgeEndpointClient(nw.extIf, ep.HostIfName, "", nw.Mode)
	}

	epClient.DeleteEndpointRules(ep)
	epClient.DeleteEndpoints(ep)

	return nil
}

// getInfoImpl returns information about the endpoint.
func (ep *endpoint) getInfoImpl(epInfo *EndpointInfo) {
}

func addRoutes(interfaceName string, routes []RouteInfo) error {
	ifIndex := 0
	interfaceIf, _ := net.InterfaceByName(interfaceName)

	for _, route := range routes {
		log.Printf("[ovs] Adding IP route %+v to link %v.", route, interfaceName)

		if route.DevName != "" {
			devIf, _ := net.InterfaceByName(route.DevName)
			ifIndex = devIf.Index
		} else {
			ifIndex = interfaceIf.Index
		}

		nlRoute := &netlink.Route{
			Family:    netlink.GetIpAddressFamily(route.Gw),
			Dst:       &route.Dst,
			Gw:        route.Gw,
			LinkIndex: ifIndex,
		}

		if err := netlink.AddIpRoute(nlRoute); err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "file exists") {
				return err
			} else {
				log.Printf("route already exists")
			}
		}
	}

	return nil
}

func deleteRoutes(interfaceName string, routes []RouteInfo) error {
	ifIndex := 0
	interfaceIf, _ := net.InterfaceByName(interfaceName)

	for _, route := range routes {
		log.Printf("[ovs] Adding IP route %+v to link %v.", route, interfaceName)

		if route.DevName != "" {
			devIf, _ := net.InterfaceByName(route.DevName)
			ifIndex = devIf.Index
		} else {
			ifIndex = interfaceIf.Index
		}

		nlRoute := &netlink.Route{
			Family:    netlink.GetIpAddressFamily(route.Gw),
			Dst:       &route.Dst,
			Gw:        route.Gw,
			LinkIndex: ifIndex,
		}

		if err := netlink.DeleteIpRoute(nlRoute); err != nil {
			return err
		}
	}

	return nil
}
