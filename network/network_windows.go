// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/Microsoft/hcsshim/hcn"
)

// Windows implementation of route.
type route interface{}

// NewNetworkImpl creates a new container network.
func (nm *networkManager) newNetworkImpl(nwInfo *NetworkInfo, extIf *externalInterface) (*network, error) {
	networkAdapterName := extIf.Name
	// FixMe: Find a better way to check if a nic that is selected is not part of a vSwitch
	if strings.HasPrefix(networkAdapterName, "vEthernet") {
		networkAdapterName = ""
	}

	hcnPolicies := policy.SerializeHostComputeNetworkPolicies(nwInfo.Policies)

	// Create NetworkAdapterName Policy
	if networkAdapterName != "" {
		hcnNetAdapterNamePolicy := policy.CreateNetworkAdapterNamePolicySetting(networkAdapterName)
		hcnPolicies = append(hcnPolicies, hcnNetAdapterNamePolicy)
	}

	// Initialize HNS network.
	hnsNetwork := &hcn.HostComputeNetwork{
		Name: nwInfo.Id,
		Ipams: []hcn.Ipam{
			hcn.Ipam{
				Type: "Static",
			},
		},
		Dns: hcn.Dns{
			Suffix:     nwInfo.DNS.Suffix,
			ServerList: nwInfo.DNS.Servers,
		},
		SchemaVersion: hcn.SchemaVersion{
			Major: 2,
			Minor: 0,
		},
		Policies: hcnPolicies,
	}

	// Set network mode.
	switch nwInfo.Mode {
	case opModeBridge:
		hnsNetwork.Type = hcn.L2Bridge
	case opModeTunnel:
		hnsNetwork.Type = hcn.L2Tunnel
	default:
		return nil, errNetworkModeInvalid
	}

	// Populate subnets.
	for _, subnet := range nwInfo.Subnets {
		// Check for nil on address objects.
		ipAddr := ""
		if subnet.Prefix.IP != nil && subnet.Prefix.Mask != nil {
			ipAddr = subnet.Prefix.String()
		}
		gwAddr := ""
		if subnet.Gateway != nil {
			gwAddr = subnet.Gateway.String()
		}
		hnsSubnet := hcn.Subnet{
			IpAddressPrefix: ipAddr,
			Routes: []hcn.Route{
				hcn.Route{
					NextHop: gwAddr,
				},
			},
		}
		hnsNetwork.Ipams[0].Subnets = append(hnsNetwork.Ipams[0].Subnets, hnsSubnet)
	}

	// Create the HNS network.
	log.Printf("[net] HostComputeNetwork CREATE id:%+v", hnsNetwork)
	hnsResponse, err := hnsNetwork.Create()
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

	globals, err := hcn.GetGlobals()
	if err != nil || globals.Version.Major <= hcn.HNSVersion1803.Major {
		// err would be not nil for windows 1709 & below
		// Sleep for 10 seconds as a workaround for windows 1803 & below
		// This is done only when the network is created.
		time.Sleep(time.Duration(10) * time.Second)
	}

	return nw, nil
}

// DeleteNetworkImpl deletes an existing container network.
func (nm *networkManager) deleteNetworkImpl(nw *network) error {
	log.Printf("[net] HostComputeNetwork DELETE id:%v", nw.HnsId)
	hnsNetwork, err := hcn.GetNetworkByID(nw.HnsId)
	if err != nil {
		return err
	}
	_, err = hnsNetwork.Delete()
	return err
}

func getNetworkInfoImpl(nwInfo *NetworkInfo, nw *network) {
}
