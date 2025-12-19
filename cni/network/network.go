// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"time"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/api"
	"github.com/Azure/azure-container-networking/cni/log"
	"github.com/Azure/azure-container-networking/cni/util"
	"github.com/Azure/azure-container-networking/cns"
	cnscli "github.com/Azure/azure-container-networking/cns/client"
	"github.com/Azure/azure-container-networking/cns/fsnotify"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/dhcp"
	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/Azure/azure-container-networking/platform"
	nnscontracts "github.com/Azure/azure-container-networking/proto/nodenetworkservice/3.302.0.744"
	"github.com/Azure/azure-container-networking/store"
	"github.com/Azure/azure-container-networking/telemetry"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/100"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// matches if the string fully consists of zero or more alphanumeric, dots, dashes, parentheses, spaces, or underscores
var (
	allowedInput    = regexp.MustCompile(`^[a-zA-Z0-9._\-\(\) ]*$`)
	telemetryClient = telemetry.AIClient
)

const (
	dockerNetworkOption = "com.docker.network.generic"
	OpModeTransparent   = "transparent"
	// Supported IP version. Currently support only IPv4
	ipamV6                = "azure-vnet-ipamv6"
	defaultRequestTimeout = 15 * time.Second
	ipv4FullMask          = 32
	ipv6FullMask          = 128
	ibInterfacePrefix     = "ib"
	apipaInterfacePrefix  = "apipa"
)

// CNI Operation Types
const (
	CNI_ADD    = "ADD"
	CNI_DEL    = "DEL"
	CNI_UPDATE = "UPDATE"
)

const (
	// URL to query NMAgent version and determine whether we snat on host
	nmAgentSupportedApisURL = "http://168.63.129.16/machine/plugins/?comp=nmagent&type=GetSupportedApis"
	// Only SNAT support (no DNS support)
	nmAgentSnatSupportAPI = "NetworkManagementSnatSupport"
	// SNAT and DNS are both supported
	nmAgentSnatAndDnsSupportAPI = "NetworkManagementDNSSupport"
)

// temporary consts related func determineSnat() which is to be deleted after
// a baking period with newest NMAgent changes
const (
	jsonFileExtension = ".json"
)

// NetPlugin represents the CNI network plugin.
type NetPlugin struct {
	*cni.Plugin
	nm                 network.NetworkManager
	ipamInvoker        IPAMInvoker
	nnsClient          NnsClient
	multitenancyClient MultitenancyClient
	netClient          InterfaceGetter
}

type PolicyArgs struct {
	subnetInfos []network.SubnetInfo
	nwCfg       *cni.NetworkConfig
	ipconfigs   []*network.IPConfig
}

// client for node network service
type NnsClient interface {
	// Do network port programming for the pod via node network service.
	// podName - name of the pod as received from containerD
	// nwNamesapce - network namespace name as received from containerD
	AddContainerNetworking(ctx context.Context, podName, nwNamespace string) (*nnscontracts.ConfigureContainerNetworkingResponse, error)

	// Undo or delete network port programming for the pod via node network service.
	// podName - name of the pod as received from containerD
	// nwNamesapce - network namespace name as received from containerD
	DeleteContainerNetworking(ctx context.Context, podName, nwNamespace string) (*nnscontracts.ConfigureContainerNetworkingResponse, error)
}

// client for getting interface
type InterfaceGetter interface {
	GetNetworkInterfaces() ([]net.Interface, error)
	GetNetworkInterfaceAddrs(iface *net.Interface) ([]net.Addr, error)
}

// snatConfiguration contains a bool that determines whether CNI enables snat on host and snat for dns
type snatConfiguration struct {
	EnableSnatOnHost bool
	EnableSnatForDns bool
}

// NewPlugin creates a new NetPlugin object.
func NewPlugin(name string,
	config *common.PluginConfig,
	client NnsClient,
	multitenancyClient MultitenancyClient,
) (*NetPlugin, error) {
	// Setup base plugin.
	plugin, err := cni.NewPlugin(name, config.Version)
	if err != nil {
		return nil, err
	}

	nl := netlink.NewNetlink()
	// Setup network manager.
	nm, err := network.NewNetworkManager(nl, platform.NewExecClient(logger), &netio.NetIO{}, network.NewNamespaceClient(), iptables.NewClient(), dhcp.New(logger))
	if err != nil {
		return nil, err
	}

	config.NetApi = nm

	return &NetPlugin{
		Plugin:             plugin,
		nm:                 nm,
		nnsClient:          client,
		multitenancyClient: multitenancyClient,
		netClient:          &netio.NetIO{},
	}, nil
}

// Starts the plugin.
func (plugin *NetPlugin) Start(config *common.PluginConfig) error {
	// Initialize base plugin.
	err := plugin.Initialize(config)
	if err != nil {
		logger.Error("Failed to initialize base plugin", zap.Error(err))
		return err
	}

	// Log platform information.
	logger.Info("Plugin Info",
		zap.String("name", plugin.Name),
		zap.String("version", plugin.Version))

	// Initialize network manager. rehyrdration not required on reboot for cni plugin
	err = plugin.nm.Initialize(config, false)
	if err != nil {
		logger.Error("Failed to initialize network manager", zap.Error(err))
		return err
	}

	logger.Info("Plugin started")

	return nil
}

func (plugin *NetPlugin) GetAllEndpointState(networkid string) (*api.AzureCNIState, error) {
	st := api.AzureCNIState{
		ContainerInterfaces: make(map[string]api.PodNetworkInterfaceInfo),
	}

	eps, err := plugin.nm.GetAllEndpoints(networkid)
	if err == store.ErrStoreEmpty {
		logger.Error("failed to retrieve endpoint state", zap.Error(err))
	} else if err != nil {
		return nil, err
	}

	for _, ep := range eps {
		id := ep.EndpointID
		info := api.PodNetworkInterfaceInfo{
			PodName:       ep.PODName,
			PodNamespace:  ep.PODNameSpace,
			PodEndpointId: ep.EndpointID,
			ContainerID:   ep.ContainerID,
			IPAddresses:   ep.IPAddresses,
		}

		st.ContainerInterfaces[id] = info
	}

	return &st, nil
}

// Stops the plugin.
func (plugin *NetPlugin) Stop() {
	plugin.nm.Uninitialize()
	plugin.Uninitialize()
	logger.Info("Plugin stopped")
}

// findInterfaceByMAC returns the name of the master interface
func (plugin *NetPlugin) findInterfaceByMAC(macAddress string) string {
	interfaces, err := plugin.netClient.GetNetworkInterfaces()
	if err != nil {
		logger.Error("failed to get interfaces", zap.Error(err))
		return ""
	}
	macs := make([]string, 0, len(interfaces))
	for _, iface := range interfaces {
		// find master interface by macAddress for Swiftv2
		macs = append(macs, iface.HardwareAddr.String())
		if iface.HardwareAddr.String() == macAddress {
			return iface.Name
		}
	}
	// Failed to find a suitable interface.
	logger.Error("Failed to find interface by MAC", zap.String("macAddress", macAddress), zap.Strings("interfaces", macs))
	return ""
}

