// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/cni/util"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network/hnswrapper"
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
	// ipv4 default hop
	ipv4DefaultHop = "0.0.0.0"
	// ipv6 default hop
	ipv6DefaultHop = "::"
	// ipv6 route cmd
	routeCmd = "netsh interface ipv6 %s route \"%s\" \"%s\" \"%s\" store=persistent"
	// add/delete ipv4 and ipv6 route rules to/from windows node
	netRouteCmd = "netsh interface %s %s route \"%s\" \"%s\" \"%s\""
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

// Regarding this Hnsv2 and Hnv1 variable
// this pattern is to avoid passing around os specific objects in platform agnostic code
var Hnsv2 hnswrapper.HnsV2WrapperInterface = hnswrapper.Hnsv2wrapper{}

var Hnsv1 hnswrapper.HnsV1WrapperInterface = hnswrapper.Hnsv1wrapper{}

func EnableHnsV2Timeout(timeoutValue int) {
	if _, ok := Hnsv2.(hnswrapper.Hnsv2wrapperwithtimeout); !ok {
		timeoutDuration := time.Duration(timeoutValue) * time.Second
		Hnsv2 = hnswrapper.Hnsv2wrapperwithtimeout{Hnsv2: hnswrapper.Hnsv2wrapper{}, HnsCallTimeout: timeoutDuration}
	}
}

func EnableHnsV1Timeout(timeoutValue int) {
	if _, ok := Hnsv1.(hnswrapper.Hnsv1wrapperwithtimeout); !ok {
		timeoutDuration := time.Duration(timeoutValue) * time.Second
		Hnsv1 = hnswrapper.Hnsv1wrapperwithtimeout{Hnsv1: hnswrapper.Hnsv1wrapper{}, HnsCallTimeout: timeoutDuration}
	}
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

	hnsResponse, err := Hnsv1.CreateNetwork(hnsNetwork, "")
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			log.Printf("[net] HNSNetworkRequest DELETE id:%v", hnsResponse.Id)
			hnsResponse, err := Hnsv1.DeleteNetwork(hnsResponse.Id)
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

	globals, err := Hnsv1.GetHNSGlobals()
	if err != nil || globals.Version.Major <= hcsshim.HNSVersion1803.Major {
		// err would be not nil for windows 1709 & below
		// Sleep for 10 seconds as a workaround for windows 1803 & below
		// This is done only when the network is created.
		time.Sleep(time.Duration(10) * time.Second)
	}

	return nw, nil
}

