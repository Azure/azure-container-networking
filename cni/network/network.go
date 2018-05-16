// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/telemetry"

	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
)

const (
	// Plugin name.
	name = "azure-vnet"
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

// GetEndpointID returns a unique endpoint ID based on the CNI args.
func GetEndpointID(args *cniSkel.CmdArgs) string {
	infraEpId, _ := network.ConstructEndpointID(args.ContainerID, args.Netns, args.IfName)
	return infraEpId
}

//
// CNI implementation
// https://github.com/containernetworking/cni/blob/master/SPEC.md
//

// Add handles CNI add commands.
func (plugin *netPlugin) Add(args *cniSkel.CmdArgs) error {
	var (
		result   *cniTypesCurr.Result
		err      error
		nwCfg    *cni.NetworkConfig
		ipconfig *cniTypesCurr.IPConfig
		epInfo   *network.EndpointInfo
		iface    *cniTypesCurr.Interface
	)

	log.Printf("[cni-net] Processing ADD command with args {ContainerID:%v Netns:%v IfName:%v Args:%v Path:%v}.",
		args.ContainerID, args.Netns, args.IfName, args.Args, args.Path)

	defer func() {
		// Add Interfaces to result.
		iface = &cniTypesCurr.Interface{
			Name: args.IfName,
		}
		result.Interfaces = append(result.Interfaces, iface)

		// Convert result to the requested CNI version.
		res, err := result.GetAsVersion(nwCfg.CNIVersion)
		if err != nil {
			err = plugin.Error(err)
		}

		// Output the result to stdout.
		res.Print()
		log.Printf("[cni-net] ADD command completed with result:%+v err:%v.", result, err)
	}()

	// Parse Pod arguments.
	podCfg, err := cni.ParseCniArgs(args.Args)
	if err != nil {
		log.Printf("Error while parsing CNI Args %v", err)
		return err
	}

	k8sNamespace := string(podCfg.K8S_POD_NAMESPACE)
	if len(k8sNamespace) == 0 {
		errMsg := "Pod Namespace not specified in CNI Args"
		log.Printf(errMsg)
		return plugin.Errorf(errMsg)
	}

	k8sPodName := string(podCfg.K8S_POD_NAME)
	if len(k8sPodName) == 0 {
		errMsg := "Pod Name not specified in CNI Args"
		log.Printf(errMsg)
		return plugin.Errorf(errMsg)
	}

	// Parse network configuration from stdin.
	nwCfg, err = cni.ParseNetworkConfig(args.StdinData)
	if err != nil {
		err = plugin.Errorf("Failed to parse network configuration: %v.", err)
		return err
	}

	log.Printf("[cni-net] Read network configuration %+v.", nwCfg)

	// Initialize values from network config.
	networkId := nwCfg.Name
	endpointId := GetEndpointID(args)

	nwInfo, nwInfoErr := plugin.nm.GetNetworkInfo(networkId)

	/* Handle consecutive ADD calls for infrastructure containers.
	 * This is a temporary work around for issue #57253 of Kubernetes.
	 * We can delete this if statement once they fix it.
	 * Issue link: https://github.com/kubernetes/kubernetes/issues/57253
	 */
	epInfo, _ = plugin.nm.GetEndpointInfo(networkId, endpointId)
	if epInfo != nil {
		result, err = handleConsecutiveAdd(args.ContainerID, endpointId, nwInfo, nwCfg)
		if err != nil {
			return err
		}

		if result != nil {
			return nil
		}
	}

	policies := cni.GetPoliciesFromNwCfg(nwCfg.AdditionalArgs)

	// Check whether the network already exists.
	if nwInfoErr != nil {
		// Network does not exist.
		log.Printf("[cni-net] Creating network %v.", networkId)

		// Call into IPAM plugin to allocate an address pool for the network.
		result, err = plugin.DelegateAdd(nwCfg.Ipam.Type, nwCfg)
		if err != nil {
			err = plugin.Errorf("Failed to allocate pool: %v", err)
			return err
		}

		// Derive the subnet prefix from allocated IP address.
		ipconfig = result.IPs[0]
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
			DNS: network.DNSInfo{
				Servers: nwCfg.DNS.Nameservers,
				Suffix:  strings.Join(nwCfg.DNS.Search, ","),
			},
			Policies: policies,
		}

		err = plugin.nm.CreateNetwork(&nwInfo)
		if err != nil {
			err = plugin.Errorf("Failed to create network: %v", err)
			return err
		}

		log.Printf("[cni-net] Created network %v with subnet %v.", networkId, subnetPrefix.String())
	} else {
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

		ipconfig = result.IPs[0]

		// On failure, call into IPAM plugin to release the address.
		defer func() {
			if err != nil {
				nwCfg.Ipam.Address = ipconfig.Address.IP.String()
				plugin.DelegateDel(nwCfg.Ipam.Type, nwCfg)
			}
		}()
	}

	// Initialize endpoint info.
	var dns network.DNSInfo
	if (len(nwCfg.DNS.Search) == 0) != (len(nwCfg.DNS.Nameservers) == 0) {
		err = plugin.Errorf("Wrong DNS configuration: %+v", nwCfg.DNS)
		return err
	}

	if len(nwCfg.DNS.Search) > 0 {
		dns = network.DNSInfo{
			Servers: nwCfg.DNS.Nameservers,
			Suffix:  strings.Join(nwCfg.DNS.Search, ","),
		}
	} else {
		dns = network.DNSInfo{
			Suffix:  result.DNS.Domain,
			Servers: result.DNS.Nameservers,
		}
	}

	epInfo = &network.EndpointInfo{
		Id:          endpointId,
		ContainerID: args.ContainerID,
		NetNsPath:   args.Netns,
		IfName:      args.IfName,
		DNS:         dns,
		Policies:    policies,
	}

	// Populate addresses.
	for _, ipconfig := range result.IPs {
		epInfo.IPAddresses = append(epInfo.IPAddresses, ipconfig.Address)
	}

	// Populate routes.
	for _, route := range result.Routes {
		epInfo.Routes = append(epInfo.Routes, network.RouteInfo{Dst: route.Dst, Gw: route.GW})
	}

	epInfo.Data = make(map[string]interface{})
	epInfo.Data[network.OptVethName] = fmt.Sprintf("%s.%s", k8sNamespace, k8sPodName)

	// Create the endpoint.
	log.Printf("[cni-net] Creating endpoint %v.", epInfo.Id)
	err = plugin.nm.CreateEndpoint(networkId, epInfo)
	if err != nil {
		err = plugin.Errorf("Failed to create endpoint: %v", err)
		return err
	}

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
	endpointId := GetEndpointID(args)

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
