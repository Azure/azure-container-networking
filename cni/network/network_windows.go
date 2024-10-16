package network

import (
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/util"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/Microsoft/hcsshim"
	hnsv2 "github.com/Microsoft/hcsshim/hcn"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/100"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/sys/windows/registry"
)

var (
	snatConfigFileName = filepath.FromSlash(os.Getenv("TEMP")) + "\\snatConfig"
	// windows build for version 1903
	win1903Version = 18362
	dualStackCount = 2
)

func addDefaultRoute(_ string, _ *network.EndpointInfo, _ *network.InterfaceInfo) {
}

func addSnatForDNS(_ string, _ *network.EndpointInfo, _ *network.InterfaceInfo) {
}

// updates options field
func setNetworkOptions(cnsNwConfig *cns.GetNetworkContainerResponse, nwInfo *network.EndpointInfo) {
	if cnsNwConfig != nil && cnsNwConfig.MultiTenancyInfo.ID != 0 {
		logger.Info("Setting Network Options")
		optionsMap := make(map[string]interface{})
		optionsMap[network.VlanIDKey] = strconv.Itoa(cnsNwConfig.MultiTenancyInfo.ID)
		logger.Info("Add vlanIDKey to optionsMap", zap.String("vlanIDKey", network.VlanIDKey))
		nwInfo.Options[dockerNetworkOption] = optionsMap
	}
}

func setEndpointOptions(cnsNwConfig *cns.GetNetworkContainerResponse, epInfo *network.EndpointInfo, _ string) {
	if cnsNwConfig != nil && cnsNwConfig.MultiTenancyInfo.ID != 0 {
		logger.Info("Setting Endpoint Options")
		var cnetAddressMap []string
		for _, ipSubnet := range cnsNwConfig.CnetAddressSpace {
			cnetAddressMap = append(cnetAddressMap, ipSubnet.IPAddress+"/"+strconv.Itoa(int(ipSubnet.PrefixLength)))
		}
		epInfo.Data[network.CnetAddressSpace] = cnetAddressMap
		epInfo.AllowInboundFromHostToNC = cnsNwConfig.AllowHostToNCCommunication
		epInfo.AllowInboundFromNCToHost = cnsNwConfig.AllowNCToHostCommunication
		epInfo.NetworkContainerID = cnsNwConfig.NetworkContainerID
	}
}

func addSnatInterface(nwCfg *cni.NetworkConfig, result *cniTypesCurr.Result) {
}

func (plugin *NetPlugin) getNetworkName(netNs string, interfaceInfo *network.InterfaceInfo, nwCfg *cni.NetworkConfig) (string, error) {
	var err error
	// Swiftv2 path => interfaceInfo.NICType = delegated NIC
	// For singletenancy => nwCfg.Name
	// Swiftv1 => interfaceInfo.NCResponse != nil && ipamAddResult != nil

	determineWinVer()
	// Swiftv2 L1VH Network Name
	swiftv2NetworkNamePrefix := "azure-"
	if interfaceInfo != nil && (interfaceInfo.NICType == cns.NodeNetworkInterfaceFrontendNIC || interfaceInfo.NICType == cns.BackendNIC) {
		logger.Info("swiftv2", zap.String("network name", interfaceInfo.MacAddress.String()))
		return swiftv2NetworkNamePrefix + interfaceInfo.MacAddress.String(), nil
	}

	// For singletenancy, the network name is simply the nwCfg.Name
	if !nwCfg.MultiTenancy {
		return nwCfg.Name, nil
	}

	// in multitenancy case, the network name will be in the state file or can be built from cnsResponse
	if len(strings.TrimSpace(netNs)) == 0 {
		return "", fmt.Errorf("NetNs cannot be empty")
	}

	// First try to build the network name from the cnsResponse if present
	// This will happen during ADD call
	// ifIndex, err := findDefaultInterface(*ipamAddResult)
	if interfaceInfo != nil && interfaceInfo.NCResponse != nil { // swiftv1 path
		if err != nil {
			logger.Error("Error finding InfraNIC interface",
				zap.Error(err))
			return "", errors.Wrap(err, "cns did not return an InfraNIC")
		}
		// networkName will look like ~ azure-vlan1-172-28-1-0_24
		ipAddrNet := interfaceInfo.IPConfigs[0].Address
		prefix, err := netip.ParsePrefix(ipAddrNet.String())
		if err != nil {
			logger.Error("Error parsing network CIDR",
				zap.String("cidr", ipAddrNet.String()),
				zap.Error(err))
			return "", errors.Wrapf(err, "cns returned invalid CIDR %s", ipAddrNet.String())
		}
		networkName := strings.ReplaceAll(prefix.Masked().String(), ".", "-")
		networkName = strings.ReplaceAll(networkName, "/", "_")
		networkName = fmt.Sprintf("%s-vlan%v-%v", nwCfg.Name, interfaceInfo.NCResponse.MultiTenancyInfo.ID, networkName)
		return networkName, nil
	}

	// If no cnsResponse was present, try to get the network name from the state file
	// This will happen during DEL call
	networkName, err := plugin.nm.FindNetworkIDFromNetNs(netNs)
	if err != nil {
		logger.Error("No endpoint available",
			zap.String("netns", netNs),
			zap.Error(err))
		return "", fmt.Errorf("No endpoint available with netNs: %s: %w", netNs, err)
	}

	return networkName, nil
}

