package network

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/cnsclient"
	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/Azure/azure-container-networking/platform"
	nnscontracts "github.com/Azure/azure-container-networking/proto/nodenetworkservice/3.302.0.744"
	"github.com/Azure/azure-container-networking/telemetry"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
)

//
// CNI implementation
// https://github.com/containernetworking/cni/blob/master/SPEC.md
//

// Add handles CNI add commands.
func (plugin *netPlugin) Add(args *cniSkel.CmdArgs) error {
	var (
		result           *cniTypesCurr.Result
		resultV6         *cniTypesCurr.Result
		azIpamResult     *cniTypesCurr.Result
		err              error
		vethName         string
		nwCfg            *cni.NetworkConfig
		epInfo           *network.EndpointInfo
		iface            *cniTypesCurr.Interface
		subnetPrefix     net.IPNet
		cnsNetworkConfig *cns.GetNetworkContainerResponse
		enableInfraVnet  bool
		enableSnatForDns bool
		nwDNSInfo        network.DNSInfo
		cniMetric        telemetry.AIMetric
	)

	startTime := time.Now()

	log.Printf("[cni-net] Processing ADD command with args {ContainerID:%v Netns:%v IfName:%v Args:%v Path:%v StdinData:%s}.",
		args.ContainerID, args.Netns, args.IfName, args.Args, args.Path, args.StdinData)

	// Parse network configuration from stdin.
	nwCfg, err = cni.ParseNetworkConfig(args.StdinData)
	if err != nil {
		err = plugin.Errorf("Failed to parse network configuration: %v.", err)
		return err
	}

	log.Printf("[cni-net] Read network configuration %+v.", nwCfg)

	// Temporary if block to determing whether we disable SNAT on host (for multi-tenant scenario only)
	if nwCfg.MultiTenancy {
		if enableSnatForDns, nwCfg.EnableSnatOnHost, err = determineSnat(); err != nil {
			return err
		}
	}

	iptables.DisableIPTableLock = nwCfg.DisableIPTableLock
	plugin.setCNIReportDetails(nwCfg, CNI_ADD, "")

	defer func() {
		operationTimeMs := time.Since(startTime).Milliseconds()
		cniMetric.Metric = aitelemetry.Metric{
			Name:             telemetry.CNIAddTimeMetricStr,
			Value:            float64(operationTimeMs),
			CustomDimensions: make(map[string]string),
		}
		SetCustomDimensions(&cniMetric, nwCfg, err)
		telemetry.SendCNIMetric(&cniMetric, plugin.tb)

		// Add Interfaces to result.
		if result == nil {
			result = &cniTypesCurr.Result{}
		}

		iface = &cniTypesCurr.Interface{
			Name: args.IfName,
		}
		result.Interfaces = append(result.Interfaces, iface)

		if resultV6 != nil {
			result.IPs = append(result.IPs, resultV6.IPs...)
		}

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
	k8sPodName, k8sNamespace, err := plugin.getPodInfo(args.Args)
	if err != nil {
		return err
	}

	plugin.report.ContainerName = k8sPodName + ":" + k8sNamespace

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

	log.Printf("Execution mode :%s", nwCfg.ExecutionMode)
	if nwCfg.ExecutionMode == string(Baremetal) {
		var res *nnscontracts.ConfigureContainerNetworkingResponse
		log.Printf("Baremetal mode. Calling vnet agent for ADD")
		err, res = plugin.nnsClient.AddContainerNetworking(context.Background(), k8sPodName, args.Netns)

		if err == nil {
			result = convertNnsToCniResult(res, args.IfName, k8sPodName, "AddContainerNetworking")
		}

		return err
	}

	if nwCfg.MultiTenancy {
		// Initialize CNSClient
		cnsclient.InitCnsClient(nwCfg.CNSUrl, defaultRequestTimeout)
	}

	for _, ns := range nwCfg.PodNamespaceForDualNetwork {
		if k8sNamespace == ns {
			log.Printf("Enable infravnet for this pod %v in namespace %v", k8sPodName, k8sNamespace)
			enableInfraVnet = true
			break
		}
	}

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

	// Initialize values from network config.
	networkId, err := getNetworkName(k8sPodName, k8sNamespace, args.IfName, nwCfg)
	if err != nil {
		log.Printf("[cni-net] Failed to extract network name from network config. error: %v", err)
		return err
	}

	endpointId := GetEndpointID(args)
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
			resultConsAdd, errConsAdd := handleConsecutiveAdd(args, endpointId, nwInfo, epInfo, nwCfg)
			if errConsAdd != nil {
				log.Printf("handleConsecutiveAdd failed with error %v", errConsAdd)
				result = resultConsAdd
				err = errConsAdd
				return err
			}

			if resultConsAdd != nil {
				result = resultConsAdd
				return nil
			}
		}
	}

	switch nwCfg.Ipam.Type {
	case network.AzureCNS:
		plugin.ipamInvoker, err = NewCNSInvoker(k8sPodName, k8sNamespace)
		if err != nil {
			log.Printf("[cni-net] Creating network %v, failed with err %v", networkId, err)
			return err
		}
	default:
		plugin.ipamInvoker = NewAzureIpamInvoker(plugin, &nwInfo)
	}

	options := make(map[string]interface{})

	if nwInfoErr != nil {
		// Network does not exist.
		log.Printf("[cni-net] Creating network %v.", networkId)

		if !nwCfg.MultiTenancy {
			result, resultV6, err = plugin.ipamInvoker.Add(nwCfg, args, &subnetPrefix, options)
			if err != nil {
				return err
			}

			defer func() {
				if err != nil {
					if result != nil && len(result.IPs) > 0 {
						if er := plugin.ipamInvoker.Delete(&result.IPs[0].Address, nwCfg, args, options); er != nil {
							err = plugin.Errorf("Failed to cleanup when NwInfo was not nil with error %v, after Add failed with error %w", er, err)
						}
					}
					if resultV6 != nil && len(resultV6.IPs) > 0 {
						if er := plugin.ipamInvoker.Delete(&resultV6.IPs[0].Address, nwCfg, args, options); er != nil {
							err = plugin.Errorf("Failed to cleanup when NwInfo was not nil with error %v, after Add failed with error %w", er, err)
						}
					}
				}
			}()
		}

		gateway := result.IPs[0].Gateway
		subnetPrefix.IP = subnetPrefix.IP.Mask(subnetPrefix.Mask)
		nwCfg.Ipam.Subnet = subnetPrefix.String()
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

		nwDNSInfo, err = getNetworkDNSSettings(nwCfg, result, k8sNamespace)
		if err != nil {
			err = plugin.Errorf("Failed to getDNSSettings: %v", err)
			return err
		}

		log.Printf("[cni-net] nwDNSInfo: %v", nwDNSInfo)
		// Update subnet prefix for multi-tenant scenario
		if err = updateSubnetPrefix(cnsNetworkConfig, &subnetPrefix); err != nil {
			err = plugin.Errorf("Failed to updateSubnetPrefix: %v", err)
			return err
		}

		// Create the network.
		nwInfo = network.NetworkInfo{
			Id:           networkId,
			Mode:         nwCfg.Mode,
			MasterIfName: masterIfName,
			AdapterName:  nwCfg.AdapterName,
			Subnets: []network.SubnetInfo{
				{
					Family:  platform.AfINET,
					Prefix:  subnetPrefix,
					Gateway: gateway,
				},
			},
			BridgeName:                    nwCfg.Bridge,
			EnableSnatOnHost:              nwCfg.EnableSnatOnHost,
			DNS:                           nwDNSInfo,
			Policies:                      policies,
			NetNs:                         args.Netns,
			DisableHairpinOnHostInterface: nwCfg.DisableHairpinOnHostInterface,
			IPV6Mode:                      nwCfg.IPV6Mode,
			ServiceCidrs:                  nwCfg.ServiceCidrs,
		}

		nwInfo.IPAMType = nwCfg.Ipam.Type

		if len(result.IPs) > 0 {
			_, podnetwork, err := net.ParseCIDR(result.IPs[0].Address.String())
			if err != nil {
				return err
			}

			nwInfo.PodSubnet = network.SubnetInfo{
				Family:  platform.GetAddressFamily(&result.IPs[0].Address.IP),
				Prefix:  *podnetwork,
				Gateway: result.IPs[0].Gateway,
			}
		}

		nwInfo.Options = options
		setNetworkOptions(cnsNetworkConfig, &nwInfo)

		addNatIPV6SubnetInfo(nwCfg, resultV6, &nwInfo)

		err = plugin.nm.CreateNetwork(&nwInfo)
		if err != nil {
			err = plugin.Errorf("Failed to create network: %v", err)
			return err
		}

		log.Printf("[cni-net] Created network %v with subnet %v.", networkId, subnetPrefix.String())
	} else {
		if !nwCfg.MultiTenancy {
			// Network already exists.
			log.Printf("[cni-net] Found network %v with subnet %v.", networkId, nwInfo.Subnets[0].Prefix.String())
			result, resultV6, err = plugin.ipamInvoker.Add(nwCfg, args, &subnetPrefix, nwInfo.Options)
			if err != nil {
				return err
			}

			nwInfo.IPAMType = nwCfg.Ipam.Type

			defer func() {
				if err != nil {
					if result != nil && len(result.IPs) > 0 {
						if er := plugin.ipamInvoker.Delete(&result.IPs[0].Address, nwCfg, args, nwInfo.Options); er != nil {
							err = plugin.Errorf("Failed to cleanup when NwInfo was nil with error %v, after Add failed with error %w", er, err)
						}
					}
					if resultV6 != nil && len(resultV6.IPs) > 0 {
						if er := plugin.ipamInvoker.Delete(&resultV6.IPs[0].Address, nwCfg, args, nwInfo.Options); er != nil {
							err = plugin.Errorf("Failed to cleanup when NwInfo was nil with error %v, after Add failed with error %w", er, err)
						}
					}
				}
			}()
		}
	}

	epDNSInfo, err := getEndpointDNSSettings(nwCfg, result, k8sNamespace)
	if err != nil {
		err = plugin.Errorf("Failed to getEndpointDNSSettings: %v", err)
		return err
	}

	if nwCfg.IPV6Mode == network.IPV6Nat {
		var ipv6Policy policy.Policy

		ipv6Policy, err = addIPV6EndpointPolicy(nwInfo)
		if err != nil {
			err = plugin.Errorf("Failed to set ipv6 endpoint policy: %v", err)
			return err
		}

		policies = append(policies, ipv6Policy)
	}

	epInfo = &network.EndpointInfo{
		Id:                 endpointId,
		ContainerID:        args.ContainerID,
		NetNsPath:          args.Netns,
		IfName:             args.IfName,
		Data:               make(map[string]interface{}),
		DNS:                epDNSInfo,
		Policies:           policies,
		IPsToRouteViaHost:  nwCfg.IPsToRouteViaHost,
		EnableSnatOnHost:   nwCfg.EnableSnatOnHost,
		EnableMultiTenancy: nwCfg.MultiTenancy,
		EnableInfraVnet:    enableInfraVnet,
		EnableSnatForDns:   enableSnatForDns,
		PODName:            k8sPodName,
		PODNameSpace:       k8sNamespace,
		SkipHotAttachEp:    false, // Hot attach at the time of endpoint creation
		IPV6Mode:           nwCfg.IPV6Mode,
		VnetCidrs:          nwCfg.VnetCidrs,
		ServiceCidrs:       nwCfg.ServiceCidrs,
	}

	epPolicies := getPoliciesFromRuntimeCfg(nwCfg)

	epInfo.Policies = append(epInfo.Policies, epPolicies...)

	// Populate addresses.
	for _, ipconfig := range result.IPs {
		epInfo.IPAddresses = append(epInfo.IPAddresses, ipconfig.Address)
	}

	if resultV6 != nil {
		for _, ipconfig := range resultV6.IPs {
			epInfo.IPAddresses = append(epInfo.IPAddresses, ipconfig.Address)
		}
	}

	// Populate routes.
	for _, route := range result.Routes {
		epInfo.Routes = append(epInfo.Routes, network.RouteInfo{Dst: route.Dst, Gw: route.GW})
	}

	if azIpamResult != nil && azIpamResult.IPs != nil {
		epInfo.InfraVnetIP = azIpamResult.IPs[0].Address
	}

	SetupRoutingForMultitenancy(nwCfg, cnsNetworkConfig, azIpamResult, epInfo, result)

	if nwCfg.Mode == opModeTransparent {
		// this mechanism of using only namespace and name is not unique for different incarnations of POD/container.
		// IT will result in unpredictable behavior if API server decides to
		// reorder DELETE and ADD call for new incarnation of same POD.
		vethName = fmt.Sprintf("%s.%s", k8sNamespace, k8sPodName)
	} else {
		// A runtime must not call ADD twice (without a corresponding DEL) for the same
		// (network name, container id, name of the interface inside the container)
		vethName = fmt.Sprintf("%s%s%s", networkId, k8sContainerID, k8sIfName)
	}
	setEndpointOptions(cnsNetworkConfig, epInfo, vethName)

	// Create the endpoint.
	log.Printf("[cni-net] Creating endpoint %v.", epInfo.Id)
	err = plugin.nm.CreateEndpoint(networkId, epInfo)
	if err != nil {
		err = plugin.Errorf("Failed to create endpoint: %v", err)
		return err
	}

	msg := fmt.Sprintf("CNI ADD succeeded : CNI Version %+v, IP:%+v, Interfaces:%+v, vlanid: %v, podname %v, namespace %v",
		result.CNIVersion, result.IPs, result.Interfaces, epInfo.Data[network.VlanIDKey], k8sPodName, k8sNamespace)
	plugin.setCNIReportDetails(nwCfg, CNI_ADD, msg)

	return nil
}