// findMasterInterfaceBySubnet returns the name of the master interface.
func (plugin *NetPlugin) findMasterInterfaceBySubnet(nwCfg *cni.NetworkConfig, subnetPrefix *net.IPNet) string {
	// An explicit master configuration wins. Explicitly specifying a master is
	// useful if host has multiple interfaces with addresses in the same subnet.
	if nwCfg.Master != "" {
		return nwCfg.Master
	}

	// Otherwise, pick the first interface with an IP address in the given subnet.
	subnetPrefixString := subnetPrefix.String()
	interfaces, err := plugin.netClient.GetNetworkInterfaces()
	if err != nil {
		logger.Error("failed to get interfaces", zap.Error(err))
		return ""
	}
	var ipnets []string
	for _, iface := range interfaces {
		addrs, _ := plugin.netClient.GetNetworkInterfaceAddrs(&iface) //nolint
		for _, addr := range addrs {
			_, ipnet, err := net.ParseCIDR(addr.String())
			if err != nil {
				continue
			}
			ipnets = append(ipnets, ipnet.String())
			if subnetPrefixString == ipnet.String() {
				return iface.Name
			}
		}
	}

	// Failed to find a suitable interface.
	logger.Error("Failed to find interface by subnet prefix", zap.String("subnetPrefix", subnetPrefixString), zap.Strings("interfaces", ipnets))
	return ""
}

// GetEndpointID returns a unique endpoint ID based on the CNI args.
func GetEndpointID(args *cniSkel.CmdArgs) string {
	infraEpId, _ := network.ConstructEndpointID(args.ContainerID, args.Netns, args.IfName)
	return infraEpId
}

// getPodInfo returns POD info by parsing the CNI args.
func (plugin *NetPlugin) getPodInfo(args string) (name, ns string, err error) {
	podCfg, err := cni.ParseCniArgs(args)
	if err != nil {
		logger.Error("Error while parsing CNI Args", zap.Error(err))
		return "", "", err
	}

	k8sNamespace := string(podCfg.K8S_POD_NAMESPACE)
	if len(k8sNamespace) == 0 {
		errMsg := "Pod Namespace not specified in CNI Args"
		logger.Error(errMsg)
		return "", "", plugin.Errorf("%s", errMsg)
	}

	k8sPodName := string(podCfg.K8S_POD_NAME)
	if len(k8sPodName) == 0 {
		errMsg := "Pod Name not specified in CNI Args"
		logger.Error(errMsg)
		return "", "", plugin.Errorf("%s", errMsg)
	}

	return k8sPodName, k8sNamespace, nil
}

func (plugin *NetPlugin) setCNIReportDetails(containerID, opType, msg string) {
	telemetryClient.Settings().OperationType = opType
	telemetryClient.Settings().SubContext = containerID
	telemetryClient.Settings().EventMessage = msg
	telemetryClient.Settings().Version = plugin.Version
}

func addNatIPV6SubnetInfo(nwCfg *cni.NetworkConfig,
	resultV6 *cniTypesCurr.Result,
	nwInfo *network.NetworkInfo,
) {
	if nwCfg.IPV6Mode == network.IPV6Nat {
		ipv6Subnet := resultV6.IPs[0].Address
		ipv6Subnet.IP = ipv6Subnet.IP.Mask(ipv6Subnet.Mask)
		ipv6SubnetInfo := network.SubnetInfo{
			Family:  platform.AfINET6,
			Prefix:  ipv6Subnet,
			Gateway: resultV6.IPs[0].Gateway,
		}
		logger.Info("ipv6 subnet info",
			zap.Any("ipv6SubnetInfo", ipv6SubnetInfo))
		nwInfo.Subnets = append(nwInfo.Subnets, ipv6SubnetInfo)
	}
}

func (plugin *NetPlugin) addIpamInvoker(ipamAddConfig IPAMAddConfig) (IPAMAddResult, error) {
	ipamAddResult, err := plugin.ipamInvoker.Add(ipamAddConfig)
	if err != nil {
		return IPAMAddResult{}, errors.Wrap(err, "failed to add ipam invoker")
	}
	return ipamAddResult, nil
}

// get network
func (plugin *NetPlugin) getNetworkID(netNs string, interfaceInfo *network.InterfaceInfo, nwCfg *cni.NetworkConfig) (string, error) {
	networkID, err := plugin.getNetworkName(netNs, interfaceInfo, nwCfg)
	if err != nil {
		return "", err
	}
	return networkID, nil
}

// get network info for legacy
func (plugin *NetPlugin) getNetworkInfo(netNs string, interfaceInfo *network.InterfaceInfo, nwCfg *cni.NetworkConfig) network.EndpointInfo {
	networkID, _ := plugin.getNetworkID(netNs, interfaceInfo, nwCfg)
	nwInfo, _ := plugin.nm.GetNetworkInfo(networkID)

	return nwInfo
}

// CNI implementation
// https://github.com/containernetworking/cni/blob/master/SPEC.md

