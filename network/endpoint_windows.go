// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Microsoft/hcsshim"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/pkg/errors"
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

	// device without error flag 0
	noError = "0"
	// device disabled flag 22
	deviceDisabled = "22"
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

func (nw *network) getEndpointWithVFDevice(plc platform.ExecClient, epInfo *EndpointInfo) (*endpoint, error) {
	logger.Info("disable and dismount VF device")

	// check device state before disabling and dismounting vf device
	devicePresence, problemCode, err := getPnpDeviceState(epInfo.PnPID, plc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get VF device state")
	}

	// state machine, use devicePresence and problemCode to determine actions
	if devicePresence == "True" && problemCode == noError { //nolint
		logger.Info("Device enabled and mounted")

		if err := disableVFDevice(epInfo.PnPID, plc); err != nil { //nolint
			return nil, errors.Wrap(err, "failed to disable VF device")
		}

		if err := dismountVFDevice(epInfo.PnPID, plc); err != nil { //nolint
			return nil, errors.Wrap(err, "failed to dismount VF device")
		}

		// get new pnp id after VF dismount
		pnpDeviceID, err := getPnPDeviceID(epInfo.PnPID, plc) //nolint
		if err != nil {
			return nil, errors.Wrap(err, "failed to get updated VF device ID")
		}

		// assign updated PciID back to containerd
		epInfo.PnPID = pnpDeviceID
	} else if devicePresence == "True" && problemCode == deviceDisabled {
		logger.Info("Device disabled")
		// device is disabled but not dismounted
		if err := dismountVFDevice(epInfo.PnPID, plc); err != nil { //nolint
			return nil, errors.Wrap(err, "failed to dismount VF device")
		}

		// get new pnp id after VF dismount
		pnpDeviceID, err := getPnPDeviceID(epInfo.PnPID, plc) //nolint
		if err != nil {
			return nil, errors.Wrap(err, "failed to get updated VF device ID")
		}

		// assign updated PciID back to containerd
		epInfo.PnPID = pnpDeviceID
	} else if devicePresence == "False" {
		logger.Info("Device dismounted")
		// device is disabled and dismounted, just get the new PciID and assign back to containerd
		pnpDeviceID, err := getPnPDeviceID(epInfo.PnPID, plc) //nolint
		if err != nil {
			return nil, errors.Wrap(err, "failed to get updated VF device ID")
		}
		// assign updated PciID back to containerd
		epInfo.PnPID = pnpDeviceID
	} else {
		// return unexpected error and log devicePresence, problemCode
		return nil, errors.Wrapf(err, "unexpected error with devicePresence %s and problemCode %s", devicePresence, problemCode)
	}

	// Create the endpoint object.
	ep := &endpoint{
		Id:          epInfo.MasterIfName,
		IfName:      epInfo.MasterIfName,
		ContainerID: epInfo.ContainerID,
		MacAddress:  epInfo.MacAddress,
		NICType:     cns.BackendNIC,
	}

	// do not create endpoint for IB NIC interface
	return ep, nil
}

// newEndpointImpl creates a new endpoint in the network.
func (nw *network) newEndpointImpl(
	cli apipaClient,
	_ netlink.NetlinkInterface,
	plc platform.ExecClient,
	_ netio.NetIOInterface,
	_ EndpointClient,
	_ NamespaceClientInterface,
	_ ipTablesClient,
	_ dhcpClient,
	epInfo *EndpointInfo,
) (*endpoint, error) {
	if epInfo.NICType == cns.BackendNIC {
		return nw.getEndpointWithVFDevice(plc, epInfo)
	}

	if epInfo.NICType == cns.ApipaNIC {
		return nw.createHostNCApipaEndpoint(cli, epInfo)
	}

	if useHnsV2, err := UseHnsV2(epInfo.NetNsPath); useHnsV2 {
		if err != nil {
			return nil, err
		}

		return nw.newEndpointImplHnsV2(cli, epInfo)

	}

	return nw.newEndpointImplHnsV1(epInfo, plc)
}

