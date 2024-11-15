// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Microsoft/hcsshim"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	// HNS network types.
	hnsL2bridge            = "l2bridge"
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
	// Default IPv6 Route
	defaultIPv6Route = "::/0"
	// Default IPv6 nextHop
	defaultIPv6NextHop = "fe80::1234:5678:9abc"
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
		if err = Hnsv2.HNSV2Supported(); err != nil {
			logger.Info("HNSV2 is not supported on this windows platform")
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
func (nm *networkManager) newNetworkImplHnsV1(nwInfo *EndpointInfo, extIf *externalInterface) (*network, error) {
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
		logger.Info("vSwitch detected, setting adapter name to empty")
		networkAdapterName = ""
	}

	logger.Info("Adapter name used with HNS is", zap.String("networkAdapterName", networkAdapterName))

	// Initialize HNS network.
	hnsNetwork := &hcsshim.HNSNetwork{
		Name:               nwInfo.NetworkID,
		Type:               hnsL2bridge,
		NetworkAdapterName: networkAdapterName,
		Policies:           policy.SerializePolicies(policy.NetworkPolicy, nwInfo.NetworkPolicies, nil, false, false),
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
			logger.Info("HNSNetworkRequest DELETE", zap.String("id", hnsResponse.Id))
			hnsResponse, err := Hnsv1.DeleteNetwork(hnsResponse.Id)
			logger.Info("HNSNetworkRequest DELETE response", zap.Any("hnsResponse", hnsResponse), zap.Error(err))
		}
	}()

	// route entry for pod cidr
	if err = nm.appIPV6RouteEntry(nwInfo); err != nil {
		return nil, err
	}

	// Create the network object.
	nw := &network{
		Id:               nwInfo.NetworkID,
		HnsId:            hnsResponse.Id,
		Mode:             nwInfo.Mode,
		Endpoints:        make(map[string]*endpoint),
		extIf:            extIf,
		VlanId:           vlanid,
		EnableSnatOnHost: nwInfo.EnableSnatOnHost,
		NetNs:            nwInfo.NetNs,
	}

	nwInfo.HNSNetworkID = hnsResponse.Id // we use this later in stateless to clean up in ADD if there is an error

	globals, err := Hnsv1.GetHNSGlobals()
	if err != nil || globals.Version.Major <= hcsshim.HNSVersion1803.Major {
		// err would be not nil for windows 1709 & below
		// Sleep for 10 seconds as a workaround for windows 1803 & below
		// This is done only when the network is created.
		time.Sleep(time.Duration(10) * time.Second)
	}

	return nw, nil
}

func (nm *networkManager) appIPV6RouteEntry(nwInfo *EndpointInfo) error {
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
		if out, err = nm.plClient.ExecuteRawCommand(cmd); err != nil {
			logger.Error("Deleting ipv6 route failed", zap.Any("out", out), zap.Error(err))
		}

		cmd = fmt.Sprintf(routeCmd, "add", nwInfo.Subnets[1].Prefix.String(),
			ifName, ipv6DefaultHop)
		if out, err = nm.plClient.ExecuteRawCommand(cmd); err != nil {
			logger.Error("Adding ipv6 route failed", zap.Any("out", out), zap.Error(err))
		}
	}

	return err
}

