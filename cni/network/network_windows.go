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
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/100"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/sys/windows/registry"
)

var (
	snatConfigFileName = filepath.FromSlash(os.Getenv("TEMP")) + "\\snatConfig"
	// windows build for version 1903
	win1903Version = 18362
)

/* handleConsecutiveAdd handles consecutive add calls for infrastructure containers on Windows platform.
 * This is a temporary work around for issue #57253 of Kubernetes.
 * We can delete this if statement once they fix it.
 * Issue link: https://github.com/kubernetes/kubernetes/issues/57253
 */
func (plugin *NetPlugin) handleConsecutiveAdd(args *cniSkel.CmdArgs, endpointId string, networkId string,
	nwInfo *network.NetworkInfo, nwCfg *cni.NetworkConfig,
) (*cniTypesCurr.Result, error) {
	epInfo, _ := plugin.nm.GetEndpointInfo(networkId, endpointId)
	if epInfo == nil {
		return nil, nil
	}

	// Return in case of HNSv2 as consecutive add call doesn't need to be handled
	if useHnsV2, err := network.UseHnsV2(args.Netns); useHnsV2 {
		return nil, err
	}

	hnsEndpoint, err := network.Hnsv1.GetHNSEndpointByName(endpointId)
	if hnsEndpoint != nil {
		logger.Info("Found existing endpoint through hcsshim",
			zap.Any("endpoint", hnsEndpoint))
		endpoint, _ := network.Hnsv1.GetHNSEndpointByID(hnsEndpoint.Id)
		isAttached, _ := network.Hnsv1.IsAttached(endpoint, args.ContainerID)
		// Attach endpoint if it's not attached yet.
		if !isAttached {
			logger.Info("Attaching endpoint to container",
				zap.String("endpoint", hnsEndpoint.Id),
				zap.String("container", args.ContainerID))
			err := network.Hnsv1.HotAttachEndpoint(args.ContainerID, hnsEndpoint.Id)
			if err != nil {
				logger.Error("Failed to hot attach shared endpoint to container",
					zap.String("endpoint", hnsEndpoint.Id),
					zap.String("container", args.ContainerID),
					zap.Error(err))
				return nil, err
			}
		}

		// Populate result.
		address := nwInfo.Subnets[0].Prefix
		address.IP = hnsEndpoint.IPAddress
		result := &cniTypesCurr.Result{
			IPs: []*cniTypesCurr.IPConfig{
				{
					Address: address,
					Gateway: net.ParseIP(hnsEndpoint.GatewayAddress),
				},
			},
			Routes: []*cniTypes.Route{
				{
					Dst: net.IPNet{net.IPv4zero, net.IPv4Mask(0, 0, 0, 0)},
					GW:  net.ParseIP(hnsEndpoint.GatewayAddress),
				},
			},
		}

		if nwCfg.IPV6Mode != "" && len(epInfo.IPAddresses) > 1 {
			ipv6Config := &cniTypesCurr.IPConfig{
				Address: epInfo.IPAddresses[1],
			}

			if len(nwInfo.Subnets) > 1 {
				ipv6Config.Gateway = nwInfo.Subnets[1].Gateway
			}

			result.IPs = append(result.IPs, ipv6Config)
		}

		// Populate DNS servers.
		result.DNS.Nameservers = nwCfg.DNS.Nameservers
		return result, nil
	}

	err = fmt.Errorf("GetHNSEndpointByName for %v returned nil with err %v", endpointId, err)
	return nil, err
}

func addDefaultRoute(gwIPString string, epInfo *network.EndpointInfo, result *cniTypesCurr.Result) {
}

func addSnatForDNS(gwIPString string, epInfo *network.EndpointInfo, result *cniTypesCurr.Result) {
}

func addInfraRoutes(azIpamResult *cniTypesCurr.Result, result *cniTypesCurr.Result, epInfo *network.EndpointInfo) {
}

func setNetworkOptions(cnsNwConfig *cns.GetNetworkContainerResponse, nwInfo *network.NetworkInfo) {
	if cnsNwConfig != nil && cnsNwConfig.MultiTenancyInfo.ID != 0 {
		logger.Info("Setting Network Options")
		vlanMap := make(map[string]interface{})
		vlanMap[network.VlanIDKey] = strconv.Itoa(cnsNwConfig.MultiTenancyInfo.ID)
		nwInfo.Options[dockerNetworkOption] = vlanMap
	}
}

