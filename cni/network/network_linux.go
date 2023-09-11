package network

import (
	"net"
	"strconv"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/network/policy"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/100"
)

const (
	snatInterface  = "eth1"
	infraInterface = "eth2"
)

const snatConfigFileName = "/tmp/snatConfig"

// handleConsecutiveAdd is a dummy function for Linux platform.
func (plugin *NetPlugin) handleConsecutiveAdd(args *cniSkel.CmdArgs, endpointID string, networkID string,
	nwInfo *network.NetworkInfo, nwCfg *cni.NetworkConfig,
) (*cniTypesCurr.Result, error) {
	return nil, nil
}

func addDefaultRoute(gwIPString string, epInfo *network.EndpointInfo, result *cniTypesCurr.Result) {
	_, defaultIPNet, _ := net.ParseCIDR("0.0.0.0/0")
	dstIP := net.IPNet{IP: net.ParseIP("0.0.0.0"), Mask: defaultIPNet.Mask}
	gwIP := net.ParseIP(gwIPString)
	epInfo.Routes = append(epInfo.Routes, network.RouteInfo{Dst: dstIP, Gw: gwIP, DevName: snatInterface})
	result.Routes = append(result.Routes, &cniTypes.Route{Dst: dstIP, GW: gwIP})
}

func addSnatForDNS(gwIPString string, epInfo *network.EndpointInfo, result *cniTypesCurr.Result) {
	_, dnsIPNet, _ := net.ParseCIDR("168.63.129.16/32")
	gwIP := net.ParseIP(gwIPString)
	epInfo.Routes = append(epInfo.Routes, network.RouteInfo{Dst: *dnsIPNet, Gw: gwIP, DevName: snatInterface})
	result.Routes = append(result.Routes, &cniTypes.Route{Dst: *dnsIPNet, GW: gwIP})
}

func addInfraRoutes(azIpamResult *cniTypesCurr.Result, result *cniTypesCurr.Result, epInfo *network.EndpointInfo) {
	for _, route := range azIpamResult.Routes {
		epInfo.Routes = append(epInfo.Routes, network.RouteInfo{Dst: route.Dst, Gw: route.GW, DevName: infraInterface})
		result.Routes = append(result.Routes, &cniTypes.Route{Dst: route.Dst, GW: route.GW})
	}
}

func setNetworkOptions(cnsNwConfig *cns.GetNetworkContainerResponse, nwInfo *network.NetworkInfo) {
	if cnsNwConfig != nil && cnsNwConfig.MultiTenancyInfo.ID != 0 {
		logger.Info("Setting Network Options")
		vlanMap := make(map[string]interface{})
		vlanMap[network.VlanIDKey] = strconv.Itoa(cnsNwConfig.MultiTenancyInfo.ID)
		vlanMap[network.SnatBridgeIPKey] = cnsNwConfig.LocalIPConfiguration.GatewayIPAddress + "/" + strconv.Itoa(int(cnsNwConfig.LocalIPConfiguration.IPSubnet.PrefixLength))
		nwInfo.Options[dockerNetworkOption] = vlanMap
	}
}

func setEndpointOptions(cnsNwConfig *cns.GetNetworkContainerResponse, epInfo *network.EndpointInfo, vethName string) {
	if cnsNwConfig != nil && cnsNwConfig.MultiTenancyInfo.ID != 0 {
		logger.Info("Setting Endpoint Options")
		epInfo.Data[network.VlanIDKey] = cnsNwConfig.MultiTenancyInfo.ID
		epInfo.Data[network.LocalIPKey] = cnsNwConfig.LocalIPConfiguration.IPSubnet.IPAddress + "/" + strconv.Itoa(int(cnsNwConfig.LocalIPConfiguration.IPSubnet.PrefixLength))
		epInfo.Data[network.SnatBridgeIPKey] = cnsNwConfig.LocalIPConfiguration.GatewayIPAddress + "/" + strconv.Itoa(int(cnsNwConfig.LocalIPConfiguration.IPSubnet.PrefixLength))
		epInfo.AllowInboundFromHostToNC = cnsNwConfig.AllowHostToNCCommunication
		epInfo.AllowInboundFromNCToHost = cnsNwConfig.AllowNCToHostCommunication
		epInfo.NetworkContainerID = cnsNwConfig.NetworkContainerID
	}

	epInfo.Data[network.OptVethName] = vethName
}

func addSnatInterface(nwCfg *cni.NetworkConfig, result *cniTypesCurr.Result) {
	if nwCfg != nil && nwCfg.MultiTenancy {
		snatIface := &cniTypesCurr.Interface{
			Name: snatInterface,
		}

		result.Interfaces = append(result.Interfaces, snatIface)
	}
}

func setupInfraVnetRoutingForMultitenancy(
	nwCfg *cni.NetworkConfig,
	azIpamResult *cniTypesCurr.Result,
	epInfo *network.EndpointInfo,
	result *cniTypesCurr.Result,
) {
	if epInfo.EnableInfraVnet {
		_, ipNet, _ := net.ParseCIDR(nwCfg.InfraVnetAddressSpace)
		epInfo.Routes = append(epInfo.Routes, network.RouteInfo{Dst: *ipNet, Gw: azIpamResult.IPs[0].Gateway, DevName: infraInterface})
	}
}

func getNetworkDNSSettings(nwCfg *cni.NetworkConfig, result *cniTypesCurr.Result) (network.DNSInfo, error) {
	var nwDNS network.DNSInfo

	if len(nwCfg.DNS.Nameservers) > 0 {
		nwDNS = network.DNSInfo{
			Servers: nwCfg.DNS.Nameservers,
			Suffix:  nwCfg.DNS.Domain,
		}
	} else {
		nwDNS = network.DNSInfo{
			Suffix:  result.DNS.Domain,
			Servers: result.DNS.Nameservers,
		}
	}

	return nwDNS, nil
}

func getEndpointDNSSettings(nwCfg *cni.NetworkConfig, result *cniTypesCurr.Result, _ string) (network.DNSInfo, error) {
	return getNetworkDNSSettings(nwCfg, result)
}

func getEndpointPolicies(PolicyArgs) ([]policy.Policy, error) {
	return nil, nil
}

// getPoliciesFromRuntimeCfg returns network policies from network config.
// getPoliciesFromRuntimeCfg is a dummy function for Linux platform.
func getPoliciesFromRuntimeCfg(_ *cni.NetworkConfig, _ bool) []policy.Policy {
	return nil
}

func addIPV6EndpointPolicy(nwInfo network.NetworkInfo) (policy.Policy, error) {
	return policy.Policy{}, nil
}

func (plugin *NetPlugin) getNetworkName(_ string, _ *IPAMAddResult, nwCfg *cni.NetworkConfig) (string, error) {
	return nwCfg.Name, nil
}

func getNATInfo(_ *cni.NetworkConfig, _ interface{}, _ bool) (natInfo []policy.NATInfo) {
	return natInfo
}

func platformInit(cniConfig *cni.NetworkConfig) {}

// isDualNicFeatureSupported returns if the dual nic feature is supported. Currently it's only supported for windows hnsv2 path
func (plugin *NetPlugin) isDualNicFeatureSupported(netNs string) bool {
	return false
}

func getOverlayGateway(_ *net.IPNet) (net.IP, error) {
	return net.ParseIP("169.254.1.1"), nil
}
