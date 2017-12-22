// Copyright 2017 Microsoft. All rights reserved.
// MIT License

// +build windows

package network

import (
	"encoding/json"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Microsoft/hcsshim"
)

// Reconstruct container id from netNsPath.
func ConstructEndpointID(netNsPath string, ifName string) (string, bool) {
	endpointID := ""
	isWorkLoad := false
	if netNsPath != "" {
		splits := strings.Split(netNsPath, ":")
		if len(splits) == 2 {
			endpointID = splits[1]
			isWorkLoad = true
		}
		if len(endpointID) > 8 {
			endpointID = endpointID[:8] + "-" + ifName
			log.Printf("[net] constructed endpointID: %v", endpointID)
		}
	}
	return endpointID, isWorkLoad
}

// newEndpointImpl creates a new endpoint in the network.
func (nw *network) newEndpointImpl(epInfo *EndpointInfo) (*endpoint, error) {
	// Check if endpoint already exists.
	log.Printf("[net] Entering newEndpointImpl.")
	log.Printf("[net] epInfo.Id: %v, epInfo.ContainerID: %v, epInfo.NetNsPath: %v", epInfo.Id, epInfo.ContainerID, epInfo.NetNsPath)

	// Ignore consecutive ADD calls for the same container.	
	if nw.Endpoints[epInfo.Id] != nil {
		log.Printf("[net] Found existing endpoint %v", epInfo.Id) 
		return nw.Endpoints[epInfo.Id], nil
	}
	
	// Get Infrastructure containerID. Ignore ADD calls for workload container.

	infraEpID, isWorkLoad := ConstructEndpointID(epInfo.NetNsPath, epInfo.IfName)
	log.Printf("[net] infraEpID: %v", infraEpID)

	if isWorkLoad && nw.Endpoints[infraEpID] != nil {
		log.Printf("[net] Found existing infrastructure endpoint %v", infraEpID)
		if hnsEndpoint != nil
		//TODO: attach
		return nw.Endpoints[infraEpID], nil		
	}	
	hnsEndpoint, err := hcsshim.GetHNSEndpointByName(infraEpID)
	if hnsEndpoint != nil {
		log.Printf("[net] Found existing endpoint %v", infraEpID)
		//TODO: attach
	}

	// Initialize HNS endpoint.
	hnsEndpoint = &hcsshim.HNSEndpoint{
		Name:           epInfo.Id,
		VirtualNetwork: nw.HnsId,
		DNSSuffix:      epInfo.DNS.Suffix,
		DNSServerList:  strings.Join(epInfo.DNS.Servers, ","),
	}

	//enable outbound NAT
	var enableOutBoundNat = json.RawMessage(`{"Type":  "OutBoundNAT"}`)
	hnsEndpoint.Policies = append(hnsEndpoint.Policies, enableOutBoundNat)

	// HNS currently supports only one IP address per endpoint.
	if epInfo.IPAddresses != nil {
		hnsEndpoint.IPAddress = epInfo.IPAddresses[0].IP
		pl, _ := epInfo.IPAddresses[0].Mask.Size()
		hnsEndpoint.PrefixLength = uint8(pl)
	}

	// Marshal the request.
	buffer, err := json.Marshal(hnsEndpoint)
	if err != nil {
		return nil, err
	}
	hnsRequest := string(buffer)

	// Create the HNS endpoint.
	log.Printf("[net] HNSEndpointRequest POST request:%+v", hnsRequest)
	hnsResponse, err := hcsshim.HNSEndpointRequest("POST", "", hnsRequest)
	log.Printf("[net] HNSEndpointRequest POST response:%+v err:%v.", hnsResponse, err)
	if err != nil {
		return nil, err
	}

	// Attach the endpoint.
	log.Printf("[net] Attaching endpoint %v to container %v.", hnsResponse.Id, epInfo.ContainerID)
	err = hcsshim.HotAttachEndpoint(epInfo.ContainerID, hnsResponse.Id)
	if err != nil {
		log.Printf("[net] Failed to attach endpoint: %v.", err)
	}

	// Create the endpoint object.
	ep := &endpoint{
		Id:          epInfo.Id,
		HnsId:       hnsResponse.Id,
		SandboxKey:  epInfo.ContainerID,
		IfName:      epInfo.IfName,
		IPAddresses: epInfo.IPAddresses,
		Gateways:    []net.IP{net.ParseIP(hnsResponse.GatewayAddress)},
	}

	ep.MacAddress, _ = net.ParseMAC(hnsResponse.MacAddress)

	return ep, nil
}

// deleteEndpointImpl deletes an existing endpoint from the network.
func (nw *network) deleteEndpointImpl(ep *endpoint) error {
	// Delete the HNS endpoint.
	log.Printf("[net] HNSEndpointRequest DELETE id:%v", ep.HnsId)
	hnsResponse, err := hcsshim.HNSEndpointRequest("DELETE", ep.HnsId, "")
	log.Printf("[net] HNSEndpointRequest DELETE response:%+v err:%v.", hnsResponse, err)

	return err
}

// getInfoImpl returns information about the endpoint.
func (ep *endpoint) getInfoImpl(epInfo *EndpointInfo) {
	epInfo.Data["hnsid"] = ep.HnsId
}
