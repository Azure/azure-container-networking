// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/telemetry"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
)

const (
	// Plugin name.
	name                = "azure-vnet"
	dockerNetworkOption = "com.docker.network.generic"

	// Supported IP version. Currently support only IPv4
	ipVersion = "4"
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
	log.Close()
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
		result           *cniTypesCurr.Result
		azIpamResult     *cniTypesCurr.Result
		err              error
		nwCfg            *cni.NetworkConfig
		epInfo           *network.EndpointInfo
		iface            *cniTypesCurr.Interface
		subnetPrefix     net.IPNet
		cnsNetworkConfig *cns.GetNetworkContainerResponse
		enableInfraVnet  bool
	)

	log.Printf("[cni-net] Processing ADD command with args {ContainerID:%v Netns:%v IfName:%v Args:%v Path:%v}.",
		args.ContainerID, args.Netns, args.IfName, args.Args, args.Path)

	// Parse network configuration from stdin.
	nwCfg, err = cni.ParseNetworkConfig(args.StdinData)
	if err != nil {
		err = plugin.Errorf("Failed to parse network configuration: %v.", err)
		return err
	}

	log.Printf("[cni-net] Read network configuration %+v.", nwCfg)

	defer func() {
		// Add Interfaces to result.
		if result == nil {
			result = &cniTypesCurr.Result{}
		}

		iface = &cniTypesCurr.Interface{
			Name: args.IfName,
		}

		result.Interfaces = append(result.Interfaces, iface)

		addSnatInterface(nwCfg, result)

		// Convert result to the requested CNI version.
		res, vererr := result.GetAsVersion(nwCfg.CNIVersion)
		if vererr != nil {
			log.Printf("GetAsVersion failed with error %v", vererr)
			plugin.Error(vererr)
		}

		if err == nil && res != nil {
			// Output the result to stdout.
			res.Print()
		}

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

	k8sContainerID := args.ContainerID
	if len(k8sContainerID) == 0 {
		errMsg := "Container ID not specified in CNI Args"
		log.Printf(errMsg)
		return plugin.Errorf(errMsg)
	}

	k8sIfName := args.IfName
	if len(k8sIfName) == 0 {
		errMsg := "Interfacename not specified in CNI Args"
		log.Printf(errMsg)
		return plugin.Errorf(errMsg)
	}

	for _, ns := range nwCfg.PodNamespaceForDualNetwork {
		if k8sNamespace == ns {
			log.Printf("Enable infravnet for this pod %v in namespace %v", k8sPodName, k8sNamespace)
			enableInfraVnet = true
			break
		}
	}

	// Initialize values from network config.
	networkId := nwCfg.Name
	endpointId := GetEndpointID(args)

	result, cnsNetworkConfig, subnetPrefix, azIpamResult, err = GetMultiTenancyCNIResult(enableInfraVnet, nwCfg, plugin, k8sPodName, k8sNamespace, args.IfName)
	if err != nil {
		log.Printf("GetMultiTenancyCNIResult failed with error %v", err)
		return err
	}

	defer func() {
		if err != nil {
			CleanupMultitenancyResources(enableInfraVnet, nwCfg, azIpamResult, plugin)
		}
	}()

	log.Printf("Result from multitenancy %+v", result)

	policies := cni.GetPoliciesFromNwCfg(nwCfg.AdditionalArgs)

	// Check whether the network already exists.
	nwInfo, nwInfoErr := plugin.nm.GetNetworkInfo(networkId)

	if nwInfoErr == nil {
		/* Handle consecutive ADD calls for infrastructure containers.
		* This is a temporary work around for issue #57253 of Kubernetes.
		* We can delete this if statement once they fix it.
		* Issue link: https://github.com/kubernetes/kubernetes/issues/57253
		 */
		epInfo, _ := plugin.nm.GetEndpointInfo(networkId, endpointId)
		if epInfo != nil {
			result, err = handleConsecutiveAdd(args.ContainerID, endpointId, nwInfo, nwCfg)
			if err != nil {
				log.Printf("handleConsecutiveAdd failed with error %v", err)
				return err
			}

			if result != nil {
				return nil
			}
		}
	}

	if nwInfoErr != nil {
		// Network does not exist.

		log.Printf("[cni-net] Creating network %v.", networkId)

		if !nwCfg.MultiTenancy {
			// Call into IPAM plugin to allocate an address pool for the network.
			result, err = plugin.DelegateAdd(nwCfg.Ipam.Type, nwCfg)
			if err != nil {
				err = plugin.Errorf("Failed to allocate pool: %v", err)
				return err
			}

			// Derive the subnet prefix from allocated IP address.
			subnetPrefix = result.IPs[0].Address

			iface := &cniTypesCurr.Interface{Name: args.IfName}
			result.Interfaces = append(result.Interfaces, iface)
		}

		ipconfig := result.IPs[0]
		gateway := ipconfig.Gateway

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

		subnetPrefix.IP = subnetPrefix.IP.Mask(subnetPrefix.Mask)
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

		nwDNSInfo, err := getNetworkDNSSettings(nwCfg, result, k8sNamespace)
		if err != nil {
			err = plugin.Errorf("Failed to getDNSSettings: %v", err)
			return err
		}

		log.Printf("[cni-net] nwDNSInfo: %v", nwDNSInfo)

		// Create the network.
		nwInfo := network.NetworkInfo{
			Id:   networkId,
			Mode: nwCfg.Mode,
			Subnets: []network.SubnetInfo{
				network.SubnetInfo{
					Family:  platform.AfINET,
					Prefix:  subnetPrefix,
					Gateway: gateway,
				},
			},
			BridgeName:       nwCfg.Bridge,
			EnableSnatOnHost: nwCfg.EnableSnatOnHost,
			DNS:              nwDNSInfo,
			Policies:         policies,
		}

		nwInfo.Options = make(map[string]interface{})
		setNetworkOptions(cnsNetworkConfig, &nwInfo)

		err = plugin.nm.CreateNetwork(&nwInfo)
		if err != nil {
			err = plugin.Errorf("Failed to create network: %v", err)
			return err
		}

		log.Printf("[cni-net] Created network %v with subnet %v.", networkId, subnetPrefix.String())
	} else {
		if !nwCfg.MultiTenancy {
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

			iface := &cniTypesCurr.Interface{Name: args.IfName}
			result.Interfaces = append(result.Interfaces, iface)

			// On failure, call into IPAM plugin to release the address.
			defer func() {
				if err != nil {
					nwCfg.Ipam.Address = ipconfig.Address.IP.String()
					plugin.DelegateDel(nwCfg.Ipam.Type, nwCfg)
				}
			}()
		}
	}

	epDNSInfo, err := getEndpointDNSSettings(nwCfg, result, k8sNamespace)
	if err != nil {
		err = plugin.Errorf("Failed to getEndpointDNSSettings: %v", err)
		return err
	}

	epInfo = &network.EndpointInfo{
		Id:               endpointId,
		ContainerID:      args.ContainerID,
		NetNsPath:        args.Netns,
		IfName:           args.IfName,
		EnableSnatOnHost: nwCfg.EnableSnatOnHost,
		EnableInfraVnet:  enableInfraVnet,
		Data:             make(map[string]interface{}),
		DNS:              epDNSInfo,
		Policies:         policies,
	}
	
	epPolicies := getPoliciesFromRuntimeCfg(nwCfg)
	for _, epPolicy := range epPolicies {
		epInfo.Policies = append(epInfo.Policies, epPolicy)
	}

	// Populate addresses.
	for _, ipconfig := range result.IPs {
		epInfo.IPAddresses = append(epInfo.IPAddresses, ipconfig.Address)
	}

	// Populate routes.
	for _, route := range result.Routes {
		epInfo.Routes = append(epInfo.Routes, network.RouteInfo{Dst: route.Dst, Gw: route.GW})
	}

	if azIpamResult != nil && azIpamResult.IPs != nil {
		epInfo.InfraVnetIP = azIpamResult.IPs[0].Address
	}

	SetupRoutingForMultitenancy(nwCfg, cnsNetworkConfig, azIpamResult, epInfo, result)

	// A runtime must not call ADD twice (without a corresponding DEL) for the same
	// (network name, container id, name of the interface inside the container)
	vethName := fmt.Sprintf("%s%s%s", networkId, k8sContainerID, k8sIfName)
	setEndpointOptions(cnsNetworkConfig, epInfo, vethName)

	// Create the endpoint.
	log.Printf("[cni-net] Creating endpoint %v.", epInfo.Id)
	err = plugin.nm.CreateEndpoint(networkId, epInfo)
	if err != nil {
		err = plugin.Errorf("Failed to create endpoint: %v", err)
		return err
	}

	return nil
}

