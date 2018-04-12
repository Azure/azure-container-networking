// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"net"

	"github.com/Azure/azure-container-networking/client/cnsclient"
	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/telemetry"

	cniSkel "github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
)

const (
	// Plugin name.
	name         = "azure-vnet"
	namespaceKey = "K8S_POD_NAMESPACE"
	podNameKey   = "K8S_POD_NAME"
)

// NetPlugin represents the CNI network plugin.
type netPlugin struct {
	*cni.Plugin
	nm            network.NetworkManager
	reportManager *telemetry.ReportManager
}

// NewPlugin creates a new netPlugin object.
func NewPlugin(config *common.PluginConfig) (*netPlugin, error) {
	// Setup base plugin.
	plugin, err := cni.NewPlugin(name, config.Version)
	if err != nil {
		return nil, err
	}

	// Setup network manager.
	nm, err := network.NewNetworkManager()
	if err != nil {
		return nil, err
	}

	config.NetApi = nm

	return &netPlugin{
		Plugin: plugin,
		nm:     nm,
	}, nil
}

func (plugin *netPlugin) SetReportManager(reportManager *telemetry.ReportManager) {
	plugin.reportManager = reportManager
}

// Starts the plugin.
func (plugin *netPlugin) Start(config *common.PluginConfig) error {
	// Initialize base plugin.
	err := plugin.Initialize(config)
	if err != nil {
		log.Printf("[cni-net] Failed to initialize base plugin, err:%v.", err)
		return err
	}

	// Log platform information.
	log.Printf("[cni-net] Plugin %v version %v.", plugin.Name, plugin.Version)
	log.Printf("[cni-net] Running on %v", platform.GetOSInfo())
	common.LogNetworkInterfaces()

	// Initialize network manager.
	err = plugin.nm.Initialize(config)
	if err != nil {
		log.Printf("[cni-net] Failed to initialize network manager, err:%v.", err)
		return err
	}

	log.Printf("[cni-net] Plugin started.")

	return nil
}

// Stops the plugin.
func (plugin *netPlugin) Stop() {
	plugin.nm.Uninitialize()
	plugin.Uninitialize()
	log.Printf("[cni-net] Plugin stopped.")
}

// FindMasterInterface returns the name of the master interface.
func (plugin *netPlugin) findMasterInterface(nwCfg *cni.NetworkConfig, subnetPrefix *net.IPNet) string {
	// An explicit master configuration wins. Explicitly specifying a master is
	// useful if host has multiple interfaces with addresses in the same subnet.
	if nwCfg.Master != "" {
		return nwCfg.Master
	}

	// Otherwise, pick the first interface with an IP address in the given subnet.
	subnetPrefixString := subnetPrefix.String()
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			_, ipnet, err := net.ParseCIDR(addr.String())
			if err != nil {
				continue
			}
			if subnetPrefixString == ipnet.String() {
				return iface.Name
			}
		}
	}

	// Failed to find a suitable interface.
	return ""
}

func convertToCniResult(networkConfig *cns.GetNetworkContainerResponse) *cniTypesCurr.Result {
	result := &cniTypesCurr.Result{}
	resultIpconfig := &cniTypesCurr.IPConfig{}

	ipconfig := networkConfig.IPConfiguration
	ipAddr := net.ParseIP(ipconfig.IPSubnet.IPAddress)

	if ipAddr.To4() != nil {
		resultIpconfig.Version = "4"
		resultIpconfig.Address = net.IPNet{IP: ipAddr, Mask: net.CIDRMask(int(ipconfig.IPSubnet.PrefixLength), 32)}
	} else {
		resultIpconfig.Version = "6"
		resultIpconfig.Address = net.IPNet{IP: ipAddr, Mask: net.CIDRMask(int(ipconfig.IPSubnet.PrefixLength), 128)}
	}

	resultIpconfig.Gateway = net.ParseIP(ipconfig.GatewayIPAddress)
	result.IPs = append(result.IPs, resultIpconfig)

	result.DNS.Nameservers = ipconfig.DNSServers

	if networkConfig.Routes == nil && len(networkConfig.Routes) > 0 {
		for _, route := range networkConfig.Routes {
			_, routeIPnet, _ := net.ParseCIDR(route.IPAddress)
			gwIP := net.ParseIP(route.GatewayIPAddress)
			result.Routes = append(result.Routes, &types.Route{Dst: *routeIPnet, GW: gwIP})
		}
	} else {
		gwIP := net.ParseIP(networkConfig.IPConfiguration.GatewayIPAddress)
		dstIP := net.IPNet{IP: net.ParseIP("0.0.0.0"), Mask: resultIpconfig.Address.Mask}
		result.Routes = append(result.Routes, &types.Route{Dst: dstIP, GW: gwIP})
	}

	return result
}