func setEndpointOptions(cnsNwConfig *cns.GetNetworkContainerResponse, epInfo *network.EndpointInfo, vethName string) {
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

func (plugin *NetPlugin) getNetworkName(netNs string, ipamAddResult *IPAMAddResult, nwCfg *cni.NetworkConfig) (string, error) {
	determineWinVer()
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
	if ipamAddResult != nil && ipamAddResult.ncResponse != nil {
		// networkName will look like ~ azure-vlan1-172-28-1-0_24
		ipAddrNet := ipamAddResult.ipv4Result.IPs[0].Address
		prefix, err := netip.ParsePrefix(ipAddrNet.String())
		if err != nil {
			logger.Error("Error parsing network CIDR",
				zap.String("cidr", ipAddrNet.String()),
				zap.Error(err))
			return "", errors.Wrapf(err, "cns returned invalid CIDR %s", ipAddrNet.String())
		}
		networkName := strings.ReplaceAll(prefix.Masked().String(), ".", "-")
		networkName = strings.ReplaceAll(networkName, "/", "_")
		networkName = fmt.Sprintf("%s-vlan%v-%v", nwCfg.Name, ipamAddResult.ncResponse.MultiTenancyInfo.ID, networkName)
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
	_ *network.EndpointInfo,
	_ *cniTypesCurr.Result) {
}

func getNetworkDNSSettings(nwCfg *cni.NetworkConfig, _ *cniTypesCurr.Result) (network.DNSInfo, error) {
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

func getEndpointDNSSettings(nwCfg *cni.NetworkConfig, result *cniTypesCurr.Result, namespace string) (network.DNSInfo, error) {
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
		epDNS = network.DNSInfo{
			Servers: result.DNS.Nameservers,
			Suffix:  result.DNS.Domain,
			Options: nwCfg.DNS.Options,
		}
	}

	return epDNS, nil
}

// getPoliciesFromRuntimeCfg returns network policies from network config.
func getPoliciesFromRuntimeCfg(nwCfg *cni.NetworkConfig, isIPv6Enabled bool) []policy.Policy {
	logger.Info("Runtime Info",
		zap.Any("config", nwCfg.RuntimeConfig))
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
		rawPolicy, _ := json.Marshal(&hnsv2.PortMappingPolicySetting{
			ExternalPort: uint16(mapping.HostPort),
			InternalPort: uint16(mapping.ContainerPort),
			VIP:          mapping.HostIp,
			Protocol:     protocol,
			Flags:        hnsv2.NatFlagsLocalRoutedVip,
		})

		hnsv2Policy, _ := json.Marshal(&hnsv2.EndpointPolicy{
			Type:     hnsv2.PortMapping,
			Settings: rawPolicy,
		})

		policyv4 := policy.Policy{
			Type: policy.EndpointPolicy,
			Data: hnsv2Policy,
		}

		logger.Info("Creating port mapping policyv4",
			zap.Any("policy", policyv4))
		policies = append(policies, policyv4)

		// add port mapping policy for v6 if we have IPV6 enabled
		if isIPv6Enabled {
			// To support hostport policy mapping for ipv6 in dualstack overlay mode
			// uint32 NatFlagsIPv6 = 2
			rawPolicyv6, _ := json.Marshal(&hnsv2.PortMappingPolicySetting{ // nolint
				ExternalPort: uint16(mapping.HostPort),
				InternalPort: uint16(mapping.ContainerPort),
				VIP:          mapping.HostIp,
				Protocol:     protocol,
				Flags:        hnsv2.NatFlagsIPv6,
			})

			hnsv2Policyv6, _ := json.Marshal(&hnsv2.EndpointPolicy{ // nolint
				Type:     hnsv2.PortMapping,
				Settings: rawPolicyv6,
			})

			policyv6 := policy.Policy{
				Type: policy.EndpointPolicy,
				Data: hnsv2Policyv6,
			}

			logger.Info("Creating port mapping policyv6",
				zap.Any("policy", policyv6))
			policies = append(policies, policyv6)
		}
	}

	return policies
}

func getEndpointPolicies(args PolicyArgs) ([]policy.Policy, error) {
	var policies []policy.Policy

	if args.nwCfg.IPV6Mode == network.IPV6Nat {
		ipv6Policy, err := getIPV6EndpointPolicy(args.nwInfo)
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

func getIPV6EndpointPolicy(nwInfo *network.NetworkInfo) (policy.Policy, error) {
	var eppolicy policy.Policy

	if len(nwInfo.Subnets) < 2 {
		return eppolicy, fmt.Errorf("network state doesn't have ipv6 subnet")
	}

	// Everything should be snat'd except podcidr
	exceptionList := []string{nwInfo.Subnets[1].Prefix.String()}
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
