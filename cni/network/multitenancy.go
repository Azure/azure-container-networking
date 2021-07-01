package network

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/cnsclient"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
)

func SetupRoutingForMultitenancy(
	nwCfg *cni.NetworkConfig,
	cnsNetworkConfig *cns.GetNetworkContainerResponse,
	azIpamResult *cniTypesCurr.Result,
	epInfo *network.EndpointInfo,
	result *cniTypesCurr.Result) {
	// Adding default gateway
	if nwCfg.MultiTenancy {
		// if snat enabled, add 169.254.128.1 as default gateway
		if nwCfg.EnableSnatOnHost {
			log.Printf("add default route for multitenancy.snat on host enabled")
			addDefaultRoute(cnsNetworkConfig.LocalIPConfiguration.GatewayIPAddress, epInfo, result)
		} else {
			_, defaultIPNet, _ := net.ParseCIDR("0.0.0.0/0")
			dstIP := net.IPNet{IP: net.ParseIP("0.0.0.0"), Mask: defaultIPNet.Mask}
			gwIP := net.ParseIP(cnsNetworkConfig.IPConfiguration.GatewayIPAddress)
			epInfo.Routes = append(epInfo.Routes, network.RouteInfo{Dst: dstIP, Gw: gwIP})
			result.Routes = append(result.Routes, &cniTypes.Route{Dst: dstIP, GW: gwIP})

			if epInfo.EnableSnatForDns {
				log.Printf("add SNAT for DNS enabled")
				addSnatForDNS(cnsNetworkConfig.LocalIPConfiguration.GatewayIPAddress, epInfo, result)
			}
		}

		setupInfraVnetRoutingForMultitenancy(nwCfg, azIpamResult, epInfo, result)
	}
}

func getContainerNetworkConfiguration(
	nwCfg *cni.NetworkConfig,
	podName string,
	podNamespace string,
	ifName string) (*cniTypesCurr.Result, *cns.GetNetworkContainerResponse, net.IPNet, error) {
	var podNameWithoutSuffix string

	if !nwCfg.EnableExactMatchForPodName {
		podNameWithoutSuffix = network.GetPodNameWithoutSuffix(podName)
	} else {
		podNameWithoutSuffix = podName
	}

	log.Printf("Podname without suffix %v", podNameWithoutSuffix)
	return getContainerNetworkConfigurationInternal(nwCfg.CNSUrl, podNamespace, podNameWithoutSuffix, ifName)
}

func getContainerNetworkConfigurationInternal(address string, namespace string, podName string, ifName string) (*cniTypesCurr.Result, *cns.GetNetworkContainerResponse, net.IPNet, error) {
	cnsClient, err := cnsclient.GetCnsClient()
	if err != nil {
		log.Printf("Failed to get CNS client. Error: %v", err)
		return nil, nil, net.IPNet{}, err
	}

	podInfo := cns.KubernetesPodInfo{
		PodName:      podName,
		PodNamespace: namespace,
	}
	orchestratorContext, err := json.Marshal(podInfo)
	if err != nil {
		log.Printf("Marshalling KubernetesPodInfo failed with %v", err)
		return nil, nil, net.IPNet{}, err
	}

	networkConfig, err := cnsClient.GetNetworkConfiguration(orchestratorContext)
	if err != nil {
		log.Printf("GetNetworkConfiguration failed with %v", err)
		return nil, nil, net.IPNet{}, err
	}

	log.Printf("Network config received from cns %+v", networkConfig)

	subnetPrefix := common.GetInterfaceSubnetWithSpecificIp(networkConfig.PrimaryInterfaceIdentifier)
	if subnetPrefix == nil {
		errBuf := fmt.Sprintf("Interface not found for this ip %v", networkConfig.PrimaryInterfaceIdentifier)
		log.Printf(errBuf)
		return nil, nil, net.IPNet{}, fmt.Errorf(errBuf)
	}

	return convertToCniResult(networkConfig, ifName), networkConfig, *subnetPrefix, nil
}

func convertToCniResult(networkConfig *cns.GetNetworkContainerResponse, ifName string) *cniTypesCurr.Result {
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

	if networkConfig.Routes != nil && len(networkConfig.Routes) > 0 {
		for _, route := range networkConfig.Routes {
			_, routeIPnet, _ := net.ParseCIDR(route.IPAddress)
			gwIP := net.ParseIP(route.GatewayIPAddress)
			result.Routes = append(result.Routes, &cniTypes.Route{Dst: *routeIPnet, GW: gwIP})
		}
	}

	var sb strings.Builder
	sb.WriteString("Adding cnetAddressspace routes ")
	for _, ipRouteSubnet := range networkConfig.CnetAddressSpace {
		sb.WriteString(ipRouteSubnet.IPAddress + "/" + strconv.Itoa((int)(ipRouteSubnet.PrefixLength)) + ", ")
		routeIPnet := net.IPNet{IP: net.ParseIP(ipRouteSubnet.IPAddress), Mask: net.CIDRMask(int(ipRouteSubnet.PrefixLength), 32)}
		gwIP := net.ParseIP(ipconfig.GatewayIPAddress)
		result.Routes = append(result.Routes, &cniTypes.Route{Dst: routeIPnet, GW: gwIP})
	}

	log.Printf(sb.String())

	iface := &cniTypesCurr.Interface{Name: ifName}
	result.Interfaces = append(result.Interfaces, iface)

	return result
}