// Add handles CNI add commands.
func (plugin *NetPlugin) Add(args *cniSkel.CmdArgs) error {
	var (
		ipamAddResult    IPAMAddResult
		azIpamResult     *cniTypesCurr.Result
		enableInfraVnet  bool
		enableSnatForDNS bool
		k8sPodName       string
		epInfos          []*network.EndpointInfo
	)

	startTime := time.Now()
	logger.Info("Processing ADD command",
		zap.String("containerId", args.ContainerID),
		zap.String("netNS", args.Netns),
		zap.String("ifName", args.IfName),
		zap.Any("args", args.Args),
		zap.String("path", args.Path),
		zap.ByteString("stdinData", args.StdinData))

	// Parse network configuration from stdin.
	nwCfg, err := cni.ParseNetworkConfig(args.StdinData)
	if err != nil {
		err = plugin.Errorf("Failed to parse network configuration: %v.", err)
		return err
	}

	if argErr := plugin.validateArgs(args, nwCfg); argErr != nil {
		err = argErr
		return err
	}

	// Parse Pod arguments.
	k8sPodName, k8sNamespace, err := plugin.getPodInfo(args.Args)
	if err != nil {
		return err
	}
	telemetryClient.Settings().ContainerName = k8sPodName + ":" + k8sNamespace

	plugin.setCNIReportDetails(args.ContainerID, CNI_ADD, "")
	telemetryClient.SendEvent(fmt.Sprintf("[cni-net] Processing ADD command with args {ContainerID:%v Netns:%v IfName:%v Args:%v Path:%v StdinData:%s}.",
		args.ContainerID, args.Netns, args.IfName, args.Args, args.Path, args.StdinData))

	iptables.DisableIPTableLock = nwCfg.DisableIPTableLock

	defer func() {
		// Add Interfaces to result.
		// previously we had a default interface info to select which interface info was the one to be returned from cni add
		cniResult := &cniTypesCurr.Result{}
		for key := range ipamAddResult.interfaceInfo {
			// now we have to infer which interface info should be returned
			// we assume that we want to return the infra nic always, and if that is not found, return any one of the secondary interfaces
			// if there is an infra nic + secondary, we will always return the infra nic (linux swift v2)
			cniResult = convertInterfaceInfoToCniResult(ipamAddResult.interfaceInfo[key], args.IfName)
			if ipamAddResult.interfaceInfo[key].NICType == cns.InfraNIC {
				break
			}
		}

		// stdout multiple cniResults for containerd to create multiple pods
		// containerd receives each cniResult as the stdout and create pod
		addSnatInterface(nwCfg, cniResult) //nolint TODO: check whether Linux supports adding secondary snatinterface

		// add IB NIC interfaceInfo to cniResult
		for _, epInfo := range epInfos {
			if epInfo.NICType == cns.BackendNIC {
				cniResult.Interfaces = append(cniResult.Interfaces, &cniTypesCurr.Interface{
					Name:  epInfo.MasterIfName,
					Mac:   epInfo.MacAddress.String(),
					PciID: epInfo.PnPID,
				})
			}
		}

		// Convert result to the requested CNI version.
		res, vererr := cniResult.GetAsVersion(nwCfg.CNIVersion)
		if vererr != nil {
			logger.Error("GetAsVersion failed", zap.Error(vererr))
			plugin.Error(vererr) //nolint
		}

		if err == nil && res != nil {
			// Output the result to stdout.
			res.Print()
		}

		logger.Info("ADD command completed for",
			zap.String("pod", k8sPodName),
			zap.Any("IPs", cniResult.IPs),
			zap.Error(log.NewErrorWithoutStackTrace(err)))

		telemetryClient.SendEvent(fmt.Sprintf("ADD command completed with [ipamAddResult]: %s [epInfos]: %s [error]: %v ", ipamAddResult.PrettyString(), network.FormatSliceOfPointersToString(epInfos), err))

		operationTimeMs := time.Since(startTime).Milliseconds()
		telemetryClient.SendMetric(telemetry.CNIAddTimeMetricStr, float64(operationTimeMs), make(map[string]string))
	}()

	ipamAddResult = IPAMAddResult{interfaceInfo: make(map[string]network.InterfaceInfo)}

	k8sContainerID := args.ContainerID
	if len(k8sContainerID) == 0 {
		errMsg := "Container ID not specified in CNI Args"
		logger.Error(errMsg)
		return plugin.Errorf("%s", errMsg)
	}

	k8sIfName := args.IfName
	if len(k8sIfName) == 0 {
		errMsg := "Interfacename not specified in CNI Args"
		logger.Error(errMsg)
		return plugin.Errorf("%s", errMsg)
	}

	platformInit(nwCfg)
	if nwCfg.ExecutionMode == string(util.Baremetal) {
		var res *nnscontracts.ConfigureContainerNetworkingResponse
		logger.Info("Baremetal mode. Calling vnet agent for ADD")
		res, err = plugin.nnsClient.AddContainerNetworking(context.Background(), k8sPodName, args.Netns)

		if err == nil {
			ipamAddResult.interfaceInfo[string(cns.InfraNIC)] = network.InterfaceInfo{
				IPConfigs: convertNnsToIPConfigs(res, args.IfName, k8sPodName, "AddContainerNetworking"),
				NICType:   cns.InfraNIC,
			}
		}
		return err
	}

	for _, ns := range nwCfg.PodNamespaceForDualNetwork {
		if k8sNamespace == ns {
			logger.Info("Enable infravnet for pod",
				zap.String("pod", k8sPodName),
				zap.String("namespace", k8sNamespace))
			enableInfraVnet = true
			break
		}
	}

	cnsClient, err := cnscli.New(nwCfg.CNSUrl, defaultRequestTimeout)
	if err != nil {
		return fmt.Errorf("failed to create cns client with error: %w", err)
	}

	options := make(map[string]any)
	ipamAddConfig := IPAMAddConfig{nwCfg: nwCfg, args: args, options: options}

	if nwCfg.MultiTenancy {
		// triggered only in swift v1 multitenancy
		// dual nic multitenancy -> two interface infos
		// multitenancy (swift v1) -> one interface info
		telemetryClient.Settings().Context = "AzureCNIMultitenancy"
		plugin.multitenancyClient.Init(cnsClient, AzureNetIOShim{})

		// Temporary if block to determining whether we disable SNAT on host (for multi-tenant scenario only)
		if enableSnatForDNS, nwCfg.EnableSnatOnHost, err = plugin.multitenancyClient.DetermineSnatFeatureOnHost(
			snatConfigFileName, nmAgentSupportedApisURL); err != nil {
			return fmt.Errorf("%w", err)
		}

		ipamAddResult, err = plugin.multitenancyClient.GetAllNetworkContainers(context.TODO(), nwCfg, k8sPodName, k8sNamespace, args.IfName)
		if err != nil {
			err = fmt.Errorf("GetAllNetworkContainers failed for podname %s namespace %s. error: %w", k8sPodName, k8sNamespace, err)
			logger.Error("GetAllNetworkContainers failed",
				zap.String("pod", k8sPodName),
				zap.String("namespace", k8sNamespace),
				zap.Error(err))
			return err
		}
		// dual nic when we get multiple interface infos back (multitenancy does not necessarily have multiple if infos)
		if len(ipamAddResult.interfaceInfo) > 1 && !plugin.isDualNicFeatureSupported(args.Netns) {
			errMsg := fmt.Sprintf("received multiple NC results %+v from CNS while dualnic feature is not supported", ipamAddResult.interfaceInfo)
			logger.Error("received multiple NC results from CNS while dualnic feature is not supported",
				zap.Any("results", ipamAddResult.interfaceInfo))
			return plugin.Errorf("%s", errMsg)
		}
	} else {
		// when nwcfg.multitenancy (use multitenancy flag for swift v1 only) is false
		if plugin.ipamInvoker == nil {
			switch nwCfg.IPAM.Type {
			case network.AzureCNS:
				plugin.ipamInvoker = NewCNSInvoker(k8sPodName, k8sNamespace, cnsClient, util.ExecutionMode(nwCfg.ExecutionMode), util.IpamMode(nwCfg.IPAM.Mode))
			default:
				// legacy
				nwInfo := plugin.getNetworkInfo(args.Netns, nil, nwCfg)
				plugin.ipamInvoker = NewAzureIpamInvoker(plugin, &nwInfo)
			}
		}

		ipamAddResult, err = plugin.addIpamInvoker(ipamAddConfig)
		if err != nil {
			return fmt.Errorf("IPAM Invoker Add failed with error: %w", err)
		}
	}

	policies := cni.GetPoliciesFromNwCfg(nwCfg.AdditionalArgs)
	defer func() { //nolint:gocritic
		if err != nil {
			// for swift v1 multi-tenancies scenario, CNI is not supposed to invoke CNS for cleaning Ips
			if !nwCfg.MultiTenancy {
				for _, ifInfo := range ipamAddResult.interfaceInfo {
					// This used to only be called for infraNIC, test if this breaks scenarios
					// If it does then will have to search for infraNIC
					if ifInfo.NICType == cns.InfraNIC {
						plugin.cleanupAllocationOnError(ifInfo.IPConfigs, nwCfg, args, options)
					}
				}
			}
		}
	}()

	infraSeen := false
	endpointIndex := 1

	for key := range ipamAddResult.interfaceInfo {
		ifInfo := ipamAddResult.interfaceInfo[key]
		logger.Info("Processing interfaceInfo:", zap.Any("ifInfo", ifInfo))

		natInfo := getNATInfo(nwCfg, options[network.SNATIPKey], enableSnatForDNS)
		networkID, _ := plugin.getNetworkID(args.Netns, &ifInfo, nwCfg)

		createEpInfoOpt := createEpInfoOpt{
			nwCfg:            nwCfg,
			cnsNetworkConfig: ifInfo.NCResponse,
			ipamAddResult:    ipamAddResult,
			azIpamResult:     azIpamResult,
			args:             args,
			policies:         policies,
			k8sPodName:       k8sPodName,
			k8sNamespace:     k8sNamespace,
			enableInfraVnet:  enableInfraVnet,
			enableSnatForDNS: enableSnatForDNS,
			natInfo:          natInfo,
			networkID:        networkID,
			ifInfo:           &ifInfo,
			ipamAddConfig:    &ipamAddConfig,
			ipv6Enabled:      ipamAddResult.ipv6Enabled,
			infraSeen:        &infraSeen,
			endpointIndex:    endpointIndex,
		}

		var epInfo *network.EndpointInfo
		epInfo, err = plugin.createEpInfo(&createEpInfoOpt)
		if err != nil {
			return err
		}

		epInfos = append(epInfos, epInfo)
		// TODO: should this statement be based on the current iteration instead of the constant ifIndex?
		// TODO figure out where to put telemetry: sendEvent(plugin, fmt.Sprintf("CNI ADD succeeded: IP:%+v, VlanID: %v, podname %v, namespace %v numendpoints:%d",
		//	ipamAddResult.interfaceInfo[ifIndex].IPConfigs, epInfo.Data[network.VlanIDKey], k8sPodName, k8sNamespace, plugin.nm.GetNumberOfEndpoints("", nwCfg.Name)))
		endpointIndex++
	}
	cnsclient, err := cnscli.New(nwCfg.CNSUrl, defaultRequestTimeout)
	if err != nil {
		return errors.Wrap(err, "failed to create cns client")
	}
	defer func() {
		if err != nil {

			// Delete all endpoints
			for _, epInfo := range epInfos {
				deleteErr := plugin.nm.DeleteEndpoint(epInfo.NetworkID, epInfo.EndpointID, epInfo, nwCfg.Mode)
				if deleteErr != nil {
					// we already do not return an error when the endpoint is not found, so deleteErr is a real error
					logger.Error("Could not delete endpoint after detecting add failure", zap.String("epInfo", epInfo.PrettyString()), zap.Error(deleteErr))
					return
				}
			}
			// Rely on cleanupAllocationOnError declared above to delete ips
			// Delete state in disk here
			delErr := plugin.nm.DeleteState(epInfos)
			if delErr != nil {
				logger.Error("Could not delete state after detecting add failure", zap.Error(delErr))
				return
			}
		}
	}()

	err = plugin.nm.EndpointCreate(cnsclient, epInfos)
	if err != nil {
		return errors.Wrap(err, "failed to create endpoint") // behavior can change if you don't assign to err prior to returning
	}
	return nil
}

