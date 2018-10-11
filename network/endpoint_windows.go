// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/Microsoft/hcsshim/hcn"
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
func (nw *network) newEndpointImpl(epInfo *EndpointInfo) (*endpoint, error) {
	// Check for missing namespace
	if epInfo.NetNsPath == "" {
		log.Printf("[net] Endpoint missing Namsepace, cannot Create. [%v].", epInfo)
		return nil, fmt.Errorf("Cannot create Endpoint without a Namespace")
	}

	// Get Infrastructure containerID. Handle ADD calls for workload container.
	var err error
	infraEpName, _ := ConstructEndpointID(epInfo.ContainerID, epInfo.NetNsPath, epInfo.IfName)

	hcnPolicies := policy.SerializeHostComputeEndpointPolicies(epInfo.Policies)

	hnsEndpoint := &hcn.HostComputeEndpoint{
		Name:                 infraEpName,
		HostComputeNetwork:   nw.HnsId,
		HostComputeNamespace: epInfo.NetNsPath, // TODOERIK: Is this in the right form? (Guid)
		Dns: hcn.Dns{
			Suffix:     epInfo.DNS.Suffix,
			ServerList: epInfo.DNS.Servers,
		},
		SchemaVersion: hcn.SchemaVersion{
			Major: 2,
			Minor: 0,
		},
		Policies: hcnPolicies,
	}

	// Populate Mac, if present.
	if epInfo.MacAddress != nil {
		hnsEndpoint.MacAddress = epInfo.MacAddress.String()
	}

	// Populate Routes.
	for _, route := range epInfo.Routes {
		nextHop := ""
		if route.Gw != nil {
			nextHop = route.Gw.String()
		}
		dest := route.Dst.String()
		hcnRoute := hcn.Route{
			NextHop:           nextHop,
			DestinationPrefix: dest,
		}
		hnsEndpoint.Routes = append(hnsEndpoint.Routes, hcnRoute)
	}

	// Populate IPConfigurations.
	for _, ipAddress := range epInfo.IPAddresses {
		ipAddr := ""
		if ipAddress.IP != nil {
			ipAddr = ipAddress.IP.String()
		}
		pl, _ := epInfo.IPAddresses[0].Mask.Size()
		ipConfig := hcn.IpConfig{
			IpAddress:    ipAddr,
			PrefixLength: uint8(pl),
		}
		hnsEndpoint.IpConfigurations = append(hnsEndpoint.IpConfigurations, ipConfig)
	}

	// Create the HNS endpoint.
	log.Printf("[net] HostComputeEndpoint CREATE id:%+v", hnsEndpoint)
	hnsResponse, err := hnsEndpoint.Create()
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			delEndpoint(hnsResponse.Id)
		}
	}()

	// Create the endpoint object.
	gatewayAddr := net.ParseIP(hnsResponse.Routes[0].NextHop)
	ep := &endpoint{
		Id:          infraEpName,
		HnsId:       hnsResponse.Id,
		SandboxKey:  epInfo.ContainerID,
		IfName:      epInfo.IfName,
		IPAddresses: epInfo.IPAddresses,
		Gateways:    []net.IP{gatewayAddr},
		DNS:         epInfo.DNS,
		Routes:      epInfo.Routes,
	}

	ep.MacAddress, _ = net.ParseMAC(hnsResponse.MacAddress)

	return ep, nil
}

func delEndpoint(id string) error {
	log.Printf("[net] HostComputeEndpoint DELETE id:%v", id)
	hnsEndpoint, err := hcn.GetEndpointByID(id)
	if err != nil {
		return err
	}
	_, err = hnsEndpoint.Delete()
	return err
}

// deleteEndpointImpl deletes an existing endpoint from a network.
func (nw *network) deleteEndpointImpl(ep *endpoint) error {
	return delEndpoint(ep.HnsId)
}

// getInfoImpl returns information about the endpoint.
func (ep *endpoint) getInfoImpl(epInfo *EndpointInfo) {
	epInfo.Data["hnsid"] = ep.HnsId
}
