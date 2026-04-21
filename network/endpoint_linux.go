// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/ovsctl"
	"github.com/Azure/azure-container-networking/platform"
	"go.uber.org/zap"
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
		logger.Info("Container ID is not greater than 8 ID", zap.String("containerID", containerID))
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
	testEpClient EndpointClient,
	nsc NamespaceClientInterface,
	iptc ipTablesClient,
	dhcpclient dhcpClient,
	epInfo *EndpointInfo,
) (*endpoint, error) {
	var (
		err         error
		hostIfName  string
		contIfName  string
		localIP     string
		vlanid      = 0
		containerIf *net.Interface
	)

	if nw.Endpoints[epInfo.EndpointID] != nil {
		logger.Info("[net] Endpoint already exists.")
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
		logger.Info("Generate veth name based on the key provided", zap.String("key", key))
		vethname := generateVethName(key)
		hostIfName = fmt.Sprintf("%s%s", hostVEthInterfacePrefix, vethname)
		// we don't save the contIfName here or below because it will be renamed to ep.IfName anyway when we call SetupContainerInterfaces in the clients
		// however, contIfName is still passed into our clients
		contIfName = fmt.Sprintf("%s%s2", hostVEthInterfacePrefix, vethname)
	} else {
		// Create a veth pair.
		logger.Info("Generate veth name based on endpoint id")
		hostIfName = fmt.Sprintf("%s%s", hostVEthInterfacePrefix, epInfo.EndpointID[:7])
		contIfName = fmt.Sprintf("%s%s-2", hostVEthInterfacePrefix, epInfo.EndpointID[:7])
	}

	nicName := epInfo.IfName
	// infra nic nicname will look like eth0, and delegated/secondary nics will be moved into the container namespace
	if epInfo.NICType != cns.InfraNIC {
		nicName = epInfo.MasterIfName
	}

	ep := &endpoint{
		Id: epInfo.EndpointID,
		// IfName should end up being eth0 in non-delegated nic cases
		IfName:                   nicName, // container veth pair name. In cnm, we won't rename this and docker expects veth name.
		HostIfName:               hostIfName,
		InfraVnetIP:              epInfo.InfraVnetIP,
		LocalIP:                  localIP,
		IPAddresses:              epInfo.IPAddresses,
		DNS:                      epInfo.EndpointDNS,
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
		Routes:                   epInfo.Routes,
		SecondaryInterfaces:      make(map[string]*InterfaceInfo),
		NICType:                  epInfo.NICType,
	}
	if nw.extIf != nil {
		ep.Gateways = []net.IP{nw.extIf.IPv4Gateway}
	}

	// testEpClient is non-nil only when the endpoint is created for the unit test
	// resetting epClient to testEpClient in loop to use the test endpoint client if specified
	epClient := testEpClient
	if epClient == nil {
		//nolint:gocritic
		if vlanid != 0 {
			if epInfo.Mode == opModeTransparentVlan {
				logger.Info("Transparent vlan client")
				if _, ok := epInfo.Data[SnatBridgeIPKey]; ok {
					nw.SnatBridgeIP = epInfo.Data[SnatBridgeIPKey].(string)
				}
				epClient = NewTransparentVlanEndpointClient(nw, epInfo, hostIfName, contIfName, vlanid, localIP, nl, plc, nsc, iptc)
			} else {
				logger.Info("OVS client")
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
					plc,
					iptc)
			}
		} else if epInfo.Mode != opModeTransparent {
			logger.Info("Bridge client")
			epClient = NewLinuxBridgeEndpointClient(nw.extIf, hostIfName, contIfName, epInfo.Mode, nl, plc)
		} else if epInfo.NICType == cns.NodeNetworkInterfaceFrontendNIC {
			logger.Info("Secondary client")
			epClient = NewSecondaryEndpointClient(nl, netioCli, plc, nsc, dhcpclient, ep)
		} else {
			logger.Info("Transparent client")
			epClient = NewTransparentEndpointClient(nw.extIf, hostIfName, contIfName, epInfo.Mode, nl, netioCli, plc)
		}
	}

	//nolint:gocritic
	defer func(client EndpointClient, contIfName string) {
		// Cleanup on failure.
		if err != nil {
			logger.Error("CNI error. Delete Endpoint and rules that are created", zap.Error(err), zap.String("contIfName", contIfName))
			if containerIf != nil {
				client.DeleteEndpointRules(ep)
			}
			// set deleteHostVeth to true to cleanup host veth interface if created
			//nolint:errcheck // ignore error
			client.DeleteEndpoints(ep)
		}
	}(epClient, contIfName)

	// wrapping endpoint client commands in anonymous func so that namespace can be exit and closed before the next loop
	//nolint:wrapcheck // ignore wrap check
	err = func() error {
		if epErr := epClient.AddEndpoints(epInfo); epErr != nil {
			return epErr
		}

		if epInfo.NICType == cns.InfraNIC {
			var epErr error
			containerIf, epErr = netioCli.GetNetworkInterfaceByName(contIfName)
			if epErr != nil {
				return epErr
			}
			ep.MacAddress = containerIf.HardwareAddr
		}

		// Setup rules for IP addresses on the container interface.
		if epErr := epClient.AddEndpointRules(epInfo); epErr != nil {
			return epErr
		}

		// If a network namespace for the container interface is specified...
		if epInfo.NetNsPath != "" {
			// Open the network namespace.
			logger.Info("Opening netns", zap.Any("NetNsPath", epInfo.NetNsPath))
			ns, epErr := nsc.OpenNamespace(epInfo.NetNsPath)
			if epErr != nil {
				return epErr
			}
			defer ns.Close()

			if epErr := epClient.MoveEndpointsToContainerNS(epInfo, ns.GetFd()); epErr != nil {
				return epErr
			}

			// Enter the container network namespace.
			logger.Info("Entering netns", zap.Any("NetNsPath", epInfo.NetNsPath))
			if epErr := ns.Enter(); epErr != nil {
				return epErr
			}

			// Return to host network namespace.
			defer func() {
				logger.Info("Exiting netns", zap.Any("NetNsPath", epInfo.NetNsPath))
				if epErr := ns.Exit(); epErr != nil {
					logger.Error("Failed to exit netns with", zap.Error(epErr))
				}
			}()
		}

		if epInfo.IPV6Mode != "" {
			// Enable ipv6 setting in container
			logger.Info("Enable ipv6 setting in container.")
			nuc := networkutils.NewNetworkUtils(nl, plc)
			if epErr := nuc.UpdateIPV6Setting(0); epErr != nil {
				return fmt.Errorf("enable ipv6 in container failed:%w", epErr)
			}
		}

		// If a name for the container interface is specified...
		if epInfo.IfName != "" {
			if epErr := epClient.SetupContainerInterfaces(epInfo); epErr != nil {
				return epErr
			}
		}

		return epClient.ConfigureContainerInterfacesAndRoutes(epInfo)
	}()
	if err != nil {
		return nil, err
	}

	return ep, nil
}