func (plugin *NetPlugin) findMasterInterface(opt *createEpInfoOpt) string {
	switch opt.ifInfo.NICType {
	case cns.InfraNIC:
		return plugin.findMasterInterfaceBySubnet(opt.ipamAddConfig.nwCfg, &opt.ifInfo.HostSubnetPrefix)
	case cns.NodeNetworkInterfaceFrontendNIC:
		return plugin.findInterfaceByMAC(opt.ifInfo.MacAddress.String())
	case cns.BackendNIC:
		// if windows swiftv2 has right network drivers, there will be an NDIS interface while the VFs are mounted
		// when the VF is dismounted, this interface will go away
		// return an unique interface name to containerd
		return ibInterfacePrefix + strconv.Itoa(opt.endpointIndex)
	case cns.ApipaNIC:
		return apipaInterfacePrefix + strconv.Itoa(opt.endpointIndex)
	default:
		return ""
	}
}

type createEpInfoOpt struct {
	nwCfg            *cni.NetworkConfig
	cnsNetworkConfig *cns.GetNetworkContainerResponse
	ipamAddResult    IPAMAddResult
	azIpamResult     *cniTypesCurr.Result
	args             *cniSkel.CmdArgs
	policies         []policy.Policy
	k8sPodName       string
	k8sNamespace     string
	enableInfraVnet  bool
	enableSnatForDNS bool
	natInfo          []policy.NATInfo
	networkID        string

	ifInfo        *network.InterfaceInfo
	ipamAddConfig *IPAMAddConfig
	ipv6Enabled   bool

	infraSeen     *bool // Only the first infra gets args.ifName, even if the second infra is on a different network
	endpointIndex int
}