func setupInfraVnetRoutingForMultitenancy(
	_ *cni.NetworkConfig,
	_ *cniTypesCurr.Result,
	_ *network.EndpointInfo) {
}

func getNetworkDNSSettings(nwCfg *cni.NetworkConfig, _ network.DNSInfo) (network.DNSInfo, error) {
	var nwDNS network.DNSInfo

	// use custom dns if present
	nwDNS = getCustomDNS(nwCfg)
	if len(nwDNS.Servers) > 0 || nwDNS.Suffix != "" {
		return nwDNS, nil
	}

	if (len(nwCfg.DNS.Search) == 0) != (len(nwCfg.DNS.Nameservers) == 0) {
		err := fmt.Errorf("Wrong DNS configuration: %+v", nwCfg.DNS)
		return nwDNS, err
	}

	nwDNS = network.DNSInfo{
		Servers: nwCfg.DNS.Nameservers,
	}

	return nwDNS, nil
}

func getEndpointDNSSettings(nwCfg *cni.NetworkConfig, dns network.DNSInfo, namespace string) (network.DNSInfo, error) {
	var epDNS network.DNSInfo

	// use custom dns if present
	epDNS = getCustomDNS(nwCfg)
	if len(epDNS.Servers) > 0 || epDNS.Suffix != "" {
		return epDNS, nil
	}

	if (len(nwCfg.DNS.Search) == 0) != (len(nwCfg.DNS.Nameservers) == 0) {
		err := fmt.Errorf("Wrong DNS configuration: %+v", nwCfg.DNS)
		return epDNS, err
	}

	if len(nwCfg.DNS.Search) > 0 {
		epDNS = network.DNSInfo{
			Servers: nwCfg.DNS.Nameservers,
			Suffix:  namespace + "." + strings.Join(nwCfg.DNS.Search, ","),
			Options: nwCfg.DNS.Options,
		}
	} else {
		epDNS = dns
		epDNS.Options = nwCfg.DNS.Options
	}

	return epDNS, nil
}

