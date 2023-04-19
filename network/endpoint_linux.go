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
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/ovsctl"
	"github.com/Azure/azure-container-networking/platform"
)

const (
	// Common prefix for all types of host network interface names.
	commonInterfacePrefix = "az"

	// Prefix for host virtual network interface names.
	hostVEthInterfacePrefix = commonInterfacePrefix + "v"
)

type AzureHNSEndpointClient interface{}

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
func (nw *network) newEndpointImpl(
	_ apipaClient,
	nl netlink.NetlinkInterface,
	plc platform.ExecClient,
	netioCli netio.NetIOInterface,
	epClient EndpointClient,
	epInfo *EndpointInfo,
) (*endpoint, error) {
	var containerIf *net.Interface
	var ns *Namespace
	var ep *endpoint
	var err error
	var hostIfName string
	var contIfName string
	var localIP string
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

	// epClient is non-nil only when the endpoint is created for the unit test.
	if epClient == nil {
		//nolint:gocritic
		if vlanid != 0 {
			if nw.Mode == opModeTransparentVlan {
				log.Printf("Transparent vlan client")
				if _, ok := epInfo.Data[SnatBridgeIPKey]; ok {
					nw.SnatBridgeIP = epInfo.Data[SnatBridgeIPKey].(string)
				}
				epClient = NewTransparentVlanEndpointClient(nw, epInfo, hostIfName, contIfName, vlanid, localIP, nl, plc)
			} else {
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
					localIP,
					nl,
					ovsctl.NewOvsctl(),
					plc)
			}
		} else if nw.Mode != opModeTransparent {
			log.Printf("Bridge client")
			epClient = NewLinuxBridgeEndpointClient(nw.extIf, hostIfName, contIfName, nw.Mode, nl, plc)
		} else {
			log.Printf("Transparent client")
			epClient = NewTransparentEndpointClient(nw.extIf, hostIfName, contIfName, nw.Mode, nl, plc)
		}
	}

	// Cleanup on failure.
	defer func() {
		if err != nil {
			log.Printf("CNI error:%v. Delete Endpoint %v and rules that are created.", err, contIfName)
			endpt := &endpoint{
				Id:                       epInfo.Id,
				IfName:                   contIfName,
				HostIfName:               hostIfName,
				LocalIP:                  localIP,
				IPAddresses:              epInfo.IPAddresses,
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

			if nw.extIf != nil {
				endpt.Gateways = []net.IP{nw.extIf.IPv4Gateway}
			}
			// set deleteHostVeth to true to cleanup host veth interface if created
			//nolint:errcheck // ignore error
			epClient.DeleteEndpoints(endpt)
		}
	}()

	if err = epClient.AddEndpoints(epInfo); err != nil {
		return nil, err
	}

	containerIf, err = netioCli.GetNetworkInterfaceByName(contIfName)
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

	if epInfo.IPV6Mode != "" {
		// Enable ipv6 setting in container
		log.Printf("Enable ipv6 setting in container.")
		nuc := networkutils.NewNetworkUtils(nl, plc)
		if err = nuc.UpdateIPV6Setting(0); err != nil {
			return nil, fmt.Errorf("Enable ipv6 in container failed:%w", err)
		}
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

	if nw.extIf != nil {
		ep.Gateways = []net.IP{nw.extIf.IPv4Gateway}
	}

	ep.Routes = append(ep.Routes, epInfo.Routes...)
	return ep, nil
}

// deleteEndpointImpl deletes an existing endpoint from the network.
func (nw *network) deleteEndpointImpl(nl netlink.NetlinkInterface, plc platform.ExecClient, epClient EndpointClient, ep *endpoint) error {
	// Delete the veth pair by deleting one of the peer interfaces.
	// Deleting the host interface is more convenient since it does not require
	// entering the container netns and hence works both for CNI and CNM.

	// epClient is nil only for unit test.
	if epClient == nil {
		//nolint:gocritic
		if ep.VlanID != 0 {
			epInfo := ep.getInfo()
			if nw.Mode == opModeTransparentVlan {
				log.Printf("Transparent vlan client")
				epClient = NewTransparentVlanEndpointClient(nw, epInfo, ep.HostIfName, "", ep.VlanID, ep.LocalIP, nl, plc)

			} else {
				epClient = NewOVSEndpointClient(nw, epInfo, ep.HostIfName, "", ep.VlanID, ep.LocalIP, nl, ovsctl.NewOvsctl(), plc)
			}
		} else if nw.Mode != opModeTransparent {
			epClient = NewLinuxBridgeEndpointClient(nw.extIf, ep.HostIfName, "", nw.Mode, nl, plc)
		} else {
			epClient = NewTransparentEndpointClient(nw.extIf, ep.HostIfName, "", nw.Mode, nl, plc)
		}
	}

	epClient.DeleteEndpointRules(ep)
	// deleteHostVeth set to false not to delete veth as CRI will remove network namespace and
	// veth will get removed as part of that.
	//nolint:errcheck // ignore error
	epClient.DeleteEndpoints(ep)

	return nil
}

// getInfoImpl returns information about the endpoint.
func (ep *endpoint) getInfoImpl(epInfo *EndpointInfo) {
}

func addRoutes(nl netlink.NetlinkInterface, netioshim netio.NetIOInterface, interfaceName string, routes []RouteInfo) error {
	ifIndex := 0

	for _, route := range routes {
		if route.DevName != "" {
			devIf, _ := netioshim.GetNetworkInterfaceByName(route.DevName)
			ifIndex = devIf.Index
		} else {
			interfaceIf, err := netioshim.GetNetworkInterfaceByName(interfaceName)
			if err != nil {
				log.Errorf("Interface not found:%v", err)
				return fmt.Errorf("addRoutes failed: %w", err)
			}
			ifIndex = interfaceIf.Index
		}

		family := netlink.GetIPAddressFamily(route.Gw)
		if route.Gw == nil {
			family = netlink.GetIPAddressFamily(route.Dst.IP)
		}

		nlRoute := &netlink.Route{
			Family:    family,
			Dst:       &route.Dst,
			Gw:        route.Gw,
			LinkIndex: ifIndex,
			Priority:  route.Priority,
			Protocol:  route.Protocol,
			Scope:     route.Scope,
			Table:     route.Table,
		}

		log.Printf("[net] Adding IP route %+v to link %v.", route, interfaceName)
		if err := nl.AddIPRoute(nlRoute); err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "file exists") {
				return err
			} else {
				log.Printf("[net] route already exists")
			}
		}
	}

	return nil
}