func (plugin *NetPlugin) createEpInfo(opt *createEpInfoOpt) (*network.EndpointInfo, error) { // you can modify to pass in whatever else you need
	// ensure we can find the master interface
	opt.ifInfo.HostSubnetPrefix.IP = opt.ifInfo.HostSubnetPrefix.IP.Mask(opt.ifInfo.HostSubnetPrefix.Mask)
	opt.ipamAddConfig.nwCfg.IPAM.Subnet = opt.ifInfo.HostSubnetPrefix.String()

	// populate endpoint info section
	masterIfName := plugin.findMasterInterface(opt)
	if masterIfName == "" {
		err := plugin.Errorf("Failed to find the master interface")
		return nil, err
	}

	networkPolicies := opt.policies // save network policies before we modify the slice pointer for ep policies

	// populate endpoint info
	epDNSInfo, err := getEndpointDNSSettings(opt.nwCfg, opt.ifInfo.DNS, opt.k8sNamespace) // Probably won't panic if given bad values
	if err != nil {
		err = plugin.Errorf("Failed to getEndpointDNSSettings: %v", err)
		return nil, err
	}

	vethName := fmt.Sprintf("%s.%s", opt.k8sNamespace, opt.k8sPodName)
	if opt.nwCfg.Mode != OpModeTransparent {
		// this mechanism of using only namespace and name is not unique for different incarnations of POD/container.
		// IT will result in unpredictable behavior if API server decides to
		// reorder DELETE and ADD call for new incarnation of same POD.
		vethName = fmt.Sprintf("%s%s%s", opt.networkID, opt.args.ContainerID, opt.args.IfName)
	}

	// for secondary (Populate addresses)
	// initially only for infra nic but now applied to all nic types
	addresses := make([]net.IPNet, len(opt.ifInfo.IPConfigs))
	for i, ipconfig := range opt.ifInfo.IPConfigs {
		addresses[i] = ipconfig.Address
	}

	// generate endpoint info
	var endpointID, ifName string

	if opt.ifInfo.NICType == cns.InfraNIC && !*opt.infraSeen {
		// so we do not break existing scenarios, only the first infra gets the original endpoint id generation
		ifName = opt.args.IfName
		endpointID = plugin.nm.GetEndpointID(opt.args.ContainerID, ifName)
		*opt.infraSeen = true
	} else {
		ifName = "eth" + strconv.Itoa(opt.endpointIndex)
		endpointID = plugin.nm.GetEndpointID(opt.args.ContainerID, ifName)
	}

	endpointInfo := network.EndpointInfo{
		NetworkID:                     opt.networkID,
		Mode:                          opt.ipamAddConfig.nwCfg.Mode,
		MasterIfName:                  masterIfName,
		AdapterName:                   opt.ipamAddConfig.nwCfg.AdapterName,
		BridgeName:                    opt.ipamAddConfig.nwCfg.Bridge,
		NetworkPolicies:               networkPolicies, // nw and ep policies separated to avoid possible conflicts
		NetNs:                         opt.ipamAddConfig.args.Netns,
		Options:                       opt.ipamAddConfig.shallowCopyIpamAddConfigOptions(),
		DisableHairpinOnHostInterface: opt.ipamAddConfig.nwCfg.DisableHairpinOnHostInterface,
		IsIPv6Enabled:                 opt.ipv6Enabled, // present infra only

		EndpointID:  endpointID,
		ContainerID: opt.args.ContainerID,
		NetNsPath:   opt.args.Netns, // probably same value as epInfo.NetNs
		IfName:      ifName,
		Data:        make(map[string]interface{}),
		EndpointDNS: epDNSInfo,
		// endpoint policies are populated later
		IPsToRouteViaHost:  opt.nwCfg.IPsToRouteViaHost,
		EnableSnatOnHost:   opt.nwCfg.EnableSnatOnHost,
		EnableMultiTenancy: opt.nwCfg.MultiTenancy,
		EnableInfraVnet:    opt.enableInfraVnet,
		EnableSnatForDns:   opt.enableSnatForDNS,
		PODName:            opt.k8sPodName,
		PODNameSpace:       opt.k8sNamespace,
		SkipHotAttachEp:    false, // Hot attach at the time of endpoint creation
		IPV6Mode:           opt.nwCfg.IPV6Mode,
		VnetCidrs:          opt.nwCfg.VnetCidrs,
		ServiceCidrs:       opt.nwCfg.ServiceCidrs,
		NATInfo:            opt.natInfo,
		NICType:            opt.ifInfo.NICType,
		SkipDefaultRoutes:  opt.ifInfo.SkipDefaultRoutes,
		Routes:             opt.ifInfo.Routes,
		// added the following for delegated vm nic
		IPAddresses: addresses,
		MacAddress:  opt.ifInfo.MacAddress,
		// the following is used for creating an external interface if we can't find an existing network
		HostSubnetPrefix:         opt.ifInfo.HostSubnetPrefix.String(),
		PnPID:                    opt.ifInfo.PnPID,
		NetworkContainerID:       opt.ifInfo.NetworkContainerID,
		AllowInboundFromHostToNC: opt.ifInfo.AllowHostToNCCommunication,
		AllowInboundFromNCToHost: opt.ifInfo.AllowNCToHostCommunication,
	}

	if err = addSubnetToEndpointInfo(*opt.ifInfo, &endpointInfo); err != nil {
		logger.Info("Failed to add subnets to endpointInfo", zap.Error(err))
		return nil, err
	}
	setNetworkOptions(opt.ifInfo.NCResponse, &endpointInfo)

	// update endpoint policies
	policyArgs := PolicyArgs{
		subnetInfos: endpointInfo.Subnets, // getEndpointPolicies requires nwInfo.Subnets only (checked)
		nwCfg:       opt.nwCfg,
		ipconfigs:   opt.ifInfo.IPConfigs,
	}
	endpointPolicies, err := getEndpointPolicies(policyArgs)
	if err != nil {
		logger.Error("Failed to get endpoint policies", zap.Error(err))
		return nil, err
	}
	// create endpoint policies by appending to network policies
	// the value passed into NetworkPolicies should be unaffected since we reassign here
	opt.policies = append(opt.policies, endpointPolicies...)

	// appends endpoint policies specific to this interface
	opt.policies = append(opt.policies, opt.ifInfo.EndpointPolicies...)

	endpointInfo.EndpointPolicies = opt.policies
	// add even more endpoint policies
	epPolicies, err := getPoliciesFromRuntimeCfg(opt.nwCfg, opt.ipamAddResult.ipv6Enabled) // not specific to delegated or infra
	if err != nil {
		logger.Error("failed to get policies from runtime configurations", zap.Error(err))
		return nil, plugin.Errorf("%s", err.Error())
	}
	endpointInfo.EndpointPolicies = append(endpointInfo.EndpointPolicies, epPolicies...)

	if opt.ipamAddResult.ipv6Enabled { // not specific to this particular interface
		endpointInfo.IPV6Mode = string(util.IpamMode(opt.nwCfg.IPAM.Mode)) // TODO: check IPV6Mode field can be deprecated and can we add IsIPv6Enabled flag for generic working
	}

	if opt.azIpamResult != nil && opt.azIpamResult.IPs != nil {
		endpointInfo.InfraVnetIP = opt.azIpamResult.IPs[0].Address
	}

	if opt.nwCfg.MultiTenancy {
		// previously only infra nic was passed into this function but now all nics are passed in (possibly breaks swift v2)
		plugin.multitenancyClient.SetupRoutingForMultitenancy(opt.nwCfg, opt.cnsNetworkConfig, opt.azIpamResult, &endpointInfo, opt.ifInfo)
	}

	setEndpointOptions(opt.cnsNetworkConfig, &endpointInfo, vethName)

	logger.Info("Generated endpoint info from fields", zap.String("epInfo", endpointInfo.PrettyString()))

	// now our ep info should have the full combined information from both the network and endpoint structs
	return &endpointInfo, nil
}

// cleanup allocated ipv4 and ipv6 addresses if they exist
func (plugin *NetPlugin) cleanupAllocationOnError(
	result []*network.IPConfig,
	nwCfg *cni.NetworkConfig,
	args *cniSkel.CmdArgs,
	options map[string]interface{},
) {
	if result != nil {
		for i := 0; i < len(result); i++ {
			if er := plugin.ipamInvoker.Delete(&result[i].Address, nwCfg, args, options); er != nil {
				logger.Error("Failed to cleanup ip allocation on failure", zap.Error(er))
			}
		}
	}
}

// construct network info with ipv4/ipv6 subnets (updates subnets field)
func addSubnetToEndpointInfo(interfaceInfo network.InterfaceInfo, nwInfo *network.EndpointInfo) error {
	for _, ipConfig := range interfaceInfo.IPConfigs {
		ip, podSubnetPrefix, err := net.ParseCIDR(ipConfig.Address.String())
		if err != nil {
			return fmt.Errorf("Failed to ParseCIDR for pod subnet prefix: %w", err)
		}

		subnet := network.SubnetInfo{
			Family:  platform.AfINET,
			Prefix:  *podSubnetPrefix,
			Gateway: ipConfig.Gateway,
		}
		if ip.To4() == nil {
			subnet.Family = platform.AfINET6
		}

		nwInfo.Subnets = append(nwInfo.Subnets, subnet)
	}

	return nil
}