// newEndpointImplHnsV1 creates a new endpoint in the network using HnsV1
func (nw *network) newEndpointImplHnsV1(epInfo *EndpointInfo, plc platform.ExecClient) (*endpoint, error) {
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
		DNSSuffix:      epInfo.EndpointDNS.Suffix,
		DNSServerList:  strings.Join(epInfo.EndpointDNS.Servers, ","),
		Policies:       policy.SerializePolicies(policy.EndpointPolicy, epInfo.EndpointPolicies, epInfo.Data, epInfo.EnableSnatForDns, epInfo.EnableMultiTenancy),
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
	if err := nw.addIPv6NeighborEntryForGateway(epInfo, plc); err != nil {
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
		DNS:              epInfo.EndpointDNS,
		VlanID:           vlanid,
		EnableSnatOnHost: epInfo.EnableSnatOnHost,
		NetNs:            epInfo.NetNsPath,
		ContainerID:      epInfo.ContainerID,
		NICType:          epInfo.NICType,
	}

	for _, route := range epInfo.Routes {
		ep.Routes = append(ep.Routes, route)
	}

	ep.MacAddress, _ = net.ParseMAC(hnsResponse.MacAddress)

	epInfo.HNSEndpointID = hnsResponse.Id // we use the ep info hns id later in stateless to clean up in ADD if there is an error

	return ep, nil
}