// add ipv4 and ipv6 routes in dualstack overlay mode to windows Node
// in dualstack overlay mode, pods are created from different subnets on different nodes, gateway has to be node ip if pods want to communicate with each other
// add routes to make node understand pod IPs come from different subnets and VFP will take decisions based on these routes to forward traffic and avoid Natting
func (nm *networkManager) addNewNetRules(nwInfo *NetworkInfo) error {
	var (
		err error
		out string
	)

	// get interface name of the VM adapter
	ifName := nwInfo.MasterIfName
	if !strings.Contains(nwInfo.MasterIfName, ifNamePrefix) {
		ifName = fmt.Sprintf("%s (%s)", ifNamePrefix, nwInfo.MasterIfName)
	}

	// check if external interface name is empty
	if ifName == "" {
		return fmt.Errorf("[net] external interface name is empty") // nolint
	}

	// check whether nwInfo subnets exist
	if nwInfo.Subnets == nil {
		return fmt.Errorf("[net] nwInfo subnets are not found") // nolint
	}

	// iterate subnet and add ipv4 and ipv6 default route and gateway only if it is not existing
	for _, subnet := range nwInfo.Subnets {
		prefix := subnet.Prefix.String()

		ip, _, errParseCIDR := net.ParseCIDR(prefix)
		if errParseCIDR != nil {
			return fmt.Errorf("[net] failed to parse prefix %s due to %+v", prefix, errParseCIDR) // nolint
		}

		if subnet.Gateway == nil {
			return fmt.Errorf("[net] failed to get subnet gateway") // nolint
		}
		gateway := subnet.Gateway.String()

		log.Printf("[net] Adding ipv4 and ipv6 net rules to windows node")

		// delete existing net rules before adding new rules to windows node in case:
		// if hnsNetwork is not existing and new pod is creating, existing rules will be applied twice that will cause the pod creation failure
		if ip.To4() != nil {
			deleteNetshV4DefaultRoute := fmt.Sprintf(netRouteCmd, "ipv4", "delete", prefix, ifName, ipv4DefaultHop)
			if _, delErr := nm.plClient.ExecuteCommand(deleteNetshV4DefaultRoute); delErr != nil {
				log.Printf("[net] Deleting ipv4 default route failed: %v", err)
			}

			// netsh interface ipv4 add route $subnetV4 $hostInterfaceAlias "0.0.0.0"
			addNetshV4DefaultRoute := fmt.Sprintf(netRouteCmd, "ipv4", "add", prefix, ifName, ipv4DefaultHop)
			if out, err = nm.plClient.ExecuteCommand(addNetshV4DefaultRoute); err != nil {
				log.Printf("[net] Adding ipv4 default route failed: %v:%v", out, err)
			}

			deleteNetshV4GatewayRoute := fmt.Sprintf(netRouteCmd, "ipv4", "delete", prefix, ifName, gateway)
			if _, delErr := nm.plClient.ExecuteCommand(deleteNetshV4GatewayRoute); delErr != nil {
				log.Printf("[net] Deleting ipv4 gateway route failed: %v", delErr)
			}

			// netsh interface ipv4 add route $subnetV4 $hostInterfaceAlias $gatewayV4
			addNetshV4GatewayRoute := fmt.Sprintf(netRouteCmd, "ipv4", "add", prefix, ifName, gateway)
			if out, err = nm.plClient.ExecuteCommand(addNetshV4GatewayRoute); err != nil {
				log.Printf("[net] Adding ipv4 gateway route failed: %v:%v", out, err)
			}
		} else {
			deleteNetshV6DefaultRoute := fmt.Sprintf(netRouteCmd, "ipv6", "delete", prefix, ifName, ipv6DefaultHop)
			if _, delErr := nm.plClient.ExecuteCommand(deleteNetshV6DefaultRoute); delErr != nil {
				log.Printf("[net] Deleting ipv6 default route failed: %v", delErr)
			}

			// netsh interface ipv6 add route $subnetV6 $hostInterfaceAlias "::"
			addNetshV6DefaultRoute := fmt.Sprintf(netRouteCmd, "ipv6", "add", prefix, ifName, ipv6DefaultHop)
			if out, err = nm.plClient.ExecuteCommand(addNetshV6DefaultRoute); err != nil {
				log.Printf("[net] Adding ipv6 default route failed: %v:%v", out, err)
			}

			deleteNetshV6GatewayRoute := fmt.Sprintf(netRouteCmd, "ipv6", "delete", prefix, ifName, gateway)
			if _, delErr := nm.plClient.ExecuteCommand(deleteNetshV6GatewayRoute); delErr != nil {
				log.Printf("[net] Deleting ipv6 gateway route failed: %v", delErr)
			}

			// netsh interface ipv6 add route $subnetV6 $hostInterfaceAlias $gatewayV6
			addNetshV6GatewayRoute := fmt.Sprintf(netRouteCmd, "ipv6", "add", prefix, ifName, gateway)
			if out, err = nm.plClient.ExecuteCommand(addNetshV6GatewayRoute); err != nil {
				log.Printf("[net] Adding ipv6 gateway route failed: %v:%v", out, err)
			}
		}
	}

	return err // nolint
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

	// check if network exists, only create the network does not exist
	hnsResponse, err := Hnsv2.GetNetworkByName(hcnNetwork.Name)

	if err != nil {
		// if network not found, create the HNS network.
		if errors.As(err, &hcn.NetworkNotFoundError{}) {
			// in dualStackOverlay mode, add net routes to windows node
			if nwInfo.IPV6Mode == string(util.DualStackOverlay) {
				if err := nm.addNewNetRules(nwInfo); err != nil { // nolint
					log.Printf("[net] Failed to add net rules due to %+v", err)
					return nil, err
				}
			}
			log.Printf("[net] Creating hcn network: %+v", hcnNetwork)
			hnsResponse, err = Hnsv2.CreateNetwork(hcnNetwork)

			if err != nil {
				return nil, fmt.Errorf("Failed to create hcn network: %s due to error: %v", hcnNetwork.Name, err)
			}

			log.Printf("[net] Successfully created hcn network with response: %+v", hnsResponse)
		} else {
			// we can't validate if the network already exists, don't continue
			return nil, fmt.Errorf("Failed to create hcn network: %s, failed to query for existing network with error: %v", hcnNetwork.Name, err)
		}
	} else {
		log.Printf("[net] Network with name %s already exists", hcnNetwork.Name)
	}

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
	hnsResponse, err := Hnsv1.DeleteNetwork(nw.HnsId)
	log.Printf("[net] HNSNetworkRequest DELETE response:%+v err:%v.", hnsResponse, err)

	return err
}

// DeleteNetworkImplHnsV2 deletes an existing container network using Hnsv2.
func (nm *networkManager) deleteNetworkImplHnsV2(nw *network) error {
	var hcnNetwork *hcn.HostComputeNetwork
	var err error
	log.Printf("[net] Deleting hcn network with id: %s", nw.HnsId)

	if hcnNetwork, err = Hnsv2.GetNetworkByID(nw.HnsId); err != nil {
		return fmt.Errorf("Failed to get hcn network with id: %s due to err: %v", nw.HnsId, err)
	}

	if err = Hnsv2.DeleteNetwork(hcnNetwork); err != nil {
		return fmt.Errorf("Failed to delete hcn network: %s due to error: %v", nw.HnsId, err)
	}

	log.Printf("[net] Successfully deleted hcn network with id: %s", nw.HnsId)

	return err
}

func getNetworkInfoImpl(nwInfo *NetworkInfo, nw *network) {
}
