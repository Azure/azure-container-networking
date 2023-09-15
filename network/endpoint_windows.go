// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Microsoft/hcsshim"
	"github.com/Microsoft/hcsshim/hcn"
	"go.uber.org/zap"
)

const (
	// hcnSchemaVersionMajor indicates major version number for hcn schema
	hcnSchemaVersionMajor = 2

	// hcnSchemaVersionMinor indicates minor version number for hcn schema
	hcnSchemaVersionMinor = 0

	// hcnIpamTypeStatic indicates the static type of ipam
	hcnIpamTypeStatic = "Static"

	// Default gateway Mac
	defaultGwMac = "12-34-56-78-9a-bc"

	// Container interface name prefix
	containerIfNamePrefix = "vEthernet"

	// hostNCApipaEndpointName indicates the prefix for the name of the apipa endpoint used for
	// the host container connectivity
	hostNCApipaEndpointNamePrefix = "HostNCApipaEndpoint"
)

// ConstructEndpointID constructs endpoint name from netNsPath.
func ConstructEndpointID(containerID string, netNsPath string, ifName string) (string, string) {
	if len(containerID) > 8 {
		containerID = containerID[:8]
	}

	infraEpName, workloadEpName := "", ""

	splits := strings.Split(netNsPath, ":")
	if len(splits) == 2 {
		// For workload containers, we extract its linking infrastructure container ID.
		if len(splits[1]) > 8 {
			splits[1] = splits[1][:8]
		}
		infraEpName = splits[1] + "-" + ifName
		workloadEpName = containerID + "-" + ifName
	} else {
		// For infrastructure containers, we use its container ID directly.
		infraEpName = containerID + "-" + ifName
	}

	return infraEpName, workloadEpName
}

// newEndpointImpl creates a new endpoint in the network.
func (nw *network) newEndpointImpl(cli apipaClient, _ netlink.NetlinkInterface, _ platform.ExecClient, _ netio.NetIOInterface, _ EndpointClient, epInfo *EndpointInfo) (*endpoint, error) {
	if useHnsV2, err := UseHnsV2(epInfo.NetNsPath); useHnsV2 {
		if err != nil {
			return nil, err
		}

		return nw.newEndpointImplHnsV2(cli, epInfo)
	}

	return nw.newEndpointImplHnsV1(epInfo)
}

