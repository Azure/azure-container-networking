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

// ConstructEpName constructs endpoint name from netNsPath.
func ConstructEpName(containerID string, netNsPath string, ifName string) (string, string) {
	infraEpName, workloadEpName := "", ""

	if len(containerID) > 8 {
		containerID = containerID[:8]
	}

	if netNsPath != "" {
		splits := strings.Split(netNsPath, ":")
		// For workload containers, we extract its linking infrastructure container ID.
		if len(splits) == 2 {
			if len(splits[1]) > 8 {
				splits[1] = splits[1][:8]
			}
			infraEpName = splits[1] + "-" + ifName
			workloadEpName = containerID + "-" + ifName
		} else {
			// For infrastructure containers, we just use its container ID.
			infraEpName = containerID + "-" + ifName
		}
	}
	return infraEpName, workloadEpName
}

// newEndpointImpl creates a new endpoint in the network.
func (nw *network) newEndpointImpl(epInfo *EndpointInfo) (*endpoint, error) {
	// Get Infrastructure containerID. Handle ADD calls for workload container.
	infraEpName, workloadEpName := ConstructEpName(epInfo.ContainerID, epInfo.NetNsPath, epInfo.IfName)

	/* Handle consecutive ADD calls for infrastructure containers.
	 * This is a temporary work around for issue #57253 of Kubernetes.
	 * We can delete this if statement once they fix it.
	 * Issue link: https://github.com/kubernetes/kubernetes/issues/57253
	 */
	if workloadEpName == "" {
		if nw.Endpoints[infraEpName] != nil {
			log.Printf("[net] Found existing endpoint %v, return immediately.", infraEpName)
			return nw.Endpoints[infraEpName], nil
		}
	}

	log.Printf("[net] infraEpName: %v", infraEpName)

	hnsEndpoint, _ := hcsshim.GetHNSEndpointByName(infraEpName)
	if hnsEndpoint != nil {
		log.Printf("[net] Found existing endpoint through hcsshim%v", infraEpName)
		log.Printf("[net] Attaching ep %v to container %v", hnsEndpoint.Id, epInfo.ContainerID)
		if err := hcsshim.HotAttachEndpoint(epInfo.ContainerID, hnsEndpoint.Id); err != nil {
			return nil, err
		}
		return nw.Endpoints[infraEpName], nil
	}

	hnsEndpoint = &hcsshim.HNSEndpoint{
		Name:           infraEpName,
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
		Id:          infraEpName,
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
