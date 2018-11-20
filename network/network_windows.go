// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/Microsoft/hcsshim"
)

const (
	// HNS network types.
	hnsL2bridge      = "l2bridge"
	hnsL2tunnel      = "l2tunnel"
	CnetAddressSpace = "cnetAddressSpace"
)

// Windows implementation of route.
type route interface{}

// NewNetworkImpl creates a new container network.
func (nm *networkManager) newNetworkImpl(nwInfo *NetworkInfo, extIf *externalInterface) (*network, error) {
	var vlanid int
	networkAdapterName := extIf.Name
	// FixMe: Find a better way to check if a nic that is selected is not part of a vSwitch
	if strings.HasPrefix(networkAdapterName, "vEthernet") {
		networkAdapterName = ""
	}
	// Initialize HNS network.
	hnsNetwork := &hcsshim.HNSNetwork{
		Name:               nwInfo.Id,
		NetworkAdapterName: networkAdapterName,
		DNSServerList:      strings.Join(nwInfo.DNS.Servers, ","),
		Policies:           policy.SerializePolicies(policy.NetworkPolicy, nwInfo.Policies),
	}

	// Set the VLAN and OutboundNAT policies
	opt, _ := nwInfo.Options[genericData].(map[string]interface{})
	if opt != nil && opt[VlanIDKey] != nil {
		vlanPolicy := hcsshim.VlanPolicy{
			Type: "VLAN",
		}
		vlanID, _ := strconv.ParseUint(opt[VlanIDKey].(string), 10, 32)
		vlanPolicy.VLAN = uint(vlanID)

		serializedVlanPolicy, _ := json.Marshal(vlanPolicy)
		hnsNetwork.Policies = append(hnsNetwork.Policies, serializedVlanPolicy)

		vlanid = (int)(vlanPolicy.VLAN)
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
		Id:               nwInfo.Id,
		HnsId:            hnsResponse.Id,
		Mode:             nwInfo.Mode,
		Endpoints:        make(map[string]*endpoint),
		extIf:            extIf,
		VlanId:           vlanid,
		EnableSnatOnHost: nwInfo.EnableSnatOnHost,
	}

	globals, err := hcsshim.GetHNSGlobals()
	if err != nil || globals.Version.Major <= hcsshim.HNSVersion1803.Major {
		// err would be not nil for windows 1709 & below
		// Sleep for 10 seconds as a workaround for windows 1803 & below
		// This is done only when the network is created.
		time.Sleep(time.Duration(10) * time.Second)
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

func getNetworkInfoImpl(nwInfo *NetworkInfo, nw *network) {
}