/*
getPoliciesFromRuntimeCfg returns network policies from network config.

Windows
test-netconnection to --->    to node ipv4    to node ipv6    to localhost ipv4    to localhost ipv6
host port mapping w/
no host ip                     ok             ok              fail                 fail
localhost ipv4 host ip         fail           fail            fail                 fail
node ipv6 host ip              fail           ok              fail                 fail
localhost ipv6 host ip         fail           fail            fail                 fail
node ipv4 host ip              ok             fail            fail                 fail
*/
func getPoliciesFromRuntimeCfg(nwCfg *cni.NetworkConfig, isIPv6Enabled bool) ([]policy.Policy, error) {
	logger.Info("Runtime Info", zap.Any("config", nwCfg.RuntimeConfig))
	var policies []policy.Policy
	var protocol uint32

	for _, mapping := range nwCfg.RuntimeConfig.PortMappings {

		cfgProto := strings.ToUpper(strings.TrimSpace(mapping.Protocol))
		switch cfgProto {
		case "TCP":
			protocol = policy.ProtocolTcp
		case "UDP":
			protocol = policy.ProtocolUdp
		}

		// To support hostport policy mapping
		// uint32 NatFlagsLocalRoutedVip = 1
		// To support hostport policy mapping for ipv6 in dualstack overlay mode
		// uint32 NatFlagsIPv6 = 2

		// if host ip is specified, we create a policy to match that ip only (ipv4 or ipv6), or ipv4 if no host ip
		flag := hnsv2.NatFlagsLocalRoutedVip // ipv4 flag
		if mapping.HostIp != "" {
			hostIP, err := netip.ParseAddr(mapping.HostIp)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse hostIP %v", hostIP)
			}

			if hostIP.Is6() && isIPv6Enabled {
				flag = hnsv2.NatFlagsIPv6
			}

			if hostIP.Is6() && !isIPv6Enabled {
				logger.Info("Do not use ipv6 hostIP to create windows pod on ipv4 cluster")
			}
		}

		hnsPortMappingPolicy, err := createPortMappingPolicy(mapping.HostPort, mapping.ContainerPort, mapping.HostIp, protocol, flag)
		if err != nil {
			return nil, err
		}

		logger.Info("Creating port mapping policy", zap.Any("policy", hnsPortMappingPolicy))
		policies = append(policies, *hnsPortMappingPolicy)

		// if no host ip specified and ipv6 enabled, we also create an identical ipv6 policy in addition to the previous ipv4 policy
		if mapping.HostIp == "" && isIPv6Enabled {
			ipv6HnsPortMappingPolicy, err := createPortMappingPolicy(mapping.HostPort, mapping.ContainerPort, mapping.HostIp, protocol, hnsv2.NatFlagsIPv6)
			if err != nil {
				return nil, err
			}
			logger.Info("Creating ipv6 port mapping policy", zap.Any("policy", ipv6HnsPortMappingPolicy))
			policies = append(policies, *ipv6HnsPortMappingPolicy)
		}
	}

	return policies, nil
}

func createPortMappingPolicy(hostPort, containerPort int, hostIP string, protocol uint32, flags hnsv2.NatFlags) (*policy.Policy, error) {
	rawPolicy, err := json.Marshal(&hnsv2.PortMappingPolicySetting{
		ExternalPort: uint16(hostPort),
		InternalPort: uint16(containerPort),
		VIP:          hostIP,
		Protocol:     protocol,
		Flags:        flags,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal HNS portMappingPolicySetting")
	}

	hnsv2Policy, err := json.Marshal(&hnsv2.EndpointPolicy{
		Type:     hnsv2.PortMapping,
		Settings: rawPolicy,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal HNS endpointPolicy")
	}

	hnsPolicy := policy.Policy{
		Type: policy.EndpointPolicy,
		Data: hnsv2Policy,
	}

	return &hnsPolicy, nil
}

func getEndpointPolicies(args PolicyArgs) ([]policy.Policy, error) {
	var policies []policy.Policy

	if args.nwCfg.IPV6Mode == network.IPV6Nat {
		ipv6Policy, err := getIPV6EndpointPolicy(args.subnetInfos)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get ipv6 endpoint policy")
		}
		policies = append(policies, ipv6Policy)
	}

	if args.nwCfg.WindowsSettings.EnableLoopbackDSR {
		dsrPolicies, err := getLoopbackDSRPolicy(args)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get loopback dsr policy")
		}
		policies = append(policies, dsrPolicies...)
	}

	return policies, nil
}

func getLoopbackDSRPolicy(args PolicyArgs) ([]policy.Policy, error) {
	var policies []policy.Policy
	for _, config := range args.ipconfigs {
		// consider DSR policy only for ipv4 address. Add for ipv6 when required
		if config.Address.IP.To4() != nil {
			dsrData := policy.LoopbackDSR{
				Type:      policy.LoopbackDSRPolicy,
				IPAddress: config.Address.IP,
			}

			dsrDataBytes, err := json.Marshal(dsrData)
			if err != nil {
				return nil, errors.Wrap(err, "failed to marshal dsr data")
			}
			dsrPolicy := policy.Policy{
				Type: policy.EndpointPolicy,
				Data: dsrDataBytes,
			}
			policies = append(policies, dsrPolicy)
		}
	}

	return policies, nil
}