// newEndpointImplHnsV1 creates a new endpoint in the network using HnsV1
func (nw *network) newEndpointImplHnsV1(epInfo *EndpointInfo) (*endpoint, error) {
	var vlanid int

	if epInfo.Data != nil {
		if _, ok := epInfo.Data[VlanIDKey]; ok {
			vlanid = epInfo.Data[VlanIDKey].(int)
		}
	}

	// Get Infrastructure containerID. Handle ADD calls for workload container.
	var err error
	infraEpName, _ := ConstructEndpointID(epInfo.ContainerID, epInfo.NetNsPath, epInfo.IfName)
	hnsEndpoint := &hcsshim.HNSEndpoint{
		Name:           infraEpName,
		VirtualNetwork: nw.HnsId,
		DNSSuffix:      epInfo.DNS.Suffix,
		DNSServerList:  strings.Join(epInfo.DNS.Servers, ","),
		Policies:       policy.SerializePolicies(policy.EndpointPolicy, epInfo.Policies, epInfo.Data, epInfo.EnableSnatForDns, epInfo.EnableMultiTenancy),
	}

	// HNS currently supports one IP address and one IPv6 address per endpoint.

	for _, ipAddr := range epInfo.IPAddresses {
		if ipAddr.IP.To4() != nil {
			hnsEndpoint.IPAddress = ipAddr.IP
			pl, _ := ipAddr.Mask.Size()
			hnsEndpoint.PrefixLength = uint8(pl)
		} else {
			hnsEndpoint.IPv6Address = ipAddr.IP
			pl, _ := ipAddr.Mask.Size()
			hnsEndpoint.IPv6PrefixLength = uint8(pl)
			if len(nw.Subnets) > 1 {
				hnsEndpoint.GatewayAddressV6 = nw.Subnets[1].Gateway.String()
			}
		}
	}

	hnsResponse, err := Hnsv1.CreateEndpoint(hnsEndpoint, "")
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			logger.Info("HNSEndpointRequest DELETE id", zap.String("id", hnsResponse.Id))
			hnsResponse, err := Hnsv1.DeleteEndpoint(hnsResponse.Id)
			logger.Error("HNSEndpointRequest DELETE response", zap.Any("hnsResponse", hnsResponse), zap.Error(err))
		}
	}()

	if epInfo.SkipHotAttachEp {
		logger.Info("Skipping attaching the endpoint to container",
			zap.String("id", hnsResponse.Id), zap.String("id", epInfo.ContainerID))
	} else {
		// Attach the endpoint.
		logger.Info("Attaching endpoint to container", zap.String("id", hnsResponse.Id), zap.String("ContainerID", epInfo.ContainerID))
		err = Hnsv1.HotAttachEndpoint(epInfo.ContainerID, hnsResponse.Id)
		if err != nil {
			logger.Error("Failed to attach endpoint", zap.Error(err))
			return nil, err
		}
	}

	// add ipv6 neighbor entry for gateway IP to default mac in container
	if err := nw.addIPv6NeighborEntryForGateway(epInfo); err != nil {
		return nil, err
	}

	// Create the endpoint object.
	ep := &endpoint{
		Id:               infraEpName,
		HnsId:            hnsResponse.Id,
		SandboxKey:       epInfo.ContainerID,
		IfName:           epInfo.IfName,
		IPAddresses:      epInfo.IPAddresses,
		Gateways:         []net.IP{net.ParseIP(hnsResponse.GatewayAddress)},
		DNS:              epInfo.DNS,
		VlanID:           vlanid,
		EnableSnatOnHost: epInfo.EnableSnatOnHost,
		NetNs:            epInfo.NetNsPath,
		ContainerID:      epInfo.ContainerID,
	}

	for _, route := range epInfo.Routes {
		ep.Routes = append(ep.Routes, route)
	}

	ep.MacAddress, _ = net.ParseMAC(hnsResponse.MacAddress)

	return ep, nil
}

func (nw *network) addIPv6NeighborEntryForGateway(epInfo *EndpointInfo) error {
	var (
		err error
		out string
	)

	if epInfo.IPV6Mode == IPV6Nat {
		if len(nw.Subnets) < 2 {
			return fmt.Errorf("Ipv6 subnet not found in network state")
		}

		// run powershell cmd to set neighbor entry for gw ip to 12-34-56-78-9a-bc
		cmd := fmt.Sprintf("New-NetNeighbor -IPAddress %s -InterfaceAlias \"%s (%s)\" -LinkLayerAddress \"%s\"",
			nw.Subnets[1].Gateway.String(), containerIfNamePrefix, epInfo.Id, defaultGwMac)
		if out, err = platform.ExecutePowershellCommand(cmd); err != nil {
			logger.Error("Adding ipv6 gw neigh entry failed", zap.Any("out", out), zap.Error(err))
			return err
		}
	}

	return err
}