// configureHcnEndpoint configures hcn endpoint for creation
func (nm *networkManager) configureHcnNetwork(nwInfo *EndpointInfo, extIf *externalInterface) (*hcn.HostComputeNetwork, error) {
	// Initialize HNS network.
	hcnNetwork := &hcn.HostComputeNetwork{
		Name: nwInfo.NetworkID,
		Type: hcn.L2Bridge,
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

		logger.Info("Adapter name used with HNS is", zap.String("adapterName", adapterName))

		netAdapterNamePolicy, err := policy.GetHcnNetAdapterPolicy(adapterName)
		if err != nil {
			logger.Error("Failed to serialize network adapter policy due to", zap.Error(err))
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
			logger.Error("Failed to serialize subnet vlan policy due to", zap.Error(err))
			return nil, err
		}

		vlanid = (int)(vlanID)
	}

	// AccelnetNIC flag: hcn.EnableIov(9216) - treat Delegated/FrontendNIC also the same as Accelnet
	// For L1VH with accelnet, hcn.DisableHostPort and hcn.EnableIov must be configured
	if nwInfo.NICType == cns.NodeNetworkInterfaceFrontendNIC {
		hcnNetwork.Type = hcn.Transparent
		// set transparent network as non-persistent so that networks will be gone after the node gets rebooted
		// hcnNetwork.flags = hcn.DisableHostPort | hcn.EnableIov | hcn.EnableNonPersistent (1024 + 8192 + 8 = 9224)
		hcnNetwork.Flags = hcn.DisableHostPort | hcn.EnableIov | hcn.EnableNonPersistent
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

func (nm *networkManager) addIPv6DefaultRoute() error {
	// add ipv6 default route if it does not exist in dualstack overlay windows node from persistent store
	// persistent store setting is only read during the adapter restarts or reboots to re-populate the active store

	// get interface index to add ipv6 default route and only consider there is one vEthernet interface for now
	getIpv6IfIndexCmd := `((Get-NetIPInterface | where InterfaceAlias -Like "vEthernet*").IfIndex)[0]`
	ifIndex, err := nm.plClient.ExecutePowershellCommand(getIpv6IfIndexCmd)
	if err != nil {
		return errors.Wrap(err, "error while executing powershell command to get ipv6 Hyper-V interface")
	}

	getIPv6RoutePersistentCmd := fmt.Sprintf("Get-NetRoute -DestinationPrefix %s -PolicyStore Persistentstore", defaultIPv6Route)
	if out, err := nm.plClient.ExecutePowershellCommand(getIPv6RoutePersistentCmd); err != nil {
		logger.Info("ipv6 default route is not found from persistentstore, adding default ipv6 route to the windows node", zap.String("out", out), zap.Error(err))
		// run powershell cmd to add ipv6 default route
		// if there is an ipv6 default route in active store but not persistent store; to add ipv6 default route to persistent store
		// need to remove ipv6 default route from active store and then use this command to add default route entry to both active and persistent store
		addCmd := fmt.Sprintf("Remove-NetRoute -DestinationPrefix %s -InterfaceIndex %s -NextHop %s -confirm:$false;New-NetRoute -DestinationPrefix %s -InterfaceIndex %s -NextHop %s -confirm:$false",
			defaultIPv6Route, ifIndex, defaultIPv6NextHop, defaultIPv6Route, ifIndex, defaultIPv6NextHop)

		if _, err := nm.plClient.ExecutePowershellCommand(addCmd); err != nil {
			return errors.Wrap(err, "Failed to add ipv6 default route to both persistent and active store")
		}
	}

	return nil
}

// newNetworkImplHnsV2 creates a new container network for HNSv2.
func (nm *networkManager) newNetworkImplHnsV2(nwInfo *EndpointInfo, extIf *externalInterface) (*network, error) {
	// network creation is not required for IB
	if nwInfo.NICType == cns.BackendNIC {
		return &network{Endpoints: make(map[string]*endpoint)}, nil
	}

	hcnNetwork, err := nm.configureHcnNetwork(nwInfo, extIf)
	if err != nil {
		logger.Error("Failed to configure hcn network due to", zap.Error(err))
		return nil, err
	}

	// check if network exists, only create the network does not exist
	hnsResponse, err := Hnsv2.GetNetworkByName(hcnNetwork.Name)

	if err != nil {
		// if network not found, create the HNS network.
		if errors.As(err, &hcn.NetworkNotFoundError{}) {
			logger.Info("Creating hcn network", zap.Any("hcnNetwork", hcnNetwork))
			hnsResponse, err = Hnsv2.CreateNetwork(hcnNetwork)
			if err != nil {
				return nil, fmt.Errorf("Failed to create hcn network: %s due to error: %v", hcnNetwork.Name, err)
			}
			logger.Info("Successfully created hcn network with response", zap.Any("hnsResponse", hnsResponse))
		} else {
			// we can't validate if the network already exists, don't continue
			return nil, fmt.Errorf("Failed to create hcn network: %s, failed to query for existing network with error: %v", hcnNetwork.Name, err)
		}
	} else {
		if hcnNetwork.Type == hcn.Transparent {
			// CNI triggers Add() for new pod first and then delete older pod later
			// for transparent network type, do not ignore network creation if network already exists
			// return error to avoid creating second endpoint
			logger.Error("HNS network with name already exists. Returning error for transparent network", zap.String("networkName", hcnNetwork.Name))
			return nil, fmt.Errorf("HNS network with name:%s already exists. Returning error for transparent network", hcnNetwork.Name) //nolint
		}
		logger.Info("Network with name already exists", zap.String("name", hcnNetwork.Name))
	}

	// check if ipv6 default gateway route is missing before windows endpoint creation
	for i := range nwInfo.Subnets {
		if nwInfo.Subnets[i].Family == platform.AfINET6 {
			if err = nm.addIPv6DefaultRoute(); err != nil {
				// should not block network creation but remind user that it's failed to add ipv6 default route to windows node
				logger.Error("failed to add missing ipv6 default route to windows node active/persistent store", zap.Error(err))
			}
			break
		}
	}

	var vlanid int
	opt, _ := nwInfo.Options[genericData].(map[string]interface{})
	if opt != nil && opt[VlanIDKey] != nil {
		vlanID, _ := strconv.ParseInt(opt[VlanIDKey].(string), baseDecimal, bitSize)
		vlanid = (int)(vlanID)
	}

	// Create the network object.
	nw := &network{
		Id:               nwInfo.NetworkID,
		HnsId:            hnsResponse.Id,
		Mode:             nwInfo.Mode,
		Endpoints:        make(map[string]*endpoint),
		extIf:            extIf,
		VlanId:           vlanid,
		EnableSnatOnHost: nwInfo.EnableSnatOnHost,
		NetNs:            nwInfo.NetNs,
	}

	nwInfo.HNSNetworkID = hnsResponse.Id // we use this later in stateless to clean up in ADD if there is an error

	return nw, nil
}

// NewNetworkImpl creates a new container network.
func (nm *networkManager) newNetworkImpl(nwInfo *EndpointInfo, extIf *externalInterface) (*network, error) {
	if useHnsV2, err := UseHnsV2(nwInfo.NetNs); useHnsV2 {
		if err != nil {
			return nil, err
		}
		return nm.newNetworkImplHnsV2(nwInfo, extIf)
	}
	return nm.newNetworkImplHnsV1(nwInfo, extIf)
}

// DeleteNetworkImpl deletes an existing container network.
func (nm *networkManager) deleteNetworkImpl(nw *network, nicType cns.NICType) error {
	if nicType != cns.NodeNetworkInterfaceFrontendNIC { //nolint
		return nil
	}

	logger.Info("Deleting HNS network", zap.String("networkID", nw.HnsId), zap.Any("nictype", nicType))

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
	logger.Info("HNSNetworkRequest DELETE id", zap.String("id", nw.HnsId))
	hnsResponse, err := Hnsv1.DeleteNetwork(nw.HnsId)
	logger.Info("HNSNetworkRequest DELETE response", zap.Any("hnsResponse", hnsResponse), zap.Error(err))

	return err
}

// DeleteNetworkImplHnsV2 deletes an existing container network using Hnsv2.
func (nm *networkManager) deleteNetworkImplHnsV2(nw *network) error {
	var hcnNetwork *hcn.HostComputeNetwork
	var err error
	logger.Info("Deleting hcn network with id", zap.String("id", nw.HnsId))

	if hcnNetwork, err = Hnsv2.GetNetworkByID(nw.HnsId); err != nil {
		return fmt.Errorf("Failed to get hcn network with id: %s due to err: %v", nw.HnsId, err)
	}

	if err = Hnsv2.DeleteNetwork(hcnNetwork); err != nil {
		return fmt.Errorf("Failed to delete hcn network: %s due to error: %v", nw.HnsId, err)
	}

	logger.Info("Successfully deleted hcn network with id", zap.String("id", nw.HnsId))

	return err
}

func getNetworkInfoImpl(_ *EndpointInfo, _ *network) {
}
