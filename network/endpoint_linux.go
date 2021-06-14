// Copyright 2017 Microsoft. All rights reserved.
// MIT License

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

func ConstructEndpointID(containerID string, _ string, ifName string) (string, string) {
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
	var localIP string
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

		if _, ok := epInfo.Data[LocalIPKey]; ok {
			localIP = epInfo.Data[LocalIPKey].(string)
		}
	}

	if _, ok := epInfo.Data[OptVethName]; ok {
		key := epInfo.Data[OptVethName].(string)
		log.Printf("Generate veth name based on the key provided %v", key)
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
		log.Printf("OVS client")
		if _, ok := epInfo.Data[SnatBridgeIPKey]; ok {
			nw.SnatBridgeIP = epInfo.Data[SnatBridgeIPKey].(string)
		}

		epClient = NewOVSEndpointClient(
			nw,
			epInfo,
			hostIfName,
			contIfName,
			vlanid,
			localIP)
	} else if nw.Mode != opModeTransparent {
		log.Printf("Bridge client")
		epClient = NewLinuxBridgeEndpointClient(nw.extIf, hostIfName, contIfName, nw.Mode)
	} else {
		log.Printf("Transparent client")
		epClient = NewTransparentEndpointClient(nw.extIf, hostIfName, contIfName, nw.Mode)
	}

	// Cleanup on failure.
	defer func() {
		if err != nil {
			log.Printf("CNI error. Delete Endpoint %v and rules that are created.", contIfName)
			endpt := &endpoint{
				Id:                       epInfo.Id,
				IfName:                   contIfName,
				HostIfName:               hostIfName,
				LocalIP:                  localIP,
				IPAddresses:              epInfo.IPAddresses,
				Gateways:                 []net.IP{nw.extIf.IPv4Gateway},
				DNS:                      epInfo.DNS,
				VlanID:                   vlanid,
				EnableSnatOnHost:         epInfo.EnableSnatOnHost,
				EnableMultitenancy:       epInfo.EnableMultiTenancy,
				AllowInboundFromHostToNC: epInfo.AllowInboundFromHostToNC,
				AllowInboundFromNCToHost: epInfo.AllowInboundFromNCToHost,
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
		Id:                       epInfo.Id,
		IfName:                   contIfName, // container veth pair name. In cnm, we won't rename this and docker expects veth name.
		HostIfName:               hostIfName,
		MacAddress:               containerIf.HardwareAddr,
		InfraVnetIP:              epInfo.InfraVnetIP,
		LocalIP:                  localIP,
		IPAddresses:              epInfo.IPAddresses,
		Gateways:                 []net.IP{nw.extIf.IPv4Gateway},
		DNS:                      epInfo.DNS,
		VlanID:                   vlanid,
		EnableSnatOnHost:         epInfo.EnableSnatOnHost,
		EnableInfraVnet:          epInfo.EnableInfraVnet,
		EnableMultitenancy:       epInfo.EnableMultiTenancy,
		AllowInboundFromHostToNC: epInfo.AllowInboundFromHostToNC,
		AllowInboundFromNCToHost: epInfo.AllowInboundFromNCToHost,
		NetworkNameSpace:         epInfo.NetNsPath,
		ContainerID:              epInfo.ContainerID,
		PODName:                  epInfo.PODName,
		PODNameSpace:             epInfo.PODNameSpace,
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
		epClient = NewOVSEndpointClient(nw, epInfo, ep.HostIfName, "", ep.VlanID, ep.LocalIP)
	} else if nw.Mode != opModeTransparent {
		epClient = NewLinuxBridgeEndpointClient(nw.extIf, ep.HostIfName, "", nw.Mode)
	} else {
		epClient = NewTransparentEndpointClient(nw.extIf, ep.HostIfName, "", nw.Mode)
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
		log.Printf("[net] Adding IP route %+v to link %v.", route, interfaceName)

		if route.DevName != "" {
			devIf, _ := net.InterfaceByName(route.DevName)
			ifIndex = devIf.Index
		} else {
			ifIndex = interfaceIf.Index
		}

		family := netlink.GetIpAddressFamily(route.Gw)
		if route.Gw == nil {
			family = netlink.GetIpAddressFamily(route.Dst.IP)
		}

		nlRoute := &netlink.Route{
			Family:    family,
			Dst:       &route.Dst,
			Gw:        route.Gw,
			LinkIndex: ifIndex,
			Priority:  route.Priority,
			Protocol:  route.Protocol,
			Scope:     route.Scope,
		}

		if err := netlink.AddIpRoute(nlRoute); err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "file exists") {
				return err
			} else {
				log.Printf("[net] route already exists")
			}
		}
	}

	return nil
}

func deleteRoutes(interfaceName string, routes []RouteInfo) error {
	ifIndex := 0
	interfaceIf, _ := net.InterfaceByName(interfaceName)

	for _, route := range routes {
		log.Printf("[net] Deleting IP route %+v from link %v.", route, interfaceName)

		if route.DevName != "" {
			devIf, _ := net.InterfaceByName(route.DevName)
			if devIf == nil {
				log.Printf("[net] Not deleting route. Interface %v doesn't exist", interfaceName)
				continue
			}

			ifIndex = devIf.Index
		} else {
			if interfaceIf == nil {
				log.Printf("[net] Not deleting route. Interface %v doesn't exist", interfaceName)
				continue
			}

			ifIndex = interfaceIf.Index
		}

		nlRoute := &netlink.Route{
			Family:    netlink.GetIpAddressFamily(route.Gw),
			Dst:       &route.Dst,
			Gw:        route.Gw,
			LinkIndex: ifIndex,
			Protocol:  route.Protocol,
			Scope:     route.Scope,
		}

		if err := netlink.DeleteIpRoute(nlRoute); err != nil {
			return err
		}
	}

	return nil
}