func getContainerNetworkConfiguration(namespace string, podName string) (*cniTypesCurr.Result, int, error) {
	cnsClient, err := cnsclient.NewCnsClient("")
	if err != nil {
		log.Printf("Initializing CNS client error %v", err)
		return nil, 0, err
	}

	networkConfig, err := cnsClient.GetNetworkConfiguration(podName, namespace)
	if err != nil {
		log.Printf("GetNetworkConfiguration failed with %v", err)
		return nil, 0, err
	}

	log.Printf("Network config received from cns %v", networkConfig)

	return convertToCniResult(networkConfig), networkConfig.MultiTenancyInfo.ID, nil
}

//
// CNI implementation
// https://github.com/containernetworking/cni/blob/master/SPEC.md
//

// Add handles CNI add commands.
func (plugin *netPlugin) Add(args *cniSkel.CmdArgs) error {
	var result *cniTypesCurr.Result
	var err error
	var epInfo *network.EndpointInfo
	var vlanid int

	log.Printf("[cni-net] Processing ADD command with args {ContainerID:%v Netns:%v IfName:%v Args:%v Path:%v}.",
		args.ContainerID, args.Netns, args.IfName, args.Args, args.Path)

	defer func() { log.Printf("[cni-net] ADD command completed with result:%+v err:%v.", result, err) }()

	// Parse network configuration from stdin.
	nwCfg, err := cni.ParseNetworkConfig(args.StdinData)
	if err != nil {
		err = plugin.Errorf("Failed to parse network configuration: %v.", err)
		return err
	}

	log.Printf("[cni-net] Read network configuration %+v.", nwCfg)

	// Initialize values from network config.
	networkId := nwCfg.Name
	endpointId := plugin.GetEndpointID(args)

	argsMap := plugin.GetCNIArgs(args.Args)
	if argsMap != nil {
		log.Printf("Argsmap %v", argsMap)
	}

	result, vlanid, err = getContainerNetworkConfiguration(argsMap[namespaceKey].(string), argsMap[podNameKey].(string))
	if err != nil {
		log.Printf("SetContainerNetworkConfiguration failed with %v", err)
	}

	epInfo = &network.EndpointInfo{
		Id:          endpointId,
		ContainerID: args.ContainerID,
		NetNsPath:   args.Netns,
		IfName:      args.IfName,
	}
	epInfo.Data = make(map[string]interface{})

	if vlanid != 0 {
		epInfo.Data["vlanid"] = vlanid
	}

	// Check whether the network already exists.
	nwInfo, err := plugin.nm.GetNetworkInfo(networkId)
	if err != nil {
		// Network does not exist.
		log.Printf("[cni-net] Creating network %v.", networkId)

		if result == nil {
			// Call into IPAM plugin to allocate an address pool for the network.
			result, err = plugin.DelegateAdd(nwCfg.Ipam.Type, nwCfg)
			if err != nil {
				err = plugin.Errorf("Failed to allocate pool: %v", err)
				return err
			}
		}
		// Derive the subnet prefix from allocated IP address.
		ipconfig := result.IPs[0]
		subnetPrefix := ipconfig.Address
		subnetPrefix.IP = subnetPrefix.IP.Mask(subnetPrefix.Mask)

		// On failure, call into IPAM plugin to release the address and address pool.
		defer func() {
			if err != nil {
				nwCfg.Ipam.Subnet = subnetPrefix.String()
				nwCfg.Ipam.Address = ipconfig.Address.IP.String()
				plugin.DelegateDel(nwCfg.Ipam.Type, nwCfg)

				nwCfg.Ipam.Address = ""
				plugin.DelegateDel(nwCfg.Ipam.Type, nwCfg)
			}
		}()

		// Find the master interface.
		masterIfName := plugin.findMasterInterface(nwCfg, &subnetPrefix)
		if masterIfName == "" {
			err = plugin.Errorf("Failed to find the master interface")
			return err
		}
		log.Printf("[cni-net] Found master interface %v.", masterIfName)

		// Add the master as an external interface.
		err = plugin.nm.AddExternalInterface(masterIfName, subnetPrefix.String())
		if err != nil {
			err = plugin.Errorf("Failed to add external interface: %v", err)
			return err
		}

		// Create the network.
		nwInfo := network.NetworkInfo{
			Id:   networkId,
			Mode: nwCfg.Mode,
			Subnets: []network.SubnetInfo{
				network.SubnetInfo{
					Family:  platform.AfINET,
					Prefix:  subnetPrefix,
					Gateway: ipconfig.Gateway,
				},
			},
			BridgeName: nwCfg.Bridge,
		}

		err = plugin.nm.CreateNetwork(&nwInfo)
		if err != nil {
			err = plugin.Errorf("Failed to create network: %v", err)
			return err
		}

		log.Printf("[cni-net] Created network %v with subnet %v.", networkId, subnetPrefix.String())
	} else {
		if result == nil {
			// Network already exists.
			subnetPrefix := nwInfo.Subnets[0].Prefix.String()
			log.Printf("[cni-net] Found network %v with subnet %v.", networkId, subnetPrefix)

			// Call into IPAM plugin to allocate an address for the endpoint.
			nwCfg.Ipam.Subnet = subnetPrefix
			result, err = plugin.DelegateAdd(nwCfg.Ipam.Type, nwCfg)
			if err != nil {
				err = plugin.Errorf("Failed to allocate address: %v", err)
				return err
			}

			ipconfig := result.IPs[0]

			// On failure, call into IPAM plugin to release the address.
			defer func() {
				if err != nil {
					nwCfg.Ipam.Address = ipconfig.Address.IP.String()
					plugin.DelegateDel(nwCfg.Ipam.Type, nwCfg)
				}
			}()
		}
	}

	// Populate addresses.
	for _, ipconfig := range result.IPs {
		epInfo.IPAddresses = append(epInfo.IPAddresses, ipconfig.Address)
	}

	// Populate routes.
	for _, route := range result.Routes {
		epInfo.Routes = append(epInfo.Routes, network.RouteInfo{Dst: route.Dst, Gw: route.GW})
	}

	// Populate DNS info.
	epInfo.DNS.Suffix = result.DNS.Domain
	epInfo.DNS.Servers = result.DNS.Nameservers

	// Create the endpoint.
	log.Printf("[cni-net] Creating endpoint %v.", epInfo.Id)
	err = plugin.nm.CreateEndpoint(networkId, epInfo)
	if err != nil {
		err = plugin.Errorf("Failed to create endpoint: %v", err)
		return err
	}

	// Add Interfaces to result.
	iface := &cniTypesCurr.Interface{
		Name: epInfo.IfName,
	}
	result.Interfaces = append(result.Interfaces, iface)

	// Convert result to the requested CNI version.
	res, err := result.GetAsVersion(nwCfg.CNIVersion)
	if err != nil {
		err = plugin.Error(err)
		return err
	}

	// Output the result to stdout.
	res.Print()

	return nil
}

