// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/Microsoft/hcsshim"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/google/uuid"
)

const (
	// HNS network types.
	hnsL2bridge            = "l2bridge"
	hnsL2tunnel            = "l2tunnel"
	CnetAddressSpace       = "cnetAddressSpace"
	vEthernetAdapterPrefix = "vEthernet"
	baseDecimal            = 10
	bitSize                = 32
	defaultRouteCIDR       = "0.0.0.0/0"
	// prefix for interface name created by azure network
	ifNamePrefix = "vEthernet"
	// ipv6 default hop
	ipv6DefaultHop = "::"
	// ipv6 route cmd
	routeCmd = "netsh interface ipv6 %s route \"%s\" \"%s\" \"%s\" store=persistent"
)

// Windows implementation of route.
type route interface{}

// UseHnsV2 indicates whether to use HNSv1 or HNSv2
// HNSv2 should be used if the NetNs is a valid GUID and if the platform
// has HCN which supports HNSv2 API.
func UseHnsV2(netNs string) (bool, error) {
	// Check if the netNs is a valid GUID to decide on HNSv1 or HNSv2
	useHnsV2 := false
	var err error
	if _, err = uuid.Parse(netNs); err == nil {
		useHnsV2 = true
		if err = hcn.V2ApiSupported(); err != nil {
			log.Printf("HNSV2 is not supported on this windows platform")
		}
	}

	return useHnsV2, err
}