// configureHcnEndpoint configures hcn endpoint for creation
func (nw *network) configureHcnEndpoint(epInfo *EndpointInfo) (*hcn.HostComputeEndpoint, error) {
	infraEpName, _ := ConstructEndpointID(epInfo.ContainerID, epInfo.NetNsPath, epInfo.IfName)

	hcnEndpoint := &hcn.HostComputeEndpoint{
		Name:               infraEpName,
		HostComputeNetwork: nw.HnsId,
		Dns: hcn.Dns{
			Search:     strings.Split(epInfo.DNS.Suffix, ","),
			ServerList: epInfo.DNS.Servers,
			Options:    epInfo.DNS.Options,
		},
		SchemaVersion: hcn.SchemaVersion{
			Major: hcnSchemaVersionMajor,
			Minor: hcnSchemaVersionMinor,
		},
		MacAddress: epInfo.MacAddress.String(),
	}

	if endpointPolicies, err := policy.GetHcnEndpointPolicies(policy.EndpointPolicy, epInfo.Policies, epInfo.Data, epInfo.EnableSnatForDns, epInfo.EnableMultiTenancy, epInfo.NATInfo); err == nil {
		for _, epPolicy := range endpointPolicies {
			hcnEndpoint.Policies = append(hcnEndpoint.Policies, epPolicy)
		}
	} else {
		logger.Error("Failed to get endpoint policies due to", zap.Error(err))
		return nil, err
	}

	for _, route := range epInfo.Routes {
		hcnRoute := hcn.Route{
			NextHop:           route.Gw.String(),
			DestinationPrefix: route.Dst.String(),
		}

		hcnEndpoint.Routes = append(hcnEndpoint.Routes, hcnRoute)
	}

	for _, ipAddress := range epInfo.IPAddresses {
		prefixLength, _ := ipAddress.Mask.Size()
		ipConfiguration := hcn.IpConfig{
			IpAddress:    ipAddress.IP.String(),
			PrefixLength: uint8(prefixLength),
		}

		hcnEndpoint.IpConfigurations = append(hcnEndpoint.IpConfigurations, ipConfiguration)
	}

	return hcnEndpoint, nil
}

func (nw *network) deleteHostNCApipaEndpoint(networkContainerID string) error {
	// TODO: this code is duplicated in cns/hnsclient, but that code has logging messages that require a CNSLogger,
	// which makes is hard to use in this package. We should refactor this into a common package with no logging deps
	// so it can be called in both places

	// HostNCApipaEndpoint name is derived from NC ID
	endpointName := fmt.Sprintf("%s-%s", hostNCApipaEndpointNamePrefix, networkContainerID)
	logger.Info("Deleting HostNCApipaEndpoint for NC", zap.String("endpointName", endpointName), zap.String("networkContainerID", networkContainerID))

	// Check if the endpoint exists
	endpoint, err := Hnsv2.GetEndpointByName(endpointName)
	if err != nil {
		// If error is anything other than EndpointNotFoundError, return error.
		// else log the error but don't return error because endpoint is already deleted.
		if _, endpointNotFound := err.(hcn.EndpointNotFoundError); !endpointNotFound {
			return fmt.Errorf("deleteEndpointByNameHnsV2 failed due to error with GetEndpointByName: %w", err)
		}

		logger.Error("Delete called on the Endpoint which doesn't exist. Error:", zap.String("endpointName", endpointName), zap.Error(err))
		return nil
	}

	if err := Hnsv2.DeleteEndpoint(endpoint); err != nil {
		return fmt.Errorf("failed to delete HostNCApipa endpoint: %+v: %w", endpoint, err)
	}

	logger.Info("Successfully deleted HostNCApipa endpoint", zap.Any("endpoint", endpoint))

	return nil
}

// createHostNCApipaEndpoint creates a new endpoint in the HostNCApipaNetwork
// for host container connectivity
func (nw *network) createHostNCApipaEndpoint(cli apipaClient, epInfo *EndpointInfo) error {
	var (
		err                   error
		hostNCApipaEndpointID string
		namespace             *hcn.HostComputeNamespace
	)

	if namespace, err = hcn.GetNamespaceByID(epInfo.NetNsPath); err != nil {
		return fmt.Errorf("Failed to retrieve namespace with GetNamespaceByID for NetNsPath: %s"+
			" due to error: %v", epInfo.NetNsPath, err)
	}

	logger.Info("Creating HostNCApipaEndpoint for host container connectivity for NC",
		zap.String("NetworkContainerID", epInfo.NetworkContainerID))

	if hostNCApipaEndpointID, err = cli.CreateHostNCApipaEndpoint(context.TODO(), epInfo.NetworkContainerID); err != nil {
		return err
	}

	defer func() {
		if err != nil {
			nw.deleteHostNCApipaEndpoint(epInfo.NetworkContainerID)
		}
	}()

	if err = hcn.AddNamespaceEndpoint(namespace.Id, hostNCApipaEndpointID); err != nil {
		return fmt.Errorf("Failed to add HostNCApipaEndpoint: %s to namespace: %s due to error: %v", hostNCApipaEndpointID, namespace.Id, err) //nolint
	}

	return nil
}

