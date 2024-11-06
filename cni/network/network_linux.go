package network

import (
	"net"
	"strconv"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/network/policy"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/100"
	"go.uber.org/zap"
)

const (
	snatInterface  = "eth1"
	infraInterface = "eth2"
)

const snatConfigFileName = "/tmp/snatConfig"

func addDefaultRoute(gwIPString string, epInfo *network.EndpointInfo, result *network.InterfaceInfo) {
	_, defaultIPNet, _ := net.ParseCIDR("0.0.0.0/0")
	dstIP := net.IPNet{IP: net.ParseIP("0.0.0.0"), Mask: defaultIPNet.Mask}
	gwIP := net.ParseIP(gwIPString)
	epInfo.Routes = append(epInfo.Routes, network.RouteInfo{Dst: dstIP, Gw: gwIP, DevName: snatInterface})
	result.Routes = append(result.Routes, network.RouteInfo{Dst: dstIP, Gw: gwIP})
}

func addSnatForDNS(gwIPString string, epInfo *network.EndpointInfo, result *network.InterfaceInfo) {
	_, dnsIPNet, _ := net.ParseCIDR("168.63.129.16/32")
	gwIP := net.ParseIP(gwIPString)
	epInfo.Routes = append(epInfo.Routes, network.RouteInfo{Dst: *dnsIPNet, Gw: gwIP, DevName: snatInterface})
	result.Routes = append(result.Routes, network.RouteInfo{Dst: *dnsIPNet, Gw: gwIP})
}

// updates options field
func setNetworkOptions(cnsNwConfig *cns.GetNetworkContainerResponse, nwInfo *network.EndpointInfo) {
	if cnsNwConfig != nil && cnsNwConfig.MultiTenancyInfo.ID != 0 {
		logger.Info("Setting Network Options")
		optionsMap := make(map[string]interface{})
		optionsMap[network.VlanIDKey] = strconv.Itoa(cnsNwConfig.MultiTenancyInfo.ID)
		optionsMap[network.SnatBridgeIPKey] = cnsNwConfig.LocalIPConfiguration.GatewayIPAddress + "/" + strconv.Itoa(int(cnsNwConfig.LocalIPConfiguration.IPSubnet.PrefixLength))
		logger.Info("Add vlanIDkey and SnatBridgeIPKey to optionsMap", zap.String("vlanIDKey", network.VlanIDKey), zap.String("SnatBridgeIPKey", network.SnatBridgeIPKey))
		nwInfo.Options[dockerNetworkOption] = optionsMap
	}
}

// update epInfo data field, allow host to nc, allow nc to host, and network container id
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
) {
	if epInfo.EnableInfraVnet {
		_, ipNet, _ := net.ParseCIDR(nwCfg.InfraVnetAddressSpace)
		epInfo.Routes = append(epInfo.Routes, network.RouteInfo{Dst: *ipNet, Gw: azIpamResult.IPs[0].Gateway, DevName: infraInterface})
	}
}

func getNetworkDNSSettings(nwCfg *cni.NetworkConfig, dns network.DNSInfo) (network.DNSInfo, error) {
	var nwDNS network.DNSInfo

	if len(nwCfg.DNS.Nameservers) > 0 {
		nwDNS = network.DNSInfo{
			Servers: nwCfg.DNS.Nameservers,
			Suffix:  nwCfg.DNS.Domain,
		}
	} else {
		nwDNS = dns
	}

	return nwDNS, nil
}

func getEndpointDNSSettings(nwCfg *cni.NetworkConfig, dns network.DNSInfo, _ string) (network.DNSInfo, error) {
	return getNetworkDNSSettings(nwCfg, dns)
}

func getEndpointPolicies(PolicyArgs) ([]policy.Policy, error) {
	return nil, nil
}

// getPoliciesFromRuntimeCfg returns network policies from network config.
// getPoliciesFromRuntimeCfg is a dummy function for Linux platform.
func getPoliciesFromRuntimeCfg(_ *cni.NetworkConfig, _ bool) ([]policy.Policy, error) {
	return nil, nil
}

func addIPV6EndpointPolicy(nwInfo network.NetworkInfo) (policy.Policy, error) {
	return policy.Policy{}, nil
}

func (plugin *NetPlugin) getNetworkName(_ string, _ *network.InterfaceInfo, nwCfg *cni.NetworkConfig) (string, error) {
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