// Get handles CNI Get commands.
func (plugin *netPlugin) Get(args *cniSkel.CmdArgs) error {
	var (
		result cniTypesCurr.Result
		err    error
		nwCfg  *cni.NetworkConfig
		epInfo *network.EndpointInfo
		iface  *cniTypesCurr.Interface
	)

	log.Printf("[cni-net] Processing GET command with args {ContainerID:%v Netns:%v IfName:%v Args:%v Path:%v}.",
		args.ContainerID, args.Netns, args.IfName, args.Args, args.Path)

	defer func() {
		// Add Interfaces to result.
		iface = &cniTypesCurr.Interface{
			Name: args.IfName,
		}
		result.Interfaces = append(result.Interfaces, iface)

		if err == nil {
			// Convert result to the requested CNI version.
			res, err := result.GetAsVersion(nwCfg.CNIVersion)
			if err != nil {
				err = plugin.Error(err)
			}
			// Output the result to stdout.
			res.Print()
		}

		log.Printf("[cni-net] GET command completed with result:%+v err:%v.", result, err)
	}()

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

	// Query the network.
	_, err = plugin.nm.GetNetworkInfo(networkId)
	if err != nil {
		plugin.Errorf("Failed to query network: %v", err)
		return err
	}

	// Query the endpoint.
	epInfo, err = plugin.nm.GetEndpointInfo(networkId, endpointId)
	if err != nil {
		plugin.Errorf("Failed to query endpoint: %v", err)
		return err
	}

	for _, ipAddresses := range epInfo.IPAddresses {
		ipConfig := &cniTypesCurr.IPConfig{
			Version:   ipVersion,
			Interface: &epInfo.IfIndex,
			Address:   ipAddresses,
		}

		if epInfo.Gateways != nil {
			ipConfig.Gateway = epInfo.Gateways[0]
		}

		result.IPs = append(result.IPs, ipConfig)
	}

	for _, route := range epInfo.Routes {
		result.Routes = append(result.Routes, &cniTypes.Route{Dst: route.Dst, GW: route.Gw})
	}

	result.DNS.Nameservers = epInfo.DNS.Servers
	result.DNS.Domain = epInfo.DNS.Suffix

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

	if !nwCfg.MultiTenancy {
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
	} else if epInfo.EnableInfraVnet {
		nwCfg.Ipam.Subnet = nwInfo.Subnets[0].Prefix.String()
		nwCfg.Ipam.Address = epInfo.InfraVnetIP.IP.String()
		err = plugin.DelegateDel(nwCfg.Ipam.Type, nwCfg)
		if err != nil {
			err = plugin.Errorf("Failed to release address: %v", err)
			return err
		}
	}

	return nil
}