// newEndpointImplHnsV2 creates a new endpoint in the network using Hnsv2
func (nw *network) newEndpointImplHnsV2(cli apipaClient, epInfo *EndpointInfo) (*endpoint, error) {
	hcnEndpoint, err := nw.configureHcnEndpoint(epInfo)
	if err != nil {
		logger.Error("Failed to configure hcn endpoint due to", zap.Error(err))
		return nil, err
	}

	// Create the HCN endpoint.
	logger.Info("Creating hcn endpoint", zap.String("name", hcnEndpoint.Name), zap.String("computenetwork", hcnEndpoint.HostComputeNetwork))
	hnsResponse, err := Hnsv2.CreateEndpoint(hcnEndpoint)
	if err != nil {
		return nil, fmt.Errorf("Failed to create endpoint: %s due to error: %v", hcnEndpoint.Name, err)
	}

	logger.Info("Successfully created hcn endpoint with response", zap.Any("hnsResponse", hnsResponse))

	defer func() {
		if err != nil {
			logger.Info("Deleting hcn endpoint with id", zap.String("id", hnsResponse.Id))
			err = Hnsv2.DeleteEndpoint(hnsResponse)
			logger.Error("Completed hcn endpoint deletion for id with error", zap.String("id", hnsResponse.Id), zap.Error(err))
		}
	}()

	var namespace *hcn.HostComputeNamespace
	if namespace, err = Hnsv2.GetNamespaceByID(epInfo.NetNsPath); err != nil {
		return nil, fmt.Errorf("Failed to get hcn namespace: %s due to error: %v", epInfo.NetNsPath, err)
	}

	if err = Hnsv2.AddNamespaceEndpoint(namespace.Id, hnsResponse.Id); err != nil {
		return nil, fmt.Errorf("Failed to add endpoint: %s to hcn namespace: %s due to error: %v", hnsResponse.Id, namespace.Id, err) //nolint
	}

	defer func() {
		if err != nil {
			if errRemoveNsEp := Hnsv2.RemoveNamespaceEndpoint(namespace.Id, hnsResponse.Id); errRemoveNsEp != nil {
				logger.Error("Failed to remove endpoint from namespace due to error",
					zap.String("id", hnsResponse.Id), zap.String("id", hnsResponse.Id), zap.Error(errRemoveNsEp))
			}
		}
	}()

	// If the Host - container connectivity is requested, create endpoint in HostNCApipaNetwork
	if epInfo.AllowInboundFromHostToNC || epInfo.AllowInboundFromNCToHost {
		if err = nw.createHostNCApipaEndpoint(cli, epInfo); err != nil {
			return nil, fmt.Errorf("Failed to create HostNCApipaEndpoint due to error: %v", err)
		}
	}

	var vlanid int
	if epInfo.Data != nil {
		if vlanData, ok := epInfo.Data[VlanIDKey]; ok {
			vlanid = vlanData.(int)
		}
	}

	var gateway net.IP
	if len(hnsResponse.Routes) > 0 {
		gateway = net.ParseIP(hnsResponse.Routes[0].NextHop)
	}

	// Create the endpoint object.
	ep := &endpoint{
		Id:                       hcnEndpoint.Name,
		HnsId:                    hnsResponse.Id,
		SandboxKey:               epInfo.ContainerID,
		IfName:                   epInfo.IfName,
		IPAddresses:              epInfo.IPAddresses,
		Gateways:                 []net.IP{gateway},
		DNS:                      epInfo.DNS,
		VlanID:                   vlanid,
		EnableSnatOnHost:         epInfo.EnableSnatOnHost,
		NetNs:                    epInfo.NetNsPath,
		AllowInboundFromNCToHost: epInfo.AllowInboundFromNCToHost,
		AllowInboundFromHostToNC: epInfo.AllowInboundFromHostToNC,
		NetworkContainerID:       epInfo.NetworkContainerID,
		ContainerID:              epInfo.ContainerID,
		PODName:                  epInfo.PODName,
		PODNameSpace:             epInfo.PODNameSpace,
	}

	for _, route := range epInfo.Routes {
		ep.Routes = append(ep.Routes, route)
	}

	ep.MacAddress, _ = net.ParseMAC(hnsResponse.MacAddress)

	return ep, nil
}

