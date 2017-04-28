// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"net"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/platform"

	cniInvoke "github.com/containernetworking/cni/pkg/invoke"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesImpl "github.com/containernetworking/cni/pkg/types/020"
)

const (
	// Plugin name.
	name = "azure-vnet"
)

// NetPlugin represents the CNI network plugin.
type netPlugin struct {
	*cni.Plugin
	nm network.NetworkManager
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

// GetEndpointID returns a unique endpoint ID based on the CNI args.
func (plugin *netPlugin) getEndpointID(args *cniSkel.CmdArgs) string {
	var containerID string
	if len(args.ContainerID) >= 8 {
		containerID = args.ContainerID[:8] + "-" + args.IfName
	} else {
		containerID = args.ContainerID + "-" + args.IfName
	}
	return containerID
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

//
// CNI implementation
// https://github.com/containernetworking/cni/blob/master/SPEC.md
//

// Add handles CNI add commands.
func (plugin *netPlugin) Add(args *cniSkel.CmdArgs) error {
	log.Printf("[cni-net] Processing ADD command with args {ContainerID:%v Netns:%v IfName:%v Args:%v Path:%v}.",
		args.ContainerID, args.Netns, args.IfName, args.Args, args.Path)

	// Parse network configuration from stdin.
	nwCfg, err := cni.ParseNetworkConfig(args.StdinData)
	if err != nil {
		return plugin.Errorf("Failed to parse network configuration: %v.", err)
	}

	log.Printf("[cni-net] Read network configuration %+v.", nwCfg)

	// Initialize values from network config.
	var result cniTypes.Result
	var resultImpl *cniTypesImpl.Result
	networkId := nwCfg.Name
	endpointId := plugin.getEndpointID(args)

	// Check whether the network already exists.
	nwInfo, err := plugin.nm.GetNetworkInfo(networkId)
	if err != nil {
		// Network does not exist.
		log.Printf("[cni-net] Creating network %v.", networkId)

		// Call into IPAM plugin to allocate an address pool for the network.
		result, err = cniInvoke.DelegateAdd(nwCfg.Ipam.Type, nwCfg.Serialize())
		if err != nil {
			return plugin.Errorf("Failed to allocate pool: %v", err)
		}

		resultImpl, err = cniTypesImpl.GetResult(result)

		log.Printf("[cni-net] IPAM plugin returned result %v.", resultImpl)

		// Derive the subnet prefix from allocated IP address.
		subnetPrefix := resultImpl.IP4.IP
		subnetPrefix.IP = subnetPrefix.IP.Mask(subnetPrefix.Mask)

		// Find the master interface.
		masterIfName := plugin.findMasterInterface(nwCfg, &subnetPrefix)
		if masterIfName == "" {
			return plugin.Errorf("Failed to find the master interface")
		}
		log.Printf("[cni-net] Found master interface %v.", masterIfName)

		// Add the master as an external interface.
		err = plugin.nm.AddExternalInterface(masterIfName, subnetPrefix.String())
		if err != nil {
			return plugin.Errorf("Failed to add external interface: %v", err)
		}

		// Create the network.
		nwInfo := network.NetworkInfo{
			Id:   networkId,
			Mode: nwCfg.Mode,
			Subnets: []network.SubnetInfo{
				network.SubnetInfo{
					Family:  platform.AfINET,
					Prefix:  subnetPrefix,
					Gateway: resultImpl.IP4.Gateway,
				},
			},
			BridgeName: nwCfg.Bridge,
		}

		err = plugin.nm.CreateNetwork(&nwInfo)
		if err != nil {
			return plugin.Errorf("Failed to create network: %v", err)
		}

		log.Printf("[cni-net] Created network %v with subnet %v.", networkId, subnetPrefix.String())
	} else {
		// Network already exists.
		subnetPrefix := nwInfo.Subnets[0].Prefix.String()
		log.Printf("[cni-net] Found network %v with subnet %v.", networkId, subnetPrefix)

		// Call into IPAM plugin to allocate an address for the endpoint.
		nwCfg.Ipam.Subnet = subnetPrefix
		result, err = cniInvoke.DelegateAdd(nwCfg.Ipam.Type, nwCfg.Serialize())
		if err != nil {
			return plugin.Errorf("Failed to allocate address: %v", err)
		}

		resultImpl, err = cniTypesImpl.GetResult(result)

		log.Printf("[cni-net] IPAM plugin returned result %v.", resultImpl)
	}

	// Initialize endpoint info.
	epInfo := &network.EndpointInfo{
		Id:          endpointId,
		ContainerID: args.ContainerID,
		NetNsPath:   args.Netns,
		IfName:      args.IfName,
	}

	// Populate addresses and routes.
	if resultImpl.IP4 != nil {
		epInfo.IPAddresses = append(epInfo.IPAddresses, resultImpl.IP4.IP)

		for _, route := range resultImpl.IP4.Routes {
			epInfo.Routes = append(epInfo.Routes, network.RouteInfo{Dst: route.Dst, Gw: route.GW})
		}
	}

	// Populate DNS info.
	epInfo.DNS.Suffix = resultImpl.DNS.Domain
	epInfo.DNS.Servers = resultImpl.DNS.Nameservers

	// Create the endpoint.
	log.Printf("[cni-net] Creating endpoint %v.", epInfo.Id)
	err = plugin.nm.CreateEndpoint(networkId, epInfo)
	if err != nil {
		return plugin.Errorf("Failed to create endpoint: %v", err)
	}

	// Convert result to the requested CNI version.
	result, err = resultImpl.GetAsVersion(nwCfg.CniVersion)
	if err != nil {
		return plugin.Error(err)
	}

	// Output the result to stdout.
	result.Print()

	log.Printf("[cni-net] ADD succeeded with output %+v.", result)

	return nil
}

// Delete handles CNI delete commands.
func (plugin *netPlugin) Delete(args *cniSkel.CmdArgs) error {
	log.Printf("[cni-net] Processing DEL command with args {ContainerID:%v Netns:%v IfName:%v Args:%v Path:%v}.",
		args.ContainerID, args.Netns, args.IfName, args.Args, args.Path)

	// Parse network configuration from stdin.
	nwCfg, err := cni.ParseNetworkConfig(args.StdinData)
	if err != nil {
		return plugin.Errorf("Failed to parse network configuration: %v", err)
	}

	log.Printf("[cni-net] Read network configuration %+v.", nwCfg)

	// Initialize values from network config.
	networkId := nwCfg.Name
	endpointId := plugin.getEndpointID(args)

	// Query the network.
	nwInfo, err := plugin.nm.GetNetworkInfo(networkId)
	if err != nil {
		return plugin.Errorf("Failed to query network: %v", err)
	}

	// Query the endpoint.
	epInfo, err := plugin.nm.GetEndpointInfo(networkId, endpointId)
	if err != nil {
		return plugin.Errorf("Failed to query endpoint: %v", err)
	}

	// Delete the endpoint.
	err = plugin.nm.DeleteEndpoint(networkId, endpointId)
	if err != nil {
		return plugin.Errorf("Failed to delete endpoint: %v", err)
	}

	// Call into IPAM plugin to release the endpoint's addresses.
	nwCfg.Ipam.Subnet = nwInfo.Subnets[0].Prefix.String()
	for _, address := range epInfo.IPAddresses {
		nwCfg.Ipam.Address = address.IP.String()
		err = cniInvoke.DelegateDel(nwCfg.Ipam.Type, nwCfg.Serialize())
		if err != nil {
			return plugin.Errorf("Failed to release address: %v", err)
		}
	}

	log.Printf("[cni-net] DEL succeeded.")

	return nil
}