func getInfraVnetIP(
	enableInfraVnet bool,
	infraSubnet string,
	nwCfg *cni.NetworkConfig,
	plugin *netPlugin,
) (*cniTypesCurr.Result, error) {

	if enableInfraVnet {
		_, ipNet, _ := net.ParseCIDR(infraSubnet)
		nwCfg.Ipam.Subnet = ipNet.String()

		log.Printf("call ipam to allocate ip from subnet %v", nwCfg.Ipam.Subnet)
		azIpamResult, err := plugin.DelegateAdd(nwCfg.Ipam.Type, nwCfg)
		if err != nil {
			err = plugin.Errorf("Failed to allocate address: %v", err)
			return nil, err
		}

		return azIpamResult, nil
	}

	return nil, nil
}

func cleanupInfraVnetIP(
	enableInfraVnet bool,
	infraIPNet *net.IPNet,
	nwCfg *cni.NetworkConfig,
	plugin *netPlugin) {

	log.Printf("Cleanup infravnet ip")

	if enableInfraVnet {
		_, ipNet, _ := net.ParseCIDR(infraIPNet.String())
		nwCfg.Ipam.Subnet = ipNet.String()
		nwCfg.Ipam.Address = infraIPNet.IP.String()
		plugin.DelegateDel(nwCfg.Ipam.Type, nwCfg)
	}
}

func checkIfSubnetOverlaps(enableInfraVnet bool, nwCfg *cni.NetworkConfig, cnsNetworkConfig *cns.GetNetworkContainerResponse) bool {
	if enableInfraVnet {
		if cnsNetworkConfig != nil {
			_, infraNet, _ := net.ParseCIDR(nwCfg.InfraVnetAddressSpace)
			for _, cnetSpace := range cnsNetworkConfig.CnetAddressSpace {
				cnetSpaceIPNet := &net.IPNet{
					IP:   net.ParseIP(cnetSpace.IPAddress),
					Mask: net.CIDRMask(int(cnetSpace.PrefixLength), 32),
				}

				return infraNet.Contains(cnetSpaceIPNet.IP) || cnetSpaceIPNet.Contains(infraNet.IP)
			}
		}
	}

	return false
}

// GetMultiTenancyCNIResult retrieves network goal state of a container from CNS
func GetMultiTenancyCNIResult(
	enableInfraVnet bool,
	nwCfg *cni.NetworkConfig,
	plugin *netPlugin,
	k8sPodName string,
	k8sNamespace string,
	ifName string) (*cniTypesCurr.Result, *cns.GetNetworkContainerResponse, net.IPNet, *cniTypesCurr.Result, error) {

	if nwCfg.MultiTenancy {
		result, cnsNetworkConfig, subnetPrefix, err := getContainerNetworkConfiguration(nwCfg, k8sPodName, k8sNamespace, ifName)
		if err != nil {
			log.Printf("GetContainerNetworkConfiguration failed for podname %v namespace %v with error %v", k8sPodName, k8sNamespace, err)
			return nil, nil, net.IPNet{}, nil, err
		}

		log.Printf("PrimaryInterfaceIdentifier :%v", subnetPrefix.IP.String())

		if checkIfSubnetOverlaps(enableInfraVnet, nwCfg, cnsNetworkConfig) {
			buf := fmt.Sprintf("InfraVnet %v overlaps with customerVnet %+v", nwCfg.InfraVnetAddressSpace, cnsNetworkConfig.CnetAddressSpace)
			log.Printf(buf)
			err = errors.New(buf)
			return nil, nil, net.IPNet{}, nil, err
		}

		if nwCfg.EnableSnatOnHost {
			if cnsNetworkConfig.LocalIPConfiguration.IPSubnet.IPAddress == "" {
				log.Printf("Snat IP is not populated. Got empty string")
				return nil, nil, net.IPNet{}, nil, fmt.Errorf("Snat IP is not populated. Got empty string")
			}
		}

		if enableInfraVnet {
			if nwCfg.InfraVnetAddressSpace == "" {
				log.Printf("InfraVnetAddressSpace is not populated. Got empty string")
				return nil, nil, net.IPNet{}, nil, fmt.Errorf("InfraVnetAddressSpace is not populated. Got empty string")
			}
		}

		azIpamResult, err := getInfraVnetIP(enableInfraVnet, subnetPrefix.String(), nwCfg, plugin)
		if err != nil {
			log.Printf("GetInfraVnetIP failed with error %v", err)
			return nil, nil, net.IPNet{}, nil, err
		}

		return result, cnsNetworkConfig, subnetPrefix, azIpamResult, nil
	}

	return nil, nil, net.IPNet{}, nil, nil
}

func CleanupMultitenancyResources(enableInfraVnet bool, nwCfg *cni.NetworkConfig, azIpamResult *cniTypesCurr.Result, plugin *netPlugin) {
	if nwCfg.MultiTenancy && azIpamResult != nil && azIpamResult.IPs != nil {
		cleanupInfraVnetIP(enableInfraVnet, &azIpamResult.IPs[0].Address, nwCfg, plugin)
	}
}