// deleteEndpointImpl deletes an existing endpoint from the network.
func (nw *network) deleteEndpointImpl(_ netlink.NetlinkInterface, _ platform.ExecClient, _ EndpointClient, ep *endpoint) error {
	if useHnsV2, err := UseHnsV2(ep.NetNs); useHnsV2 {
		if err != nil {
			return err
		}

		return nw.deleteEndpointImplHnsV2(ep)
	}

	return nw.deleteEndpointImplHnsV1(ep)
}

// deleteEndpointImplHnsV1 deletes an existing endpoint from the network using HNS v1.
func (nw *network) deleteEndpointImplHnsV1(ep *endpoint) error {
	logger.Info("HNSEndpointRequest DELETE id", zap.String("id", ep.HnsId))
	hnsResponse, err := Hnsv1.DeleteEndpoint(ep.HnsId)
	logger.Info("HNSEndpointRequest DELETE response err", zap.Any("hnsResponse", hnsResponse), zap.Error(err))

	// todo: may need to improve error handling if hns or hcsshim change their error bubbling.
	// hcsshim bubbles up a generic error when delete fails with message "The endpoint was not found".
	// the best we can do at the moment is string comparison, which is never great for error checking
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			logger.Info("HNS endpoint id not found", zap.String("id", ep.HnsId))
			return nil
		}
	}

	return err
}

// deleteEndpointImplHnsV2 deletes an existing endpoint from the network using HNS v2.
func (nw *network) deleteEndpointImplHnsV2(ep *endpoint) error {
	var (
		hcnEndpoint *hcn.HostComputeEndpoint
		err         error
	)

	if ep.AllowInboundFromHostToNC || ep.AllowInboundFromNCToHost {
		if err = nw.deleteHostNCApipaEndpoint(ep.NetworkContainerID); err != nil {
			logger.Error("Failed to delete HostNCApipaEndpoint due to error", zap.Error(err))
			return err
		}
	}

	logger.Info("Deleting hcn endpoint with id", zap.String("HnsId", ep.HnsId))

	hcnEndpoint, err = Hnsv2.GetEndpointByID(ep.HnsId)
	if err != nil {
		// If error is anything other than EndpointNotFoundError, return error.
		// else log the error but don't return error because endpoint is already deleted.
		if _, endpointNotFound := err.(hcn.EndpointNotFoundError); !endpointNotFound {
			return fmt.Errorf("Failed to get hcn endpoint with id: %s due to err: %w", ep.HnsId, err)
		}

		logger.Error("Delete called on the Endpoint which doesn't exist. Error:", zap.String("HnsId", ep.HnsId), zap.Error(err))
		return nil
	}

	// Remove this endpoint from the namespace
	if err = Hnsv2.RemoveNamespaceEndpoint(hcnEndpoint.HostComputeNamespace, hcnEndpoint.Id); err != nil {
		logger.Error("Failed to remove hcn endpoint from namespace due to error", zap.String("HnsId", ep.HnsId),
			zap.String("HostComputeNamespace", hcnEndpoint.HostComputeNamespace), zap.Error(err))
	}

	if err = Hnsv2.DeleteEndpoint(hcnEndpoint); err != nil {
		return fmt.Errorf("Failed to delete hcn endpoint: %s due to error: %v", ep.HnsId, err)
	}

	logger.Info("Successfully deleted hcn endpoint with id", zap.String("HnsId", ep.HnsId))

	return nil
}

// getInfoImpl returns information about the endpoint.
func (ep *endpoint) getInfoImpl(epInfo *EndpointInfo) {
	epInfo.Data["hnsid"] = ep.HnsId
}

// updateEndpointImpl in windows does nothing for now
func (nm *networkManager) updateEndpointImpl(nw *network, existingEpInfo *EndpointInfo, targetEpInfo *EndpointInfo) (*endpoint, error) {
	return nil, nil
}