func getIPV6EndpointPolicy(subnetInfos []network.SubnetInfo) (policy.Policy, error) {
	var eppolicy policy.Policy

	if len(subnetInfos) < dualStackCount {
		return eppolicy, fmt.Errorf("network state doesn't have ipv6 subnet")
	}

	// Everything should be snat'd except podcidr
	exceptionList := []string{subnetInfos[1].Prefix.String()}
	rawPolicy, _ := json.Marshal(&hcsshim.OutboundNatPolicy{
		Policy:     hcsshim.Policy{Type: hcsshim.OutboundNat},
		Exceptions: exceptionList,
	})

	eppolicy = policy.Policy{
		Type: policy.EndpointPolicy,
		Data: rawPolicy,
	}

	logger.Info("ipv6 outboundnat policy", zap.Any("policy", eppolicy))
	return eppolicy, nil
}

func getCustomDNS(nwCfg *cni.NetworkConfig) network.DNSInfo {
	var search string
	if len(nwCfg.RuntimeConfig.DNS.Searches) > 0 {
		search = strings.Join(nwCfg.RuntimeConfig.DNS.Searches, ",")
	}

	return network.DNSInfo{
		Servers: nwCfg.RuntimeConfig.DNS.Servers,
		Suffix:  search,
		Options: nwCfg.RuntimeConfig.DNS.Options,
	}
}

func determineWinVer() {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err == nil {
		defer k.Close()

		cb, _, err := k.GetStringValue("CurrentBuild")
		if err == nil {
			winVer, err := strconv.Atoi(cb)
			if err == nil {
				policy.ValidWinVerForDnsNat = winVer >= win1903Version
			}
		}
	}

	if err != nil {
		logger.Error(err.Error())
	}
}

func getNATInfo(nwCfg *cni.NetworkConfig, ncPrimaryIPIface interface{}, enableSnatForDNS bool) (natInfo []policy.NATInfo) {
	// TODO: Remove v4overlay and dualstackoverlay options, after 'overlay' rolls out in AKS-RP
	if nwCfg.ExecutionMode == string(util.V4Swift) && nwCfg.IPAM.Mode != string(util.V4Overlay) && nwCfg.IPAM.Mode != string(util.DualStackOverlay) && nwCfg.IPAM.Mode != string(util.Overlay) {
		ncPrimaryIP := ""
		if ncPrimaryIPIface != nil {
			ncPrimaryIP = ncPrimaryIPIface.(string)
		}

		natInfo = append(natInfo, []policy.NATInfo{{VirtualIP: ncPrimaryIP, Destinations: []string{networkutils.AzureDNS}}, {Destinations: []string{networkutils.AzureIMDS}}}...)
	} else if nwCfg.MultiTenancy && enableSnatForDNS {
		natInfo = append(natInfo, policy.NATInfo{Destinations: []string{networkutils.AzureDNS}})
	}

	return natInfo
}

func platformInit(cniConfig *cni.NetworkConfig) {
	if cniConfig.WindowsSettings.HnsTimeoutDurationInSeconds > 0 {
		logger.Info("Enabling timeout for Hns calls",
			zap.Int("timeout", cniConfig.WindowsSettings.HnsTimeoutDurationInSeconds))
		network.EnableHnsV1Timeout(cniConfig.WindowsSettings.HnsTimeoutDurationInSeconds)
		network.EnableHnsV2Timeout(cniConfig.WindowsSettings.HnsTimeoutDurationInSeconds)
	}
}

// isDualNicFeatureSupported returns if the dual nic feature is supported. Currently it's only supported for windows hnsv2 path
func (plugin *NetPlugin) isDualNicFeatureSupported(netNs string) bool {
	useHnsV2, err := network.UseHnsV2(netNs)
	if useHnsV2 && err == nil {
		return true
	}
	logger.Error("DualNicFeature is not supported")
	return false
}

func getOverlayGateway(podsubnet *net.IPNet) (net.IP, error) {
	logger.Warn("No gateway specified for Overlay NC. CNI will choose one, but connectivity may break")
	ncgw := podsubnet.IP
	ncgw[3]++
	ncgw = net.ParseIP(ncgw.String())
	if ncgw == nil || !podsubnet.Contains(ncgw) {
		return nil, errors.Wrap(errInvalidArgs, "%w: Failed to retrieve overlay gateway from podsubnet"+podsubnet.IP.String())
	}

	return ncgw, nil
}