func deleteRoutes(nl netlink.NetlinkInterface, netioshim netio.NetIOInterface, interfaceName string, routes []RouteInfo) error {
	ifIndex := 0

	for _, route := range routes {
		if route.DevName != "" {
			devIf, _ := netioshim.GetNetworkInterfaceByName(route.DevName)
			if devIf == nil {
				log.Printf("[net] Not deleting route. Interface %v doesn't exist", interfaceName)
				continue
			}

			ifIndex = devIf.Index
		} else if interfaceName != "" {
			interfaceIf, _ := netioshim.GetNetworkInterfaceByName(interfaceName)
			if interfaceIf == nil {
				log.Printf("[net] Not deleting route. Interface %v doesn't exist", interfaceName)
				continue
			}
			ifIndex = interfaceIf.Index
		}

		family := netlink.GetIPAddressFamily(route.Gw)
		if route.Gw == nil {
			family = netlink.GetIPAddressFamily(route.Dst.IP)
		}

		nlRoute := &netlink.Route{
			Family:    family,
			Dst:       &route.Dst,
			LinkIndex: ifIndex,
			Gw:        route.Gw,
			Protocol:  route.Protocol,
			Scope:     route.Scope,
		}

		log.Printf("[net] Deleting IP route %+v from link %v.", route, interfaceName)
		if err := nl.DeleteIPRoute(nlRoute); err != nil {
			return err
		}
	}

	return nil
}

// updateEndpointImpl updates an existing endpoint in the network.
func (nm *networkManager) updateEndpointImpl(nw *network, existingEpInfo *EndpointInfo, targetEpInfo *EndpointInfo) (*endpoint, error) {
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
	if err = nm.updateRoutes(existingEpInfo, targetEpInfo); err != nil {
		return nil, err
	}

	// Create the endpoint object.
	ep = &endpoint{
		Id: existingEpInfo.Id,
	}

	// Update existing endpoint state with the new routes to persist
	ep.Routes = append(ep.Routes, targetEpInfo.Routes...)

	return ep, nil
}

func (nm *networkManager) updateRoutes(existingEp *EndpointInfo, targetEp *EndpointInfo) error {
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

	err := deleteRoutes(nm.netlink, &netio.NetIO{}, existingEp.IfName, tobeDeletedRoutes)
	if err != nil {
		return err
	}

	err = addRoutes(nm.netlink, &netio.NetIO{}, existingEp.IfName, tobeAddedRoutes)
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