// Get handles CNI Get commands.
func (plugin *NetPlugin) Get(args *cniSkel.CmdArgs) error {
	var (
		result    cniTypesCurr.Result
		err       error
		nwCfg     *cni.NetworkConfig
		epInfo    *network.EndpointInfo
		iface     *cniTypesCurr.Interface
		networkID string
	)

	logger.Info("Processing GET command",
		zap.String("container", args.ContainerID),
		zap.String("netns", args.Netns),
		zap.String("ifname", args.IfName),
		zap.String("args", args.Args),
		zap.String("path", args.Path))

	defer func() {
		// Add Interfaces to result.
		iface = &cniTypesCurr.Interface{
			Name: args.IfName,
		}
		result.Interfaces = append(result.Interfaces, iface)

		// Convert result to the requested CNI version.
		res, vererr := result.GetAsVersion(nwCfg.CNIVersion)
		if vererr != nil {
			logger.Error("GetAsVersion failed", zap.Error(vererr))
			plugin.Error(vererr)
		}

		if err == nil && res != nil {
			// Output the result to stdout.
			res.Print()
		}

		logger.Info("GET command completed", zap.Any("result", result),
			zap.Error(log.NewErrorWithoutStackTrace(err)))
	}()

	// Parse network configuration from stdin.
	if nwCfg, err = cni.ParseNetworkConfig(args.StdinData); err != nil {
		err = plugin.Errorf("Failed to parse network configuration: %v.", err)
		return err
	}

	logger.Info("Read network configuration", zap.Any("config", nwCfg))

	if argErr := plugin.validateArgs(args, nwCfg); argErr != nil {
		err = argErr
		return err
	}

	iptables.DisableIPTableLock = nwCfg.DisableIPTableLock

	// Initialize values from network config.
	if networkID, err = plugin.getNetworkName(args.Netns, nil, nwCfg); err != nil {
		// TODO: Ideally we should return from here only.
		logger.Error("Failed to extract network name from network config",
			zap.Error(err))
	}

	endpointID := GetEndpointID(args)

	// Query the network.
	if _, err = plugin.nm.GetNetworkInfo(networkID); err != nil {
		logger.Error("Failed to query network", zap.Error(err))
		return err
	}

	// Query the endpoint.
	if epInfo, err = plugin.nm.GetEndpointInfo(networkID, endpointID); err != nil {
		logger.Error("Failed to query endpoint", zap.Error(err))
		return err
	}

	for _, ipAddresses := range epInfo.IPAddresses {
		ipConfig := &cniTypesCurr.IPConfig{
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

	result.DNS.Nameservers = epInfo.EndpointDNS.Servers
	result.DNS.Domain = epInfo.EndpointDNS.Suffix

	return nil
}

// Delete handles CNI delete commands.
func (plugin *NetPlugin) Delete(args *cniSkel.CmdArgs) error {
	var (
		err          error
		nwCfg        *cni.NetworkConfig
		k8sPodName   string
		k8sNamespace string
		networkID    string
		nwInfo       network.EndpointInfo
	)
	startTime := time.Now()
	logger.Info("Processing DEL command",
		zap.String("containerId", args.ContainerID),
		zap.String("netNS", args.Netns),
		zap.String("ifName", args.IfName),
		zap.Any("args", args.Args),
		zap.String("path", args.Path),
		zap.ByteString("stdinData", args.StdinData))

	defer func() {
		logger.Info("DEL command completed",
			zap.String("pod", k8sPodName),
			zap.Error(log.NewErrorWithoutStackTrace(err)))
		telemetryClient.SendEvent(fmt.Sprintf("DEL command completed: [podname]: %s [namespace]: %s [error]: %v", k8sPodName, k8sNamespace, err))
		operationTimeMs := time.Since(startTime).Milliseconds()
		telemetryClient.SendMetric(telemetry.CNIDelTimeMetricStr, float64(operationTimeMs), make(map[string]string))
	}()

	// Parse network configuration from stdin.
	if nwCfg, err = cni.ParseNetworkConfig(args.StdinData); err != nil {
		err = plugin.Errorf("[cni-net] Failed to parse network configuration: %v", err)
		return err
	}

	if argErr := plugin.validateArgs(args, nwCfg); argErr != nil {
		err = argErr
		return err
	}

	// Parse Pod arguments.
	if k8sPodName, k8sNamespace, err = plugin.getPodInfo(args.Args); err != nil {
		logger.Error("Failed to get POD info", zap.Error(err))
	}
	telemetryClient.Settings().ContainerName = k8sPodName + ":" + k8sNamespace

	plugin.setCNIReportDetails(args.ContainerID, CNI_DEL, "")
	telemetryClient.SendEvent(fmt.Sprintf("[cni-net] Processing DEL command with args {ContainerID:%v Netns:%v IfName:%v Args:%v Path:%v, StdinData:%s}.",
		args.ContainerID, args.Netns, args.IfName, args.Args, args.Path, args.StdinData))

	iptables.DisableIPTableLock = nwCfg.DisableIPTableLock

	platformInit(nwCfg)

	logger.Info("Execution mode", zap.String("mode", nwCfg.ExecutionMode))
	if nwCfg.ExecutionMode == string(util.Baremetal) {
		_, err = plugin.nnsClient.DeleteContainerNetworking(context.Background(), k8sPodName, args.Netns)
		if err != nil {
			return fmt.Errorf("nnsClient.DeleteContainerNetworking failed with err %w", err)
		}
	}

	if plugin.ipamInvoker == nil {
		switch nwCfg.IPAM.Type {
		case network.AzureCNS:
			cnsClient, cnsErr := cnscli.New("", defaultRequestTimeout)
			if cnsErr != nil {
				logger.Error("failed to create cns client", zap.Error(cnsErr))
				return errors.Wrap(cnsErr, "failed to create cns client")
			}
			plugin.ipamInvoker = NewCNSInvoker(k8sPodName, k8sNamespace, cnsClient, util.ExecutionMode(nwCfg.ExecutionMode), util.IpamMode(nwCfg.IPAM.Mode))

		default:
			// nwInfo gets populated later in the function
			plugin.ipamInvoker = NewAzureIpamInvoker(plugin, &nwInfo)
		}
	}

	// Loop through all the networks that are created for the given Netns. In case of multi-nic scenario ( currently supported
	// scenario is dual-nic ), single container may have endpoints created in multiple networks. As all the endpoints are
	// deleted, getNetworkName will return error of the type NetworkNotFoundError which will result in nil error as compliance
	// with CNI SPEC as mentioned below.

	// We get the network id and nw info here to preserve existing behavior
	networkID, err = plugin.getNetworkID(args.Netns, nil, nwCfg)
	if nwInfo, err = plugin.nm.GetNetworkInfo(networkID); err != nil {
		if !nwCfg.MultiTenancy {
			logger.Error("Failed to query network",
				zap.String("network", networkID),
				zap.Error(err))
			// Log the error if the network is not found.
			// if cni hits this, mostly state file would be missing and it can be reboot scenario where
			// container runtime tries to delete and create pods which existed before reboot.
			// this condition will not apply to stateless CNI since the network struct will be crated on each call
			err = nil
		}
	}
	// Initialize values from network config.
	if err != nil {
		// if swift v1 multitenancy and we got an error retrieving the nwInfo
		// If error is not found error, then we ignore it, to comply with CNI SPEC.
		if network.IsNetworkNotFoundError(err) {
			err = nil
			return err
		}

		logger.Error("Failed to extract network name from network config", zap.Error(err))
		err = plugin.Errorf("Failed to extract network name from network config. error: %v", err)
		return err
	}
	logger.Info("Retrieved network info, populating endpoint infos with container id", zap.String("containerID", args.ContainerID))

	var epInfos []*network.EndpointInfo
	if plugin.nm.IsStatelessCNIMode() {
		// network ID is passed in and used only for migration
		// otherwise, in stateless, we don't need the network id for deletion
		epInfos, err = plugin.nm.GetEndpointState(networkID, args.ContainerID)
		// if stateless CNI fail to get the endpoint from CNS for any reason other than  Endpoint Not found
		if err != nil {
			// async delete should be disabled for standalone scenario
			if errors.Is(err, network.ErrConnectionFailure) && !nwCfg.DisableAsyncDelete {
				logger.Info("failed to connect to CNS", zap.String("containerID", args.ContainerID), zap.Error(err))
				addErr := fsnotify.AddFile(args.ContainerID, args.ContainerID, watcherPath)
				logger.Info("add containerid file for Asynch delete", zap.String("containerID", args.ContainerID), zap.Error(addErr))
				if addErr != nil {
					logger.Error("failed to add file to watcher", zap.String("containerID", args.ContainerID), zap.Error(addErr))
					return errors.Wrap(addErr, fmt.Sprintf("failed to add file to watcher with containerID %s", args.ContainerID))
				}
				return nil
			}
			if errors.Is(err, network.ErrEndpointStateNotFound) {
				logger.Info("Endpoint Not found", zap.String("containerID", args.ContainerID), zap.Error(err))
				return nil
			}
			logger.Error("Get Endpoint State API returned error", zap.String("containerID", args.ContainerID), zap.Error(err))
			return plugin.RetriableError(fmt.Errorf("failed to delete endpoint: %w", err))
		}
	} else {
		epInfos = plugin.nm.GetEndpointInfosFromContainerID(args.ContainerID)
	}

	// for when the endpoint is not created, but the ips are already allocated (only works if single network, single infra)
	// this block is not applied to stateless CNI
	if len(epInfos) == 0 {
		endpointID := plugin.nm.GetEndpointID(args.ContainerID, args.IfName)
		if !nwCfg.MultiTenancy {
			logger.Warn("Could not query endpoint",
				zap.String("endpoint", endpointID),
				zap.Error(err))

			logger.Warn("Release ip by ContainerID (endpoint not found)",
				zap.String("containerID", args.ContainerID))
			if err = plugin.ipamInvoker.Delete(nil, nwCfg, args, nwInfo.Options); err != nil {
				return plugin.RetriableError(fmt.Errorf("failed to release address(no endpoint): %w", err))
			}
		}
		// Log the error but return success if the endpoint being deleted is not found.
		err = nil
		return err
	}
	logger.Info("Deleting the endpoints", zap.Any("endpointInfos", epInfos))
	// populate ep infos here in loop if necessary
	// delete endpoints
	for _, epInfo := range epInfos {
		// in stateless, network id is not populated in epInfo, but in stateful cni, it is (nw id is used in stateful)
		if err = plugin.nm.DeleteEndpoint(epInfo.NetworkID, epInfo.EndpointID, epInfo, nwCfg.Mode); err != nil {
			// An error will not be returned if the endpoint is not found
			// return a retriable error so the container runtime will retry this DEL later
			// the implementation of this function returns nil if the endpoint doens't exist, so
			// we don't have to check that here
			return plugin.RetriableError(fmt.Errorf("failed to delete endpoint: %w", err))
		}
	}
	logger.Info("Deleting the endpoints from the ipam")
	// delete endpoint state in cns and in statefile
	for _, epInfo := range epInfos {
		logger.Info("Deleting endpoint",
			zap.String("endpointID", epInfo.EndpointID))
		telemetryClient.SendEvent("Deleting endpoint: " + epInfo.EndpointID)

		if !nwCfg.MultiTenancy && (epInfo.NICType == cns.InfraNIC || epInfo.NICType == "") {
			// Delegated/secondary nic ips are statically allocated so we don't need to release
			// Call into IPAM plugin to release the endpoint's addresses.
			for i := range epInfo.IPAddresses {
				logger.Info("Release ip", zap.String("ip", epInfo.IPAddresses[i].IP.String()))
				telemetryClient.SendEvent(fmt.Sprintf("Release ip: %s container id: %s endpoint id: %s", epInfo.IPAddresses[i].IP.String(), args.ContainerID, epInfo.EndpointID))
				err = plugin.ipamInvoker.Delete(&epInfo.IPAddresses[i], nwCfg, args, nwInfo.Options)
				if err != nil {
					return plugin.RetriableError(fmt.Errorf("failed to release address: %w", err))
				}
			}
		} else if epInfo.EnableInfraVnet { // remove in future PR
			nwCfg.IPAM.Subnet = nwInfo.Subnets[0].Prefix.String()
			nwCfg.IPAM.Address = epInfo.InfraVnetIP.IP.String()
			err = plugin.ipamInvoker.Delete(nil, nwCfg, args, nwInfo.Options)
			if err != nil {
				return plugin.RetriableError(fmt.Errorf("failed to release address: %w", err))
			}
		}
	}
	logger.Info("Deleting endpoint state from statefile")
	err = plugin.nm.DeleteState(epInfos)
	if err != nil {
		return plugin.RetriableError(fmt.Errorf("failed to delete state: %w", err))
	}

	return err
}

// Update handles CNI update commands.
// Update is only supported for multitenancy and to update routes.
func (plugin *NetPlugin) Update(args *cniSkel.CmdArgs) error {
	var (
		result              *cniTypesCurr.Result
		err                 error
		nwCfg               *cni.NetworkConfig
		existingEpInfo      *network.EndpointInfo
		podCfg              *cni.K8SPodEnvArgs
		orchestratorContext []byte
		targetNetworkConfig *cns.GetNetworkContainerResponse
	)

	logger.Info("Processing UPDATE command",
		zap.String("netns", args.Netns),
		zap.String("args", args.Args),
		zap.String("path", args.Path))

	// Parse network configuration from stdin.
	if nwCfg, err = cni.ParseNetworkConfig(args.StdinData); err != nil {
		err = plugin.Errorf("Failed to parse network configuration: %v.", err)
		return err
	}

	if argErr := plugin.validateArgs(args, nwCfg); argErr != nil {
		err = argErr
		return err
	}

	logger.Info("Read network configuration", zap.Any("config", nwCfg))

	iptables.DisableIPTableLock = nwCfg.DisableIPTableLock
	plugin.setCNIReportDetails(args.ContainerID, CNI_UPDATE, "")

	defer func() {
		if result == nil {
			result = &cniTypesCurr.Result{}
		}

		// Convert result to the requested CNI version.
		res, vererr := result.GetAsVersion(nwCfg.CNIVersion)
		if vererr != nil {
			logger.Error("GetAsVersion failed", zap.Error(vererr))
			plugin.Error(vererr) //nolint
		}

		if err == nil && res != nil {
			// Output the result to stdout.
			res.Print()
		}

		logger.Info("UPDATE command completed",
			zap.Any("result", result),
			zap.Error(log.NewErrorWithoutStackTrace(err)))
	}()

	// Parse Pod arguments.
	if podCfg, err = cni.ParseCniArgs(args.Args); err != nil {
		logger.Error("Error while parsing CNI Args during UPDATE",
			zap.Error(err))
		return err
	}

	k8sNamespace := string(podCfg.K8S_POD_NAMESPACE)
	if len(k8sNamespace) == 0 {
		errMsg := "Required parameter Pod Namespace not specified in CNI Args during UPDATE"
		logger.Error(errMsg)
		return plugin.Errorf("%s", errMsg)
	}

	k8sPodName := string(podCfg.K8S_POD_NAME)
	if len(k8sPodName) == 0 {
		errMsg := "Required parameter Pod Name not specified in CNI Args during UPDATE"
		logger.Error(errMsg)
		return plugin.Errorf("%s", errMsg)
	}

	// Initialize values from network config.
	networkID := nwCfg.Name

	// Query the network.
	if _, err = plugin.nm.GetNetworkInfo(networkID); err != nil {
		errMsg := fmt.Sprintf("Failed to query network during CNI UPDATE: %v", err)
		logger.Error(errMsg)
		return plugin.Errorf("%s", errMsg)
	}

	// Query the existing endpoint since this is an update.
	// Right now, we do not support updating pods that have multiple endpoints.
	existingEpInfo, err = plugin.nm.GetEndpointInfoBasedOnPODDetails(networkID, k8sPodName, k8sNamespace, nwCfg.EnableExactMatchForPodName)
	if err != nil {
		plugin.Errorf("Failed to retrieve target endpoint for CNI UPDATE [name=%v, namespace=%v]: %v", k8sPodName, k8sNamespace, err)
		return err
	}

	logger.Info("Retrieved existing endpoint from state that may get update",
		zap.Any("info", existingEpInfo))

	// now query CNS to get the target routes that should be there in the networknamespace (as a result of update)
	logger.Info("Going to collect target routes from CNS",
		zap.String("pod", k8sPodName),
		zap.String("namespace", k8sNamespace))

	// create struct with info for target POD
	podInfo := cns.KubernetesPodInfo{
		PodName:      k8sPodName,
		PodNamespace: k8sNamespace,
	}
	if orchestratorContext, err = json.Marshal(podInfo); err != nil {
		logger.Error("Marshalling KubernetesPodInfo failed",
			zap.Error(err))
		return plugin.Errorf("%s", err.Error())
	}

	cnsclient, err := cnscli.New(nwCfg.CNSUrl, defaultRequestTimeout)
	if err != nil {
		logger.Error("failed to initialized cns client",
			zap.String("url", nwCfg.CNSUrl),
			zap.String("error", err.Error()))
		return plugin.Errorf("%s", err.Error())
	}

	if targetNetworkConfig, err = cnsclient.GetNetworkContainer(context.TODO(), orchestratorContext); err != nil {
		logger.Info("GetNetworkContainer failed",
			zap.Error(err))
		return plugin.Errorf("%s", err.Error())
	}

	logger.Info("Network config received from cns",
		zap.String("pod", k8sPodName),
		zap.String("namespace", k8sNamespace),
		zap.Any("config", targetNetworkConfig))
	targetEpInfo := &network.EndpointInfo{}

	// get the target routes that should replace existingEpInfo.Routes inside the network namespace
	if targetNetworkConfig.Routes != nil && len(targetNetworkConfig.Routes) > 0 {
		for _, route := range targetNetworkConfig.Routes {
			logger.Info("Adding route from routes from targetNetworkConfig to targetEpInfo", zap.Any("route", route))
			_, dstIPNet, _ := net.ParseCIDR(route.IPAddress)
			gwIP := net.ParseIP(route.GatewayIPAddress)
			targetEpInfo.Routes = append(targetEpInfo.Routes, network.RouteInfo{Dst: *dstIPNet, Gw: gwIP, DevName: existingEpInfo.IfName})
		}
	}

	logger.Info("Going to collect target routes based on Cnetaddressspace from targetNetworkConfig",
		zap.String("pod", k8sPodName),
		zap.String("namespace", k8sNamespace))

	ipconfig := targetNetworkConfig.IPConfiguration
	for _, ipRouteSubnet := range targetNetworkConfig.CnetAddressSpace {
		logger.Info("Adding route from cnetAddressspace to targetEpInfo", zap.Any("subnet", ipRouteSubnet))
		dstIPNet := net.IPNet{IP: net.ParseIP(ipRouteSubnet.IPAddress), Mask: net.CIDRMask(int(ipRouteSubnet.PrefixLength), 32)}
		gwIP := net.ParseIP(ipconfig.GatewayIPAddress)
		route := network.RouteInfo{Dst: dstIPNet, Gw: gwIP, DevName: existingEpInfo.IfName}
		targetEpInfo.Routes = append(targetEpInfo.Routes, route)
	}

	logger.Info("Finished collecting new routes in targetEpInfo", zap.Any("route", targetEpInfo.Routes))
	logger.Info("Now saving existing infravnetaddress space if needed.")
	for _, ns := range nwCfg.PodNamespaceForDualNetwork {
		if k8sNamespace == ns {
			targetEpInfo.EnableInfraVnet = true
			targetEpInfo.InfraVnetAddressSpace = nwCfg.InfraVnetAddressSpace
			logger.Info("Saving infravnet address space",
				zap.String("space", targetEpInfo.InfraVnetAddressSpace),
				zap.String("namespace", existingEpInfo.PODNameSpace),
				zap.String("pod", existingEpInfo.PODName))
			break
		}
	}

	// Update the endpoint.
	logger.Info("Now updating existing endpoint with targetNetworkConfig",
		zap.String("endpoint", existingEpInfo.EndpointID),
		zap.Any("config", targetNetworkConfig))
	if err = plugin.nm.UpdateEndpoint(networkID, existingEpInfo, targetEpInfo); err != nil {
		err = plugin.Errorf("Failed to update endpoint: %v", err)
		return err
	}

	msg := fmt.Sprintf("CNI UPDATE succeeded : Updated %+v podname %v namespace %v", targetNetworkConfig, k8sPodName, k8sNamespace)
	plugin.setCNIReportDetails(args.ContainerID, CNI_UPDATE, msg)

	return nil
}

func convertNnsToIPConfigs(
	netRes *nnscontracts.ConfigureContainerNetworkingResponse,
	ifName string,
	podName string,
	operationName string,
) []*network.IPConfig {
	// This function does not add interfaces to CNI result. Reason being CRI (containerD in baremetal case)
	// only looks for default interface named "eth0" and this default interface is added in the defer
	// method of ADD method
	var ipConfigs []*network.IPConfig

	if netRes.Interfaces != nil {
		for _, ni := range netRes.Interfaces {
			for _, ip := range ni.Ipaddresses {
				ipAddr := net.ParseIP(ip.Ip)

				prefixLength, err := strconv.Atoi(ip.PrefixLength)
				if err != nil {
					logger.Error("Error parsing prefix length while converting to cni result",
						zap.String("prefixLength", ip.PrefixLength),
						zap.String("operation", operationName),
						zap.String("pod", podName),
						zap.Error(err))
					continue
				}

				address := net.IPNet{
					IP:   ipAddr,
					Mask: net.CIDRMask(prefixLength, ipv6FullMask),
				}

				if ipAddr.To4() != nil {
					address.Mask = net.CIDRMask(prefixLength, ipv4FullMask)
				}

				gateway := net.ParseIP(ip.DefaultGateway)

				ipConfigs = append(ipConfigs, &network.IPConfig{
					Address: address,
					Gateway: gateway,
				})
			}
		}
	}

	return ipConfigs
}

func convertInterfaceInfoToCniResult(info network.InterfaceInfo, ifName string) *cniTypesCurr.Result {
	result := &cniTypesCurr.Result{
		Interfaces: []*cniTypesCurr.Interface{
			{
				Name: ifName,
				Mac:  info.MacAddress.String(),
			},
		},
		DNS: cniTypes.DNS{
			Domain:      info.DNS.Suffix,
			Nameservers: info.DNS.Servers,
		},
	}

	if len(info.IPConfigs) > 0 {
		for _, ipconfig := range info.IPConfigs {
			result.IPs = append(result.IPs, &cniTypesCurr.IPConfig{Address: ipconfig.Address, Gateway: ipconfig.Gateway})
		}

		for i := range info.Routes {
			result.Routes = append(result.Routes, &cniTypes.Route{Dst: info.Routes[i].Dst, GW: info.Routes[i].Gw})
		}
	}

	return result
}

func convertCniResultToInterfaceInfo(result *cniTypesCurr.Result) network.InterfaceInfo {
	interfaceInfo := network.InterfaceInfo{}

	if result != nil {
		for _, ipconfig := range result.IPs {
			interfaceInfo.IPConfigs = append(interfaceInfo.IPConfigs, &network.IPConfig{Address: ipconfig.Address, Gateway: ipconfig.Gateway})
		}

		for _, route := range result.Routes {
			interfaceInfo.Routes = append(interfaceInfo.Routes, network.RouteInfo{Dst: route.Dst, Gw: route.GW})
		}

		interfaceInfo.DNS = network.DNSInfo{
			Suffix:  result.DNS.Domain,
			Servers: result.DNS.Nameservers,
		}
	}

	return interfaceInfo
}

func (plugin *NetPlugin) validateArgs(args *cniSkel.CmdArgs, nwCfg *cni.NetworkConfig) error {
	if !allowedInput.MatchString(args.ContainerID) || !allowedInput.MatchString(args.IfName) {
		return errors.New("invalid args value")
	}
	if !allowedInput.MatchString(nwCfg.Bridge) {
		return errors.New("invalid network config value")
	}

	return nil
}