// Delete handles CNI delete commands.
func (plugin *netPlugin) Delete(args *cniSkel.CmdArgs) error {
	var err error

	log.Printf("[cni-net] Processing DEL command with args {ContainerID:%v Netns:%v IfName:%v Args:%v Path:%v}.",
		args.ContainerID, args.Netns, args.IfName, args.Args, args.Path)

	defer func() { log.Printf("[cni-net] DEL command completed with err:%v.", err) }()

	// Parse network configuration from stdin.
	nwCfg, err := cni.ParseNetworkConfig(args.StdinData)
	if err != nil {
		err = plugin.Errorf("Failed to parse network configuration: %v", err)
		return err
	}

	log.Printf("[cni-net] Read network configuration %+v.", nwCfg)

	// Initialize values from network config.
	networkId := nwCfg.Name
	endpointId := plugin.GetEndpointID(args)

	// Query the network.
	nwInfo, err := plugin.nm.GetNetworkInfo(networkId)
	if err != nil {
		// Log the error but return success if the endpoint being deleted is not found.
		plugin.Errorf("Failed to query network: %v", err)
		err = nil
		return err
	}

	// Query the endpoint.
	epInfo, err := plugin.nm.GetEndpointInfo(networkId, endpointId)
	if err != nil {
		// Log the error but return success if the endpoint being deleted is not found.
		plugin.Errorf("Failed to query endpoint: %v", err)
		err = nil
		return err
	}

	// Delete the endpoint.
	err = plugin.nm.DeleteEndpoint(networkId, endpointId)
	if err != nil {
		err = plugin.Errorf("Failed to delete endpoint: %v", err)
		return err
	}

	// Call into IPAM plugin to release the endpoint's addresses.
	nwCfg.Ipam.Subnet = nwInfo.Subnets[0].Prefix.String()
	for _, address := range epInfo.IPAddresses {
		nwCfg.Ipam.Address = address.IP.String()
		err = plugin.DelegateDel(nwCfg.Ipam.Type, nwCfg)
		if err != nil {
			err = plugin.Errorf("Failed to release address: %v", err)
			return err
		}
	}

	return nil
}