// deleteEndpointImpl deletes an existing endpoint from the network.
func (nw *network) deleteEndpointImpl(nl netlink.NetlinkInterface, plc platform.ExecClient, epClient EndpointClient, nioc netio.NetIOInterface, nsc NamespaceClientInterface,
	iptc ipTablesClient, dhcpc dhcpClient, ep *endpoint, mode string,
) error {
	// Delete the veth pair by deleting one of the peer interfaces.
	// Deleting the host interface is more convenient since it does not require
	// entering the container netns and hence works both for CNI and CNM.

	// epClient is nil only for unit test.
	if epClient == nil {
		//nolint:gocritic
		if ep.VlanID != 0 {
			epInfo := ep.getInfo()
			if mode == opModeTransparentVlan {
				epClient = NewTransparentVlanEndpointClient(nw, epInfo, ep.HostIfName, "", ep.VlanID, ep.LocalIP, nl, plc, nsc, iptc)
			} else {
				epClient = NewOVSEndpointClient(nw, epInfo, ep.HostIfName, "", ep.VlanID, ep.LocalIP, nl, ovsctl.NewOvsctl(), plc, iptc)
			}
		} else if mode != opModeTransparent {
			epClient = NewLinuxBridgeEndpointClient(nw.extIf, ep.HostIfName, "", mode, nl, plc)
		} else {
			// delete if secondary interfaces populated or endpoint of type delegated (new way)
			if len(ep.SecondaryInterfaces) > 0 || ep.NICType == cns.NodeNetworkInterfaceFrontendNIC {
				epClient = NewSecondaryEndpointClient(nl, nioc, plc, nsc, dhcpc, ep)
				epClient.DeleteEndpointRules(ep)
				//nolint:errcheck // ignore error
				epClient.DeleteEndpoints(ep)
				if ep.NICType == cns.NodeNetworkInterfaceFrontendNIC {
					// if the ep itself is of type secondary (new way), don't use transparent client below
					return nil
				}
			}

			epClient = NewTransparentEndpointClient(nw.extIf, ep.HostIfName, "", mode, nl, nioc, plc)
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
				logger.Error("Interface not found with", zap.Error(err))
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

		logger.Info("Adding IP route to link", zap.Any("route", route), zap.String("interfaceName", interfaceName))
		if err := nl.AddIPRoute(nlRoute); err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "file exists") {
				return err
			} else {
				logger.Info("route already exists")
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
				logger.Info("Not deleting route. Interface doesn't exist", zap.String("interfaceName", interfaceName))
				continue
			}

			ifIndex = devIf.Index
		} else if interfaceName != "" {
			interfaceIf, _ := netioshim.GetNetworkInterfaceByName(interfaceName)
			if interfaceIf == nil {
				logger.Info("Not deleting route. Interface doesn't exist", zap.String("interfaceName", interfaceName))
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

		logger.Info("Deleting IP route from link", zap.Any("route", route), zap.String("interfaceName", interfaceName))
		if err := nl.DeleteIPRoute(nlRoute); err != nil {
			return err
		}
	}

	return nil
}

// updateEndpointImpl updates an existing endpoint in the network.
func (nm *networkManager) updateEndpointImpl(nw *network, existingEpInfo *EndpointInfo, targetEpInfo *EndpointInfo) (*endpoint, error) {
	var ep *endpoint

	existingEpFromRepository := nw.Endpoints[existingEpInfo.EndpointID]
	logger.Info("[updateEndpointImpl] Going to retrieve endpoint with Id to update", zap.String("id", existingEpInfo.EndpointID))
	if existingEpFromRepository == nil {
		logger.Info("[updateEndpointImpl] Endpoint cannot be updated as it does not exist")
		return nil, errEndpointNotFound
	}

	netns := existingEpFromRepository.NetworkNameSpace
	// Network namespace for the container interface has to be specified
	if netns != "" {
		// Open the network namespace.
		logger.Info("[updateEndpointImpl] Opening netns", zap.Any("netns", netns))
		ns, err := nm.nsClient.OpenNamespace(netns)
		if err != nil {
			return nil, err
		}
		defer ns.Close()

		// Enter the container network namespace.
		logger.Info("[updateEndpointImpl] Entering netns", zap.Any("netns", netns))
		if err = ns.Enter(); err != nil {
			return nil, err
		}

		// Return to host network namespace.
		defer func() {
			logger.Info("[updateEndpointImpl] Exiting netns", zap.Any("netns", netns))
			if err := ns.Exit(); err != nil {
				logger.Error("[updateEndpointImpl] Failed to exit netns with", zap.Error(err))
			}
		}()
	} else {
		logger.Info("[updateEndpointImpl] Endpoint cannot be updated as the network namespace does not exist: Epid", zap.String("id", existingEpInfo.EndpointID),
			zap.String("component", "updateEndpointImpl"))
		return nil, errNamespaceNotFound
	}

	logger.Info("[updateEndpointImpl] Going to update routes in netns", zap.Any("netns", netns))
	if err := nm.updateRoutes(existingEpInfo, targetEpInfo); err != nil {
		return nil, err
	}

	// Create the endpoint object.
	ep = &endpoint{
		Id: existingEpInfo.EndpointID,
	}

	// Update existing endpoint state with the new routes to persist
	ep.Routes = append(ep.Routes, targetEpInfo.Routes...)

	return ep, nil
}

func (nm *networkManager) updateRoutes(existingEp *EndpointInfo, targetEp *EndpointInfo) error {
	logger.Info("Updating routes for the endpoint", zap.Any("existingEp", existingEp))
	logger.Info("Target endpoint is", zap.Any("targetEp", targetEp))

	existingRoutes := make(map[string]RouteInfo)
	targetRoutes := make(map[string]RouteInfo)
	var tobeDeletedRoutes []RouteInfo
	var tobeAddedRoutes []RouteInfo

	// we should not remove default route from container if it exists
	// we do not support enable/disable snat for now
	defaultDst := net.ParseIP("0.0.0.0")

	// collect routes and skip default and infravnet routes if applicable
	logger.Info("Key for default route", zap.String("route", defaultDst.String()))

	infraVnetKey := ""
	if targetEp.EnableInfraVnet {
		infraVnetSubnet := targetEp.InfraVnetAddressSpace
		if infraVnetSubnet != "" {
			infraVnetKey = strings.Split(infraVnetSubnet, "/")[0]
		}
	}

	logger.Info("Key for route to infra vnet", zap.String("infraVnetKey", infraVnetKey))
	for _, route := range existingEp.Routes {
		destination := route.Dst.IP.String()
		isDefaultRoute := destination == defaultDst.String()
		isInfraVnetRoute := targetEp.EnableInfraVnet && (destination == infraVnetKey)
		if !isDefaultRoute && !isInfraVnetRoute {
			existingRoutes[route.Dst.String()] = route
			logger.Info("was skipped", zap.String("destination", destination))
		}
	}

	for _, route := range targetEp.Routes {
		targetRoutes[route.Dst.String()] = route
	}

	for _, existingRoute := range existingRoutes {
		dst := existingRoute.Dst.String()
		if _, ok := targetRoutes[dst]; !ok {
			tobeDeletedRoutes = append(tobeDeletedRoutes, existingRoute)
			logger.Info("Adding following route to the tobeDeleted list", zap.Any("existingRoute", existingRoute))
		}
	}

	for _, targetRoute := range targetRoutes {
		dst := targetRoute.Dst.String()
		if _, ok := existingRoutes[dst]; !ok {
			tobeAddedRoutes = append(tobeAddedRoutes, targetRoute)
			logger.Info("Adding following route to the tobeAdded list", zap.Any("targetRoute", targetRoute))
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

	logger.Info("Successfully updated routes for the endpoint using target", zap.Any("existingEp", existingEp), zap.Any("targetEp", targetEp))

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

// GetEndpointInfoByIPImpl returns an endpointInfo that contains corresponding HostVethName.
// TODO: It needs to be tested to see if HostVethName is required for SingleTenancy, WorkItem: 26606939
func (epInfo *EndpointInfo) GetEndpointInfoByIPImpl(_ []net.IPNet, _ string) (*EndpointInfo, error) {
	return epInfo, nil
}
