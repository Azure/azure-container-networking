package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
)

var (
	errMultitenancySubnetOverlaps             = errors.New("infravnet subnet overlaps")
	errMultitenancySnatOnHostEmptyIP          = errors.New("SNAT IP is not populated with SnatOnHost option enabled")
	errMultitenancyEmptyInfraVnetAddressSpace = errors.New("InfraVnetAddressSpace is not populated. Got empty string")
)

// TODO: use common interface when defined
type cnsclient interface {
	RequestIPAddress(ctx context.Context, ipconfig cns.IPConfigRequest) (*cns.IPConfigResponse, error)
	ReleaseIPAddress(ctx context.Context, ipconfig cns.IPConfigRequest) error
	GetNetworkConfiguration(ctx context.Context, orchestratorContext []byte) (*cns.GetNetworkContainerResponse, error)
}

type multitenancyClient interface {
	SetupRoutingForMultitenancy(nwCfg *cni.NetworkConfig,
		cnsNetworkConfig *cns.GetNetworkContainerResponse,
		azIpamResult *cniTypesCurr.Result,
		epInfo *network.EndpointInfo,
		result *cniTypesCurr.Result)

	GetMultiTenancyCNIResult(
		ctx context.Context,
		enableInfraVnet bool,
		nwCfg *cni.NetworkConfig,
		plugin *netPlugin,
		k8sPodName string,
		k8sNamespace string,
		ifName string) (*cniTypesCurr.Result, *cns.GetNetworkContainerResponse, net.IPNet, *cniTypesCurr.Result, error)

	CleanupMultitenancyResources(enableInfraVnet bool,
		infraIPNet *net.IPNet,
		nwCfg *cni.NetworkConfig,
		plugin *netPlugin)
}

type netioshim interface {
	GetInterfaceSubnetWithSpecificIP(ipAddr string) *net.IPNet
}

type azureMultitenancyClient struct {
	cnsclient cnsclient
	netioshim netioshim
}

func (a *azureMultitenancyClient) SetupRoutingForMultitenancy(
	nwCfg *cni.NetworkConfig,
	cnsNetworkConfig *cns.GetNetworkContainerResponse,
	azIpamResult *cniTypesCurr.Result,
	epInfo *network.EndpointInfo,
	result *cniTypesCurr.Result) {
	// Adding default gateway

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

func (a *azureMultitenancyClient) getContainerNetworkConfiguration(
	ctx context.Context, nwCfg *cni.NetworkConfig, podName1 string, podNamespace string, ifName string) (*cniTypesCurr.Result, *cns.GetNetworkContainerResponse, net.IPNet, error) {
	var podNameWithoutSuffix string

	if !nwCfg.EnableExactMatchForPodName {
		podNameWithoutSuffix = network.GetPodNameWithoutSuffix(podName1)
	} else {
		podNameWithoutSuffix = podName1
	}

	log.Printf("Podname without suffix %v", podNameWithoutSuffix)

	podInfo := cns.KubernetesPodInfo{
		PodName:      podNameWithoutSuffix,
		PodNamespace: podNamespace,
	}
	orchestratorContext, err := json.Marshal(podInfo)
	if err != nil {
		log.Printf("Marshalling KubernetesPodInfo failed with %v", err)
		return nil, nil, net.IPNet{}, err
	}

	networkConfig, err := a.cnsclient.GetNetworkConfiguration(ctx, orchestratorContext)
	if err != nil {
		log.Printf("GetNetworkConfiguration failed with %v", err)
		return nil, nil, net.IPNet{}, err
	}

	log.Printf("Network config received from cns %+v", networkConfig)

	subnetPrefix := a.netioshim.GetInterfaceSubnetWithSpecificIP(networkConfig.PrimaryInterfaceIdentifier)
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
		subnetPrefix := &net.IPNet{}
		options := make(map[string]interface{})
		azIpamResult, _, err := plugin.ipamInvoker.Add(nwCfg, nil, subnetPrefix, options)
		if err != nil {
			err = plugin.Errorf("Failed to allocate address: %v", err)
			return nil, err
		}

		return azIpamResult, nil
	}

	return nil, nil
}

func (a *azureMultitenancyClient) CleanupMultitenancyResources(enableInfraVnet bool, infraIPNet *net.IPNet, nwCfg *cni.NetworkConfig, plugin *netPlugin) {
	log.Printf("Cleanup infravnet ip %v", infraIPNet)
	if enableInfraVnet {
		_, ipNet, _ := net.ParseCIDR(infraIPNet.String())
		nwCfg.Ipam.Subnet = ipNet.String()
		nwCfg.Ipam.Address = infraIPNet.IP.String()
		if err := plugin.DelegateDel(nwCfg.Ipam.Type, nwCfg); err != nil {
			log.Errorf("failed to cleanup infravnet ip with err %w", err)
		}
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
func (a *azureMultitenancyClient) GetMultiTenancyCNIResult(
	ctx context.Context,
	enableInfraVnet bool,
	nwCfg *cni.NetworkConfig,
	plugin *netPlugin,
	k8sPodName string,
	k8sNamespace string,
	ifName string) (*cniTypesCurr.Result, *cns.GetNetworkContainerResponse, net.IPNet, *cniTypesCurr.Result, error) {

	result, cnsNetworkConfig, subnetPrefix, err := a.getContainerNetworkConfiguration(ctx, nwCfg, k8sPodName, k8sNamespace, ifName)
	if err != nil {
		log.Printf("GetContainerNetworkConfiguration failed for podname %v namespace %v with error %v", k8sPodName, k8sNamespace, err)
		return nil, nil, net.IPNet{}, nil, err
	}

	log.Printf("PrimaryInterfaceIdentifier :%v", subnetPrefix.IP.String())

	if checkIfSubnetOverlaps(enableInfraVnet, nwCfg, cnsNetworkConfig) {
		buf := fmt.Sprintf("InfraVnet %v overlaps with customerVnet %+v", nwCfg.InfraVnetAddressSpace, cnsNetworkConfig.CnetAddressSpace)
		log.Printf(buf)
		return nil, nil, net.IPNet{}, nil, errMultitenancySubnetOverlaps
	}

	if nwCfg.EnableSnatOnHost {
		if cnsNetworkConfig.LocalIPConfiguration.IPSubnet.IPAddress == "" {
			log.Printf("Snat IP is not populated. Got empty string")
			return nil, nil, net.IPNet{}, nil, errMultitenancySnatOnHostEmptyIP
		}
	}

	if enableInfraVnet {
		if nwCfg.InfraVnetAddressSpace == "" {
			log.Printf("InfraVnetAddressSpace is not populated. Got empty string")
			return nil, nil, net.IPNet{}, nil, errMultitenancyEmptyInfraVnetAddressSpace
		}
	}

	azIpamResult, err := getInfraVnetIP(enableInfraVnet, subnetPrefix.String(), nwCfg, plugin)
	if err != nil {
		log.Printf("GetInfraVnetIP failed with error %v", err)
		return nil, nil, net.IPNet{}, nil, err
	}

	return result, cnsNetworkConfig, subnetPrefix, azIpamResult, nil
}