// updateEndpointImpl updates an existing endpoint in the network.
func (nw *network) updateEndpointImpl(existingEpInfo *EndpointInfo, targetEpInfo *EndpointInfo) (*endpoint, error) {
	var ns *Namespace
	var ep *endpoint
	var err error

	existingEpFromRepository := nw.Endpoints[existingEpInfo.Id]
	log.Printf("[updateEndpointImpl] Going to retrieve endpoint with Id %+v to update.", existingEpInfo.Id)
	if existingEpFromRepository == nil {
		log.Printf("[updateEndpointImpl] Endpoint cannot be updated as it does not exist.")
		err = errEndpointNotFound
		return nil, err
	}

	netns := existingEpFromRepository.NetworkNameSpace
	// Network namespace for the container interface has to be specified
	if netns != "" {
		// Open the network namespace.
		log.Printf("[updateEndpointImpl] Opening netns %v.", netns)
		ns, err = OpenNamespace(netns)
		if err != nil {
			return nil, err
		}
		defer ns.Close()

		// Enter the container network namespace.
		log.Printf("[updateEndpointImpl] Entering netns %v.", netns)
		if err = ns.Enter(); err != nil {
			return nil, err
		}

		// Return to host network namespace.
		defer func() {
			log.Printf("[updateEndpointImpl] Exiting netns %v.", netns)
			if err := ns.Exit(); err != nil {
				log.Printf("[updateEndpointImpl] Failed to exit netns, err:%v.", err)
			}
		}()
	} else {
		log.Printf("[updateEndpointImpl] Endpoint cannot be updated as the network namespace does not exist: Epid: %v", existingEpInfo.Id)
		err = errNamespaceNotFound
		return nil, err
	}

	log.Printf("[updateEndpointImpl] Going to update routes in netns %v.", netns)
	if err = updateRoutes(existingEpInfo, targetEpInfo); err != nil {
		return nil, err
	}

	// Create the endpoint object.
	ep = &endpoint{
		Id: existingEpInfo.Id,
	}

	// Update existing endpoint state with the new routes to persist
	for _, route := range targetEpInfo.Routes {
		ep.Routes = append(ep.Routes, route)
	}

	return ep, nil
}

func updateRoutes(existingEp *EndpointInfo, targetEp *EndpointInfo) error {
	log.Printf("Updating routes for the endpoint %+v.", existingEp)
	log.Printf("Target endpoint is %+v", targetEp)

	existingRoutes := make(map[string]RouteInfo)
	targetRoutes := make(map[string]RouteInfo)
	var tobeDeletedRoutes []RouteInfo
	var tobeAddedRoutes []RouteInfo

	// we should not remove default route from container if it exists
	// we do not support enable/disable snat for now
	defaultDst := net.ParseIP("0.0.0.0")

	log.Printf("Going to collect routes and skip default and infravnet routes if applicable.")
	log.Printf("Key for default route: %+v", defaultDst.String())

	infraVnetKey := ""
	if targetEp.EnableInfraVnet {
		infraVnetSubnet := targetEp.InfraVnetAddressSpace
		if infraVnetSubnet != "" {
			infraVnetKey = strings.Split(infraVnetSubnet, "/")[0]
		}
	}

	log.Printf("Key for route to infra vnet: %+v", infraVnetKey)
	for _, route := range existingEp.Routes {
		destination := route.Dst.IP.String()
		log.Printf("Checking destination as %+v to skip or not", destination)
		isDefaultRoute := destination == defaultDst.String()
		isInfraVnetRoute := targetEp.EnableInfraVnet && (destination == infraVnetKey)
		if !isDefaultRoute && !isInfraVnetRoute {
			existingRoutes[route.Dst.String()] = route
			log.Printf("%+v was skipped", destination)
		}
	}

	for _, route := range targetEp.Routes {
		targetRoutes[route.Dst.String()] = route
	}

	for _, existingRoute := range existingRoutes {
		dst := existingRoute.Dst.String()
		if _, ok := targetRoutes[dst]; !ok {
			tobeDeletedRoutes = append(tobeDeletedRoutes, existingRoute)
			log.Printf("Adding following route to the tobeDeleted list: %+v", existingRoute)
		}
	}

	for _, targetRoute := range targetRoutes {
		dst := targetRoute.Dst.String()
		if _, ok := existingRoutes[dst]; !ok {
			tobeAddedRoutes = append(tobeAddedRoutes, targetRoute)
			log.Printf("Adding following route to the tobeAdded list: %+v", targetRoute)
		}

	}

	err := deleteRoutes(existingEp.IfName, tobeDeletedRoutes)
	if err != nil {
		return err
	}

	err = addRoutes(existingEp.IfName, tobeAddedRoutes)
	if err != nil {
		return err
	}

	log.Printf("Successfully updated routes for the endpoint %+v using target: %+v", existingEp, targetEp)

	return nil
}

func getDefaultGateway(routes []RouteInfo) net.IP {
	_, defDstIP, _ := net.ParseCIDR("0.0.0.0/0")
	for _, route := range routes {
		if route.Dst.String() == defDstIP.String() {
			return route.Gw
		}
	}

	return nil
}
