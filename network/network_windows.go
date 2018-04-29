// Copyright 2017 Microsoft. All rights reserved.
// MIT License

// +build windows

package network

import (
	"encoding/json"
	"strings"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Microsoft/hcsshim"
)

const (
	// HNS network types.
	hnsL2bridge = "l2bridge"
	hnsL2tunnel = "l2tunnel"
)

// Windows implementation of route.
type route interface{}

// SerializeNwPolicies serializes network policies to json.
func SerializeNwPolicies(policies []Policy) []json.RawMessage {
	var jsonPolicies []json.RawMessage
	for _, policy := range policies {
		jsonPolicies = append(jsonPolicies, policy.Data)
	}

	return jsonPolicies
}

// NewNetworkImpl creates a new container network.
func (nm *networkManager) newNetworkImpl(nwInfo *NetworkInfo, extIf *externalInterface) (*network, error) {
	// Initialize HNS network.
	hnsNetwork := &hcsshim.HNSNetwork{
		Name:               nwInfo.Id,
		NetworkAdapterName: extIf.Name,
		DNSSuffix:          nwInfo.DNS.Suffix,
		DNSServerList:      strings.Join(nwInfo.DNS.Servers, ","),
		Policies:           SerializeNwPolicies(nwInfo.Policies),
	}

	// Set network mode.
	switch nwInfo.Mode {
	case opModeBridge:
		hnsNetwork.Type = hnsL2bridge
	case opModeTunnel:
		hnsNetwork.Type = hnsL2tunnel
	default:
		return nil, errNetworkModeInvalid
	}

	// Populate subnets.
	for _, subnet := range nwInfo.Subnets {
		hnsSubnet := hcsshim.Subnet{
			AddressPrefix:  subnet.Prefix.String(),
			GatewayAddress: subnet.Gateway.String(),
		}

		hnsNetwork.Subnets = append(hnsNetwork.Subnets, hnsSubnet)
	}

	// Marshal the request.
	buffer, err := json.Marshal(hnsNetwork)
	if err != nil {
		return nil, err
	}
	hnsRequest := string(buffer)

	// Create the HNS network.
	log.Printf("[net] HNSNetworkRequest POST request:%+v", hnsRequest)
	hnsResponse, err := hcsshim.HNSNetworkRequest("POST", "", hnsRequest)
	log.Printf("[net] HNSNetworkRequest POST response:%+v err:%v.", hnsResponse, err)
	if err != nil {
		return nil, err
	}

	// Create the network object.
	nw := &network{
		Id:        nwInfo.Id,
		HnsId:     hnsResponse.Id,
		Mode:      nwInfo.Mode,
		Endpoints: make(map[string]*endpoint),
		extIf:     extIf,
	}

	return nw, nil
}

// DeleteNetworkImpl deletes an existing container network.
func (nm *networkManager) deleteNetworkImpl(nw *network) error {
	// Delete the HNS network.
	log.Printf("[net] HNSNetworkRequest DELETE id:%v", nw.HnsId)
	hnsResponse, err := hcsshim.HNSNetworkRequest("DELETE", nw.HnsId, "")
	log.Printf("[net] HNSNetworkRequest DELETE response:%+v err:%v.", hnsResponse, err)

	return err
}