func (nw *network) addIPv6NeighborEntryForGateway(epInfo *EndpointInfo, plc platform.ExecClient) error {
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
			nw.Subnets[1].Gateway.String(), containerIfNamePrefix, epInfo.EndpointID, defaultGwMac)

		if out, err = plc.ExecutePowershellCommand(cmd); err != nil {
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
			Search:     strings.Split(epInfo.EndpointDNS.Suffix, ","),
			ServerList: epInfo.EndpointDNS.Servers,
			Options:    epInfo.EndpointDNS.Options,
		},
		SchemaVersion: hcn.SchemaVersion{
			Major: hcnSchemaVersionMajor,
			Minor: hcnSchemaVersionMinor,
		},
	}

	// macAddress type for InfraNIC is like "60:45:bd:12:45:65"
	// if NICType is delegatedVMNIC or AccelnetNIC, convert the macaddress format
	macAddress := epInfo.MacAddress.String()
	if epInfo.NICType == cns.NodeNetworkInterfaceFrontendNIC {
		// convert the format of macAddress that HNS can accept, i.e, "60-45-bd-12-45-65" if NIC type is delegated NIC
		macAddress = strings.Join(strings.Split(macAddress, ":"), "-")
	}
	hcnEndpoint.MacAddress = macAddress

	if epPolicies, err := policy.GetHcnEndpointPolicies(policy.EndpointPolicy, epInfo.EndpointPolicies, epInfo.Data, epInfo.EnableSnatForDns, epInfo.EnableMultiTenancy, epInfo.NATInfo); err == nil {
		hcnEndpoint.Policies = append(hcnEndpoint.Policies, epPolicies...)
	} else {
		logger.Error("Failed to get endpoint policies due to", zap.Error(err))
		return nil, err
	}

	// add hcnEndpoint policy for accelnet for frontendNIC
	if epInfo.NICType == cns.NodeNetworkInterfaceFrontendNIC {
		endpointPolicy, err := policy.AddAccelnetPolicySetting()
		if err != nil {
			logger.Error("Failed to set iov endpoint policy", zap.Error(err))
			return nil, errors.Wrapf(err, "Failed to set iov endpoint policy for endpointId :%s", epInfo.EndpointID)
		}
		hcnEndpoint.Policies = append(hcnEndpoint.Policies, endpointPolicy)
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

func getApipaEndpointName(networkContainerID string) string {
	endpointName := fmt.Sprintf("%s-%s", hostNCApipaEndpointNamePrefix, networkContainerID)
	return endpointName
}

func (nw *network) deleteHostNCApipaEndpoint(networkContainerID string) error {
	// TODO: this code is duplicated in cns/hnsclient, but that code has logging messages that require a CNSLogger,
	// which makes is hard to use in this package. We should refactor this into a common package with no logging deps
	// so it can be called in both places
	endpointName := getApipaEndpointName(networkContainerID)

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

	logger.Info("Deleting Apipa Endpoint", zap.String("endpointName", endpointName))
	if err := Hnsv2.DeleteEndpoint(endpoint); err != nil {
		return fmt.Errorf("failed to delete HostNCApipa endpoint: %+v: %w", endpoint, err)
	}

	logger.Info("Successfully deleted HostNCApipa endpoint", zap.Any("endpoint", endpoint))

	return nil
}

// createHostNCApipaEndpoint creates a new endpoint in the HostNCApipaNetwork
// for host container connectivity
func (nw *network) createHostNCApipaEndpoint(cli apipaClient, epInfo *EndpointInfo) (*endpoint, error) {
	var (
		err                   error
		hostNCApipaEndpointID string
		namespace             *hcn.HostComputeNamespace
	)

	if namespace, err = hcn.GetNamespaceByID(epInfo.NetNsPath); err != nil {
		//nolint
		return nil, fmt.Errorf("Failed to retrieve namespace with GetNamespaceByID for NetNsPath: %s"+
			" due to error: %v", epInfo.NetNsPath, err)
	}

	logger.Info("Creating HostNCApipaEndpoint for host container connectivity for NC",
		zap.String("NetworkContainerID", epInfo.NetworkContainerID))

	if hostNCApipaEndpointID, err = cli.CreateHostNCApipaEndpoint(context.TODO(), epInfo.NetworkContainerID); err != nil {
		//nolint
		return nil, err
	}

	defer func() {
		if err != nil {
			nw.deleteHostNCApipaEndpoint(epInfo.NetworkContainerID)
		}
	}()

	if err = hcn.AddNamespaceEndpoint(namespace.Id, hostNCApipaEndpointID); err != nil {
		//nolint
		return nil, fmt.Errorf("Failed to add HostNCApipaEndpoint: %s to namespace: %s due to error: %v", hostNCApipaEndpointID, namespace.Id, err)
	}

	ep := &endpoint{
		Id:                 getApipaEndpointName(epInfo.NetworkContainerID),
		HnsId:              hostNCApipaEndpointID,
		IfName:             getApipaEndpointName(epInfo.NetworkContainerID),
		ContainerID:        epInfo.ContainerID,
		NICType:            cns.ApipaNIC,
		NetworkContainerID: epInfo.NetworkContainerID,
	}

	return ep, nil
}

// newEndpointImplHnsV2 creates a new endpoint in the network using Hnsv2
func (nw *network) newEndpointImplHnsV2(cli apipaClient, epInfo *EndpointInfo) (*endpoint, error) {
	hcnEndpoint, err := nw.configureHcnEndpoint(epInfo)
	if err != nil {
		logger.Error("Failed to configure hcn endpoint due to", zap.Error(err))
		return nil, err
	}

	// Create the HCN endpoint.
	logger.Info("Creating hcn endpoint", zap.Any("hcnEndpoint", hcnEndpoint), zap.String("computenetwork", hcnEndpoint.HostComputeNetwork))
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
		if _, err = nw.createHostNCApipaEndpoint(cli, epInfo); err != nil {
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

	nicName := epInfo.IfName
	// infra nic nicname will look like eth0, but delegated/secondary nics will look like "vEthernet x" where x is 1-7
	if epInfo.NICType != cns.InfraNIC {
		nicName = epInfo.MasterIfName
	}

	// Create the endpoint object.
	ep := &endpoint{
		Id:                       hcnEndpoint.Name,
		HnsId:                    hnsResponse.Id,
		SandboxKey:               epInfo.ContainerID,
		IfName:                   nicName,
		IPAddresses:              epInfo.IPAddresses,
		Gateways:                 []net.IP{gateway},
		DNS:                      epInfo.EndpointDNS,
		VlanID:                   vlanid,
		EnableSnatOnHost:         epInfo.EnableSnatOnHost,
		NetNs:                    epInfo.NetNsPath,
		AllowInboundFromNCToHost: epInfo.AllowInboundFromNCToHost,
		AllowInboundFromHostToNC: epInfo.AllowInboundFromHostToNC,
		NetworkContainerID:       epInfo.NetworkContainerID,
		ContainerID:              epInfo.ContainerID,
		PODName:                  epInfo.PODName,
		PODNameSpace:             epInfo.PODNameSpace,
		HNSNetworkID:             epInfo.HNSNetworkID,
		NICType:                  epInfo.NICType,
	}

	for _, route := range epInfo.Routes {
		ep.Routes = append(ep.Routes, route)
	}

	ep.MacAddress, _ = net.ParseMAC(hnsResponse.MacAddress)

	epInfo.HNSEndpointID = hnsResponse.Id // we use the ep info hns id later in stateless to clean up in ADD if there is an error

	return ep, nil
}

// deleteEndpointImpl deletes an existing endpoint from the network.
func (nw *network) deleteEndpointImpl(_ netlink.NetlinkInterface, _ platform.ExecClient, _ EndpointClient, _ netio.NetIOInterface, _ NamespaceClientInterface,
	_ ipTablesClient, _ dhcpClient, ep *endpoint, _ string,
) error {
	// endpoint deletion is not required for IB
	if ep.NICType == cns.BackendNIC {
		return nil
	}

	if ep.NICType == cns.ApipaNIC {
		return nw.deleteHostNCApipaEndpoint(ep.NetworkContainerID)
	}

	if ep.HnsId == "" {
		logger.Info("No HNS id found. Skip endpoint deletion", zap.Any("nicType", ep.NICType), zap.String("containerId", ep.ContainerID))
		return nil
	}

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

// GetEndpointInfoByIPImpl returns an endpointInfo with the corrsponding HNS Endpoint ID that matches an specific IP Address.
func (epInfo *EndpointInfo) GetEndpointInfoByIPImpl(ipAddresses []net.IPNet, networkID string) (*EndpointInfo, error) {
	logger.Info("Fetching missing HNS endpoint id for endpoints in network with id", zap.String("id", networkID))
	hnsResponse, err := Hnsv2.GetNetworkByName(networkID)
	if err != nil || hnsResponse == nil {
		return epInfo, errors.Wrapf(err, "HNS Network or endpoints not found")
	}
	hcnEndpoints, err := Hnsv2.ListEndpointsOfNetwork(hnsResponse.Id)
	if err != nil {
		return epInfo, errors.Wrapf(err, "failed to fetch HNS endpoints for the given network")
	}
	for i := range hcnEndpoints {
		for _, ipConfiguration := range hcnEndpoints[i].IpConfigurations {
			for _, ipAddress := range ipAddresses {
				if ipConfiguration.IpAddress == ipAddress.IP.String() {
					logger.Info("Successfully found hcn endpoint id for endpoint with ip", zap.String("id", hcnEndpoints[i].Id), zap.String("ip", ipAddress.IP.String()))
					epInfo.HNSEndpointID = hcnEndpoints[i].Id
					return epInfo, nil
				}
			}
		}
	}
	return epInfo, errors.Wrapf(err, "No HNSEndpointID matches the IPAddress: %s", ipAddresses[0].IP.String())
}

// Get PnP Device ID
func getPnPDeviceID(instanceID string, plc platform.ExecClient) (string, error) {
	// get device locationPath
	getLocationPath := fmt.Sprintf("(Get-PnpDeviceProperty -KeyName DEVPKEY_Device_LocationPaths –InstanceId \"%s\").Data[0]", instanceID) //nolint
	locationPath, err := plc.ExecutePowershellCommand(getLocationPath)
	if err != nil {
		return "", fmt.Errorf("Failed to get VF locationPath due to error:%w", err)
	}

	// get device PnP id by locationPath
	getPnPDeviceID := fmt.Sprintf("(Get-VMHostAssignableDevice | Where-Object LocationPath -eq \"%s\").InstanceID", locationPath) //nolint
	pnpDeviceID, err := plc.ExecutePowershellCommand(getPnPDeviceID)
	if err != nil {
		logger.Error("Failed to get PnP device ID", zap.Error(err))
		return "", fmt.Errorf("Failed to get PnP device ID due to error:%w", err)
	}

	logger.Info("Get PnP device ID succeeded", zap.String("new device pciID", pnpDeviceID))
	return pnpDeviceID, nil
}

// Disable VF device
func disableVFDevice(instanceID string, plc platform.ExecClient) error {
	// disable device
	disableVFDevice := fmt.Sprintf("Disable-PnpDevice -InstanceId \"%s\" -confirm:$false", instanceID) //nolint
	_, err := plc.ExecutePowershellCommand(disableVFDevice)
	if err != nil {
		logger.Error("Failed to disable VF device", zap.Error(err))
		return fmt.Errorf("Failed to disable VF device due to error:%w", err)
	}

	logger.Info("pnp device disable succeeded", zap.String("VF device", instanceID))
	return nil
}

// Dismount VF device
func dismountVFDevice(instanceID string, plc platform.ExecClient) error {
	locationPath, err := getLocationPath(instanceID, plc)
	if err != nil {
		return err
	}

	// dismount device
	dismountVFDevice := fmt.Sprintf("Dismount-VMHostAssignableDevice -Force -LocationPath \"%s\" -confirm:$false", locationPath) //nolint
	_, err = plc.ExecutePowershellCommand(dismountVFDevice)
	if err != nil {
		logger.Error("Failed to dismount VF device", zap.Error(err))
		return fmt.Errorf("Failed to disamount VF device due to error:%w", err)
	}

	logger.Info("PnP device dismount succeeded", zap.String("VF device", instanceID))
	return nil
}

// Get LocationPath
func getLocationPath(instanceID string, plc platform.ExecClient) (string, error) {
	// get device locationPath
	getLocationPath := fmt.Sprintf("(Get-PnpDeviceProperty -KeyName DEVPKEY_Device_LocationPaths –InstanceId \"%s\").Data[0]", instanceID) //nolint
	locationPath, err := plc.ExecutePowershellCommand(getLocationPath)
	if err != nil {
		logger.Error("Failed to get VF locationPath", zap.Error(err))
		return "", fmt.Errorf("Failed to get VF locationPath due to error:%w", err)
	}

	logger.Info("Get pnp device locationPath succeeded", zap.String("locationPath", locationPath))
	return locationPath, nil
}

// Get PnP device state; PnP device objects represent the mounted/dismounted IB VFs
// return devpkeyDeviceIsPresent and devpkeyDeviceProblemCode
func getPnpDeviceState(instanceID string, plc platform.ExecClient) (string, string, error) { //nolint
	// get if device is present
	getDeviceIsPresent := fmt.Sprintf("(Get-PnpDeviceProperty -InstanceId \"%s\" | Where-Object KeyName -eq DEVPKEY_Device_IsPresent).Data[0]", instanceID) //nolint
	devpkeyDeviceIsPresent, err := plc.ExecutePowershellCommand(getDeviceIsPresent)
	if err != nil {
		logger.Error("Failed to get PnP device devpKeyIsPresent", zap.Error(err))
		return "", "", fmt.Errorf("Failed to get PnP device devpKeyIsPresent due to error:%w", err)
	}
	logger.Info("Get pnp device property succeeded", zap.String("deviceKeyExists", devpkeyDeviceIsPresent))

	// DEVPKEY_Device_ProblemCode is not there once device is disabled and dismounted, so need to check if DEVPKEY_Device_ProblemCode exists first
	getDeviceProblemCodeExist := fmt.Sprintf("(Get-PnpDeviceProperty -InstanceId \"%s\" | Where-Object KeyName -eq DEVPKEY_Device_ProblemCode)", instanceID) //nolint
	devpkeyDeviceProblemCodeExist, err := plc.ExecutePowershellCommand(getDeviceProblemCodeExist)
	if err != nil {
		logger.Error("problemCode is unknown", zap.Error(err))
		return "", "", fmt.Errorf("problemCode is unknown due to error:%w", err)
	}

	// only return isPresent flag and empty string as problemCode
	if devpkeyDeviceProblemCodeExist == "" {
		return devpkeyDeviceIsPresent, "", nil
	}

	// get device problemCode
	getDeviceProblemCode := fmt.Sprintf("(Get-PnpDeviceProperty -InstanceId \"%s\" | Where-Object KeyName -eq DEVPKEY_Device_ProblemCode).Data[0]", instanceID) //nolint
	devpkeyDeviceProblemCode, err := plc.ExecutePowershellCommand(getDeviceProblemCode)
	if err != nil {
		logger.Error("Failed to get PnP device problemCode", zap.Error(err))
		return "", "", fmt.Errorf("Failed to get if PnP device problemCode due to error:%w", err)
	}

	logger.Info("Retrieved device problem code", zap.String("code", devpkeyDeviceProblemCode))
	return devpkeyDeviceIsPresent, devpkeyDeviceProblemCode, nil
}