// newNetworkImplHnsV1 creates a new container network for HNSv1.
func (nm *networkManager) newNetworkImplHnsV1(nwInfo *NetworkInfo, extIf *externalInterface) (*network, error) {
	var (
		vlanid int
		err    error
	)

	networkAdapterName := extIf.Name

	// Pass adapter name here if it is not empty, this is cause if we don't tell HNS which adapter to use
	// it will just pick one randomly, this is a problem for customers that have multiple adapters
	if nwInfo.AdapterName != "" {
		networkAdapterName = nwInfo.AdapterName
	}

	// FixMe: Find a better way to check if a nic that is selected is not part of a vSwitch
	// per hns team, the hns calls fails if passed a vSwitch interface
	if strings.HasPrefix(networkAdapterName, vEthernetAdapterPrefix) {
		log.Printf("[net] vSwitch detected, setting adapter name to empty")
		networkAdapterName = ""
	}

	log.Printf("[net] Adapter name used with HNS is : %s", networkAdapterName)

	// Initialize HNS network.
	hnsNetwork := &hcsshim.HNSNetwork{
		Name:               nwInfo.Id,
		NetworkAdapterName: networkAdapterName,
		DNSServerList:      strings.Join(nwInfo.DNS.Servers, ","),
		Policies:           policy.SerializePolicies(policy.NetworkPolicy, nwInfo.Policies, nil, false, false),
	}

	// Set the VLAN and OutboundNAT policies
	opt, _ := nwInfo.Options[genericData].(map[string]interface{})
	if opt != nil && opt[VlanIDKey] != nil {
		vlanPolicy := hcsshim.VlanPolicy{
			Type: "VLAN",
		}
		vlanID, _ := strconv.ParseUint(opt[VlanIDKey].(string), baseDecimal, bitSize)
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

	defer func() {
		if err != nil {
			log.Printf("[net] HNSNetworkRequest DELETE id:%v", hnsResponse.Id)
			hnsResponse, err := hcsshim.HNSNetworkRequest("DELETE", hnsResponse.Id, "")
			log.Printf("[net] HNSNetworkRequest DELETE response:%+v err:%v.", hnsResponse, err)
		}
	}()

	// route entry for pod cidr
	if err = nm.appIPV6RouteEntry(nwInfo); err != nil {
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
		NetNs:            nwInfo.NetNs,
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

func (nm *networkManager) appIPV6RouteEntry(nwInfo *NetworkInfo) error {
	var (
		err error
		out string
	)

	if nwInfo.IPV6Mode == IPV6Nat {
		if len(nwInfo.Subnets) < 2 {
			return fmt.Errorf("Ipv6 subnet not found in network state")
		}

		// get interface name of VM adapter
		ifName := nwInfo.MasterIfName
		if !strings.Contains(nwInfo.MasterIfName, ifNamePrefix) {
			ifName = fmt.Sprintf("%s (%s)", ifNamePrefix, nwInfo.MasterIfName)
		}

		cmd := fmt.Sprintf(routeCmd, "delete", nwInfo.Subnets[1].Prefix.String(),
			ifName, ipv6DefaultHop)
		if out, err = nm.plClient.ExecuteCommand(cmd); err != nil {
			log.Printf("[net] Deleting ipv6 route failed: %v:%v", out, err)
		}

		cmd = fmt.Sprintf(routeCmd, "add", nwInfo.Subnets[1].Prefix.String(),
			ifName, ipv6DefaultHop)
		if out, err = nm.plClient.ExecuteCommand(cmd); err != nil {
			log.Printf("[net] Adding ipv6 route failed: %v:%v", out, err)
		}
	}

	return err
}

// configureHcnEndpoint configures hcn endpoint for creation
func (nm *networkManager) configureHcnNetwork(nwInfo *NetworkInfo, extIf *externalInterface) (*hcn.HostComputeNetwork, error) {
	// Initialize HNS network.
	hcnNetwork := &hcn.HostComputeNetwork{
		Name: nwInfo.Id,
		Dns: hcn.Dns{
			Domain:     nwInfo.DNS.Suffix,
			ServerList: nwInfo.DNS.Servers,
		},
		Ipams: []hcn.Ipam{
			{
				Type: hcnIpamTypeStatic,
			},
		},
		SchemaVersion: hcn.SchemaVersion{
			Major: hcnSchemaVersionMajor,
			Minor: hcnSchemaVersionMinor,
		},
	}

	// Set hcn network adaptor name policy
	// FixMe: Find a better way to check if a nic that is selected is not part of a vSwitch
	// per hns team, the hns calls fails if passed a vSwitch interface
	// Pass adapter name here if it is not empty, this is cause if we don't tell HNS which adapter to use
	// it will just pick one randomly, this is a problem for customers that have multiple adapters
	if nwInfo.AdapterName != "" || !strings.HasPrefix(extIf.Name, vEthernetAdapterPrefix) {
		var adapterName string
		if nwInfo.AdapterName != "" {
			adapterName = nwInfo.AdapterName
		} else {
			adapterName = extIf.Name
		}

		log.Printf("[net] Adapter name used with HNS is : %s", adapterName)

		netAdapterNamePolicy, err := policy.GetHcnNetAdapterPolicy(adapterName)
		if err != nil {
			log.Printf("[net] Failed to serialize network adapter policy due to error: %v", err)
			return nil, err
		}

		hcnNetwork.Policies = append(hcnNetwork.Policies, netAdapterNamePolicy)
	}

	// Set hcn subnet policy
	var (
		vlanid       int
		subnetPolicy []byte
	)

	opt, _ := nwInfo.Options[genericData].(map[string]interface{})
	if opt != nil && opt[VlanIDKey] != nil {
		var err error
		vlanID, _ := strconv.ParseUint(opt[VlanIDKey].(string), baseDecimal, bitSize)
		subnetPolicy, err = policy.SerializeHcnSubnetVlanPolicy((uint32)(vlanID))
		if err != nil {
			log.Printf("[net] Failed to serialize subnet vlan policy due to error: %v", err)
			return nil, err
		}

		vlanid = (int)(vlanID)
	}

	// Set network mode.
	switch nwInfo.Mode {
	case opModeBridge:
		hcnNetwork.Type = hcn.L2Bridge
	case opModeTunnel:
		hcnNetwork.Type = hcn.L2Tunnel
	default:
		return nil, errNetworkModeInvalid
	}

	// Populate subnets.
	for _, subnet := range nwInfo.Subnets {
		hnsSubnet := hcn.Subnet{
			IpAddressPrefix: subnet.Prefix.String(),
			// Set the Gateway route
			Routes: []hcn.Route{
				{
					NextHop:           subnet.Gateway.String(),
					DestinationPrefix: defaultRouteCIDR,
				},
			},
		}

		// Set the subnet policy
		if vlanid > 0 {
			hnsSubnet.Policies = append(hnsSubnet.Policies, subnetPolicy)
		}

		hcnNetwork.Ipams[0].Subnets = append(hcnNetwork.Ipams[0].Subnets, hnsSubnet)
	}

	return hcnNetwork, nil
}

// newNetworkImplHnsV2 creates a new container network for HNSv2.
func (nm *networkManager) newNetworkImplHnsV2(nwInfo *NetworkInfo, extIf *externalInterface) (*network, error) {
	hcnNetwork, err := nm.configureHcnNetwork(nwInfo, extIf)
	if err != nil {
		log.Printf("[net] Failed to configure hcn network due to error: %v", err)
		return nil, err
	}

	// Create the HNS network.
	log.Printf("[net] Creating hcn network: %+v", hcnNetwork)
	hnsResponse, err := hnsv2.CreateNetwork(hcnNetwork)
	if err != nil {
		return nil, fmt.Errorf("Failed to create hcn network: %s due to error: %v", hcnNetwork.Name, err)
	}

	log.Printf("[net] Successfully created hcn network with response: %+v", hnsResponse)

	var vlanid int
	opt, _ := nwInfo.Options[genericData].(map[string]interface{})
	if opt != nil && opt[VlanIDKey] != nil {
		vlanID, _ := strconv.ParseInt(opt[VlanIDKey].(string), baseDecimal, bitSize)
		vlanid = (int)(vlanID)
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
		NetNs:            nwInfo.NetNs,
	}

	return nw, nil
}

// NewNetworkImpl creates a new container network.
func (nm *networkManager) newNetworkImpl(nwInfo *NetworkInfo, extIf *externalInterface) (*network, error) {
	if useHnsV2, err := UseHnsV2(nwInfo.NetNs); useHnsV2 {
		if err != nil {
			return nil, err
		}

		return nm.newNetworkImplHnsV2(nwInfo, extIf)
	}

	return nm.newNetworkImplHnsV1(nwInfo, extIf)
}

// DeleteNetworkImpl deletes an existing container network.
func (nm *networkManager) deleteNetworkImpl(nw *network) error {
	if useHnsV2, err := UseHnsV2(nw.NetNs); useHnsV2 {
		if err != nil {
			return err
		}

		return nm.deleteNetworkImplHnsV2(nw)
	}

	return nm.deleteNetworkImplHnsV1(nw)
}

// DeleteNetworkImplHnsV1 deletes an existing container network using HnsV1.
func (nm *networkManager) deleteNetworkImplHnsV1(nw *network) error {
	log.Printf("[net] HNSNetworkRequest DELETE id:%v", nw.HnsId)
	hnsResponse, err := hcsshim.HNSNetworkRequest("DELETE", nw.HnsId, "")
	log.Printf("[net] HNSNetworkRequest DELETE response:%+v err:%v.", hnsResponse, err)

	return err
}

// DeleteNetworkImplHnsV2 deletes an existing container network using HnsV2.
func (nm *networkManager) deleteNetworkImplHnsV2(nw *network) error {
	var hcnNetwork *hcn.HostComputeNetwork
	var err error
	log.Printf("[net] Deleting hcn network with id: %s", nw.HnsId)

	if hcnNetwork, err = hnsv2.GetNetworkByID(nw.HnsId); err != nil {
		return fmt.Errorf("Failed to get hcn network with id: %s due to err: %v", nw.HnsId, err)
	}

	if err = hnsv2.DeleteNetwork(hcnNetwork); err != nil {
		return fmt.Errorf("Failed to delete hcn network: %s due to error: %v", nw.HnsId, err)
	}

	log.Printf("[net] Successfully deleted hcn network with id: %s", nw.HnsId)

	return err
}

func getNetworkInfoImpl(nwInfo *NetworkInfo, nw *network) {
}
