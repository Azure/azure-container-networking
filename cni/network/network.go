// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/api"
	"github.com/Azure/azure-container-networking/cni/utils"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/cnsclient"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/Azure/azure-container-networking/platform"
	nnscontracts "github.com/Azure/azure-container-networking/proto/nodenetworkservice/3.302.0.744"
	"github.com/Azure/azure-container-networking/store"
	"github.com/Azure/azure-container-networking/telemetry"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
)

const (
	dockerNetworkOption = "com.docker.network.generic"
	opModeTransparent   = "transparent"
	// Supported IP version. Currently support only IPv4
	ipVersion             = "4"
	ipamV6                = "azure-vnet-ipamv6"
	defaultRequestTimeout = 15 * time.Second
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

type ExecutionMode string

const (
	Default   ExecutionMode = "default"
	Baremetal ExecutionMode = "baremetal"
)

// NetPlugin represents the CNI network plugin.
type netPlugin struct {
	*cni.Plugin
	nm          network.NetworkManager
	ipamInvoker IPAMInvoker
	report      *telemetry.CNIReport
	tb          *telemetry.TelemetryBuffer
	nnsClient   NnsClient
}

// client for node network service
type NnsClient interface {
	// Do network port programming for the pod via node network service.
	// podName - name of the pod as received from containerD
	// nwNamesapce - network namespace name as received from containerD
	AddContainerNetworking(ctx context.Context, podName, nwNamespace string) (error, *nnscontracts.ConfigureContainerNetworkingResponse)

	// Undo or delete network port programming for the pod via node network service.
	// podName - name of the pod as received from containerD
	// nwNamesapce - network namespace name as received from containerD
	DeleteContainerNetworking(ctx context.Context, podName, nwNamespace string) (error, *nnscontracts.ConfigureContainerNetworkingResponse)
}

// snatConfiguration contains a bool that determines whether CNI enables snat on host and snat for dns
type snatConfiguration struct {
	EnableSnatOnHost bool
	EnableSnatForDns bool
}

// NewPlugin creates a new netPlugin object.
func NewPlugin(name string, config *common.PluginConfig, client NnsClient) (*netPlugin, error) {
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
		Plugin:    plugin,
		nm:        nm,
		nnsClient: client,
	}, nil
}

func (plugin *netPlugin) SetCNIReport(report *telemetry.CNIReport, tb *telemetry.TelemetryBuffer) {
	plugin.report = report
	plugin.tb = tb
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
	platform.PrintDependencyPackageDetails()
	common.LogNetworkInterfaces()

	// Initialize network manager.
	err = plugin.nm.Initialize(config, rehydrateNetworkInfoOnReboot)
	if err != nil {
		log.Printf("[cni-net] Failed to initialize network manager, err:%v.", err)
		return err
	}

	log.Printf("[cni-net] Plugin started.")

	return nil
}

func (plugin *netPlugin) GetAllEndpointState(networkid string) (*api.AzureCNIState, error) {
	st := api.AzureCNIState{
		ContainerInterfaces: make(map[string]api.PodNetworkInterfaceInfo),
	}

	eps, err := plugin.nm.GetAllEndpoints(networkid)
	if err == store.ErrStoreEmpty {
		log.Printf("failed to retrieve endpoint state with err %v", err)
	} else if err != nil {
		return nil, err
	}

	for _, ep := range eps {
		id := ep.Id
		info := api.PodNetworkInterfaceInfo{
			PodName:       ep.PODName,
			PodNamespace:  ep.PODNameSpace,
			PodEndpointId: ep.Id,
			ContainerID:   ep.ContainerID,
			IPAddresses:   ep.IPAddresses,
		}

		st.ContainerInterfaces[id] = info
	}

	return &st, nil
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

// getPodInfo returns POD info by parsing the CNI args.
func (plugin *netPlugin) getPodInfo(args string) (string, string, error) {
	podCfg, err := cni.ParseCniArgs(args)
	if err != nil {
		log.Printf("Error while parsing CNI Args %v", err)
		return "", "", err
	}

	k8sNamespace := string(podCfg.K8S_POD_NAMESPACE)
	if len(k8sNamespace) == 0 {
		errMsg := "Pod Namespace not specified in CNI Args"
		log.Printf(errMsg)
		return "", "", plugin.Errorf(errMsg)
	}

	k8sPodName := string(podCfg.K8S_POD_NAME)
	if len(k8sPodName) == 0 {
		errMsg := "Pod Name not specified in CNI Args"
		log.Printf(errMsg)
		return "", "", plugin.Errorf(errMsg)
	}

	return k8sPodName, k8sNamespace, nil
}

func SetCustomDimensions(cniMetric *telemetry.AIMetric, nwCfg *cni.NetworkConfig, err error) {
	if cniMetric == nil {
		log.Errorf("[CNI] Unable to set custom dimension. Report is nil")
		return
	}

	if err != nil {
		cniMetric.Metric.CustomDimensions[telemetry.StatusStr] = telemetry.FailedStr
	} else {
		cniMetric.Metric.CustomDimensions[telemetry.StatusStr] = telemetry.SucceededStr
	}

	if nwCfg != nil {
		if nwCfg.MultiTenancy {
			cniMetric.Metric.CustomDimensions[telemetry.CNIModeStr] = telemetry.MultiTenancyStr
		} else {
			cniMetric.Metric.CustomDimensions[telemetry.CNIModeStr] = telemetry.SingleTenancyStr
		}

		cniMetric.Metric.CustomDimensions[telemetry.CNINetworkModeStr] = nwCfg.Mode
	}
}

func (plugin *netPlugin) setCNIReportDetails(nwCfg *cni.NetworkConfig, opType string, msg string) {
	if nwCfg.MultiTenancy {
		plugin.report.Context = "AzureCNIMultitenancy"
	}

	plugin.report.OperationType = opType
	plugin.report.SubContext = fmt.Sprintf("%+v", nwCfg)
	plugin.report.EventMessage = msg
	plugin.report.BridgeDetails.NetworkMode = nwCfg.Mode
	plugin.report.InterfaceDetails.SecondaryCAUsedCount = plugin.nm.GetNumberOfEndpoints("", nwCfg.Name)
}

func addNatIPV6SubnetInfo(nwCfg *cni.NetworkConfig,
	resultV6 *cniTypesCurr.Result,
	nwInfo *network.NetworkInfo) {
	if nwCfg.IPV6Mode == network.IPV6Nat {
		ipv6Subnet := resultV6.IPs[0].Address
		ipv6Subnet.IP = ipv6Subnet.IP.Mask(ipv6Subnet.Mask)
		ipv6SubnetInfo := network.SubnetInfo{
			Family:  platform.AfINET6,
			Prefix:  ipv6Subnet,
			Gateway: resultV6.IPs[0].Gateway,
		}
		log.Printf("[net] ipv6 subnet info:%+v", ipv6SubnetInfo)
		nwInfo.Subnets = append(nwInfo.Subnets, ipv6SubnetInfo)
	}
}

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

// Get handles CNI Get commands.
func (plugin *netPlugin) Get(args *cniSkel.CmdArgs) error {
	var (
		result       cniTypesCurr.Result
		err          error
		nwCfg        *cni.NetworkConfig
		epInfo       *network.EndpointInfo
		iface        *cniTypesCurr.Interface
		k8sPodName   string
		k8sNamespace string
		networkId    string
	)

	log.Printf("[cni-net] Processing GET command with args {ContainerID:%v Netns:%v IfName:%v Args:%v Path:%v}.",
		args.ContainerID, args.Netns, args.IfName, args.Args, args.Path)

	defer func() {
		// Add Interfaces to result.
		iface = &cniTypesCurr.Interface{
			Name: args.IfName,
		}
		result.Interfaces = append(result.Interfaces, iface)

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

		log.Printf("[cni-net] GET command completed with result:%+v err:%v.", result, err)
	}()

	// Parse network configuration from stdin.
	if nwCfg, err = cni.ParseNetworkConfig(args.StdinData); err != nil {
		err = plugin.Errorf("Failed to parse network configuration: %v.", err)
		return err
	}

	log.Printf("[cni-net] Read network configuration %+v.", nwCfg)

	iptables.DisableIPTableLock = nwCfg.DisableIPTableLock

	// Parse Pod arguments.
	if k8sPodName, k8sNamespace, err = plugin.getPodInfo(args.Args); err != nil {
		return err
	}

	if nwCfg.MultiTenancy {
		// Initialize CNSClient
		cnsclient.InitCnsClient(nwCfg.CNSUrl, defaultRequestTimeout)
	}

	// Initialize values from network config.
	if networkId, err = getNetworkName(k8sPodName, k8sNamespace, args.IfName, nwCfg); err != nil {
		// TODO: Ideally we should return from here only.
		log.Printf("[cni-net] Failed to extract network name from network config. error: %v", err)
	}

	endpointId := GetEndpointID(args)

	// Query the network.
	if _, err = plugin.nm.GetNetworkInfo(networkId); err != nil {
		plugin.Errorf("Failed to query network: %v", err)
		return err
	}

	// Query the endpoint.
	if epInfo, err = plugin.nm.GetEndpointInfo(networkId, endpointId); err != nil {
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
	var (
		err          error
		nwCfg        *cni.NetworkConfig
		k8sPodName   string
		k8sNamespace string
		networkId    string
		nwInfo       network.NetworkInfo
		epInfo       *network.EndpointInfo
		cniMetric    telemetry.AIMetric
		msg          string
	)

	startTime := time.Now()

	log.Printf("[cni-net] Processing DEL command with args {ContainerID:%v Netns:%v IfName:%v Args:%v Path:%v, StdinData:%s}.",
		args.ContainerID, args.Netns, args.IfName, args.Args, args.Path, args.StdinData)

	defer func() {
		log.Printf("[cni-net] DEL command completed with err:%v.", err)
	}()

	// Parse network configuration from stdin.
	if nwCfg, err = cni.ParseNetworkConfig(args.StdinData); err != nil {
		err = plugin.Errorf("[cni-net] Failed to parse network configuration: %v", err)
		return err
	}

	log.Printf("[cni-net] Read network configuration %+v.", nwCfg)

	// Parse Pod arguments.
	if k8sPodName, k8sNamespace, err = plugin.getPodInfo(args.Args); err != nil {
		log.Printf("[cni-net] Failed to get POD info due to error: %v", err)
	}

	plugin.setCNIReportDetails(nwCfg, CNI_DEL, "")
	iptables.DisableIPTableLock = nwCfg.DisableIPTableLock

	sendMetricFunc := func() {
		operationTimeMs := time.Since(startTime).Milliseconds()
		cniMetric.Metric = aitelemetry.Metric{
			Name:             telemetry.CNIDelTimeMetricStr,
			Value:            float64(operationTimeMs),
			CustomDimensions: make(map[string]string),
		}
		SetCustomDimensions(&cniMetric, nwCfg, err)
		telemetry.SendCNIMetric(&cniMetric, plugin.tb)
	}

	log.Printf("Execution mode :%s", nwCfg.ExecutionMode)
	if nwCfg.ExecutionMode == string(Baremetal) {

		log.Printf("Baremetal mode. Calling vnet agent for delete container")

		// schedule send metric before attempting delete
		defer sendMetricFunc()
		err, _ = plugin.nnsClient.DeleteContainerNetworking(context.Background(), k8sPodName, args.Netns)
		return err
	}

	if nwCfg.MultiTenancy {
		// Initialize CNSClient
		cnsclient.InitCnsClient(nwCfg.CNSUrl, defaultRequestTimeout)
	}

	switch nwCfg.Ipam.Type {
	case network.AzureCNS:
		plugin.ipamInvoker, err = NewCNSInvoker(k8sPodName, k8sNamespace)
		if err != nil {
			log.Printf("[cni-net] Creating network %v failed with err %v.", networkId, err)
			return err
		}
	default:
		plugin.ipamInvoker = NewAzureIpamInvoker(plugin, &nwInfo)
	}

	// Initialize values from network config.
	networkId, err = getNetworkName(k8sPodName, k8sNamespace, args.IfName, nwCfg)

	// If error is not found error, then we ignore it, to comply with CNI SPEC.
	if err != nil {
		log.Printf("[cni-net] Failed to extract network name from network config. error: %v", err)

		if !utils.IsNotFoundError(err) {
			err = plugin.Errorf("Failed to extract network name from network config. error: %v", err)
			return err
		}
	}

	endpointId := GetEndpointID(args)

	// Query the network.
	if nwInfo, err = plugin.nm.GetNetworkInfo(networkId); err != nil {

		if !nwCfg.MultiTenancy {
			// attempt to release address associated with this Endpoint id
			// This is to ensure clean up is done even in failure cases
			err = plugin.ipamInvoker.Delete(nil, nwCfg, args, nwInfo.Options)
			if err != nil {
				log.Printf("Network not found, attempted to release address with error:  %v", err)
			}
		}

		// Log the error but return success if the endpoint being deleted is not found.
		plugin.Errorf("[cni-net] Failed to query network: %v", err)
		err = nil
		return err
	}

	// Query the endpoint.
	if epInfo, err = plugin.nm.GetEndpointInfo(networkId, endpointId); err != nil {

		if !nwCfg.MultiTenancy {
			// attempt to release address associated with this Endpoint id
			// This is to ensure clean up is done even in failure cases
			log.Printf("release ip ep not found")
			if err = plugin.ipamInvoker.Delete(nil, nwCfg, args, nwInfo.Options); err != nil {
				log.Printf("Endpoint not found, attempted to release address with error: %v", err)
			}
		}

		// Log the error but return success if the endpoint being deleted is not found.
		plugin.Errorf("[cni-net] Failed to query endpoint: %v", err)
		err = nil
		return err
	}

	// schedule send metric before attempting delete
	defer sendMetricFunc()
	// Delete the endpoint.
	if err = plugin.nm.DeleteEndpoint(networkId, endpointId); err != nil {
		err = plugin.Errorf("Failed to delete endpoint: %v", err)
		return err
	}

	if !nwCfg.MultiTenancy {
		// Call into IPAM plugin to release the endpoint's addresses.
		for _, address := range epInfo.IPAddresses {
			log.Printf("release ip:%s", address.IP.String())
			err = plugin.ipamInvoker.Delete(&address, nwCfg, args, nwInfo.Options)
			if err != nil {
				err = plugin.Errorf("Failed to release address %v with error: %v", address, err)
				return err
			}
		}
	} else if epInfo.EnableInfraVnet {
		nwCfg.Ipam.Subnet = nwInfo.Subnets[0].Prefix.String()
		nwCfg.Ipam.Address = epInfo.InfraVnetIP.IP.String()
		err = plugin.ipamInvoker.Delete(nil, nwCfg, args, nwInfo.Options)
		if err != nil {
			log.Printf("Failed to release address: %v", err)
			err = plugin.Errorf("Failed to release address %v with error: %v", nwCfg.Ipam.Address, err)
		}
	}

	msg = fmt.Sprintf("CNI DEL succeeded : Released ip %+v podname %v namespace %v", nwCfg.Ipam.Address, k8sPodName, k8sNamespace)
	plugin.setCNIReportDetails(nwCfg, CNI_DEL, msg)

	return err
}

// Update handles CNI update commands.
// Update is only supported for multitenancy and to update routes.
func (plugin *netPlugin) Update(args *cniSkel.CmdArgs) error {
	var (
		result              *cniTypesCurr.Result
		err                 error
		nwCfg               *cni.NetworkConfig
		existingEpInfo      *network.EndpointInfo
		podCfg              *cni.K8SPodEnvArgs
		cnsClient           *cnsclient.CNSClient
		orchestratorContext []byte
		targetNetworkConfig *cns.GetNetworkContainerResponse
		cniMetric           telemetry.AIMetric
	)

	startTime := time.Now()

	log.Printf("[cni-net] Processing UPDATE command with args {Netns:%v Args:%v Path:%v}.",
		args.Netns, args.Args, args.Path)

	// Parse network configuration from stdin.
	if nwCfg, err = cni.ParseNetworkConfig(args.StdinData); err != nil {
		err = plugin.Errorf("Failed to parse network configuration: %v.", err)
		return err
	}

	log.Printf("[cni-net] Read network configuration %+v.", nwCfg)

	iptables.DisableIPTableLock = nwCfg.DisableIPTableLock
	plugin.setCNIReportDetails(nwCfg, CNI_UPDATE, "")

	defer func() {
		operationTimeMs := time.Since(startTime).Milliseconds()
		cniMetric.Metric = aitelemetry.Metric{
			Name:             telemetry.CNIUpdateTimeMetricStr,
			Value:            float64(operationTimeMs),
			CustomDimensions: make(map[string]string),
		}
		SetCustomDimensions(&cniMetric, nwCfg, err)
		telemetry.SendCNIMetric(&cniMetric, plugin.tb)

		if result == nil {
			result = &cniTypesCurr.Result{}
		}

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

		log.Printf("[cni-net] UPDATE command completed with result:%+v err:%v.", result, err)
	}()

	// Parse Pod arguments.
	if podCfg, err = cni.ParseCniArgs(args.Args); err != nil {
		log.Printf("[cni-net] Error while parsing CNI Args during UPDATE %v", err)
		return err
	}

	k8sNamespace := string(podCfg.K8S_POD_NAMESPACE)
	if len(k8sNamespace) == 0 {
		errMsg := "Required parameter Pod Namespace not specified in CNI Args during UPDATE"
		log.Printf(errMsg)
		return plugin.Errorf(errMsg)
	}

	k8sPodName := string(podCfg.K8S_POD_NAME)
	if len(k8sPodName) == 0 {
		errMsg := "Required parameter Pod Name not specified in CNI Args during UPDATE"
		log.Printf(errMsg)
		return plugin.Errorf(errMsg)
	}

	// Initialize values from network config.
	networkID := nwCfg.Name

	// Query the network.
	if _, err = plugin.nm.GetNetworkInfo(networkID); err != nil {
		errMsg := fmt.Sprintf("Failed to query network during CNI UPDATE: %v", err)
		log.Printf(errMsg)
		return plugin.Errorf(errMsg)
	}

	// Query the existing endpoint since this is an update.
	// Right now, we do not support updating pods that have multiple endpoints.
	existingEpInfo, err = plugin.nm.GetEndpointInfoBasedOnPODDetails(networkID, k8sPodName, k8sNamespace, nwCfg.EnableExactMatchForPodName)
	if err != nil {
		plugin.Errorf("Failed to retrieve target endpoint for CNI UPDATE [name=%v, namespace=%v]: %v", k8sPodName, k8sNamespace, err)
		return err
	}

	log.Printf("Retrieved existing endpoint from state that may get update: %+v", existingEpInfo)

	// now query CNS to get the target routes that should be there in the networknamespace (as a result of update)
	log.Printf("Going to collect target routes for [name=%v, namespace=%v] from CNS.", k8sPodName, k8sNamespace)
	if cnsClient, err = cnsclient.InitCnsClient(nwCfg.CNSUrl, defaultRequestTimeout); err != nil {
		log.Printf("Initializing CNS client error in CNI Update%v", err)
		log.Printf(err.Error())
		return plugin.Errorf(err.Error())
	}

	// create struct with info for target POD
	podInfo := cns.KubernetesPodInfo{
		PodName:      k8sPodName,
		PodNamespace: k8sNamespace,
	}
	if orchestratorContext, err = json.Marshal(podInfo); err != nil {
		log.Printf("Marshalling KubernetesPodInfo failed with %v", err)
		return plugin.Errorf(err.Error())
	}

	if targetNetworkConfig, err = cnsClient.GetNetworkConfiguration(orchestratorContext); err != nil {
		log.Printf("GetNetworkConfiguration failed with %v", err)
		return plugin.Errorf(err.Error())
	}

	log.Printf("Network config received from cns for [name=%v, namespace=%v] is as follows -> %+v", k8sPodName, k8sNamespace, targetNetworkConfig)
	targetEpInfo := &network.EndpointInfo{}

	// get the target routes that should replace existingEpInfo.Routes inside the network namespace
	log.Printf("Going to collect target routes for [name=%v, namespace=%v] from targetNetworkConfig.", k8sPodName, k8sNamespace)
	if targetNetworkConfig.Routes != nil && len(targetNetworkConfig.Routes) > 0 {
		for _, route := range targetNetworkConfig.Routes {
			log.Printf("Adding route from routes to targetEpInfo %+v", route)
			_, dstIPNet, _ := net.ParseCIDR(route.IPAddress)
			gwIP := net.ParseIP(route.GatewayIPAddress)
			targetEpInfo.Routes = append(targetEpInfo.Routes, network.RouteInfo{Dst: *dstIPNet, Gw: gwIP, DevName: existingEpInfo.IfName})
			log.Printf("Successfully added route from routes to targetEpInfo %+v", route)
		}
	}

	log.Printf("Going to collect target routes based on Cnetaddressspace for [name=%v, namespace=%v] from targetNetworkConfig.", k8sPodName, k8sNamespace)
	ipconfig := targetNetworkConfig.IPConfiguration
	for _, ipRouteSubnet := range targetNetworkConfig.CnetAddressSpace {
		log.Printf("Adding route from cnetAddressspace to targetEpInfo %+v", ipRouteSubnet)
		dstIPNet := net.IPNet{IP: net.ParseIP(ipRouteSubnet.IPAddress), Mask: net.CIDRMask(int(ipRouteSubnet.PrefixLength), 32)}
		gwIP := net.ParseIP(ipconfig.GatewayIPAddress)
		route := network.RouteInfo{Dst: dstIPNet, Gw: gwIP, DevName: existingEpInfo.IfName}
		targetEpInfo.Routes = append(targetEpInfo.Routes, route)
		log.Printf("Successfully added route from cnetAddressspace to targetEpInfo %+v", ipRouteSubnet)
	}

	log.Printf("Finished collecting new routes in targetEpInfo as follows: %+v", targetEpInfo.Routes)
	log.Printf("Now saving existing infravnetaddress space if needed.")
	for _, ns := range nwCfg.PodNamespaceForDualNetwork {
		if k8sNamespace == ns {
			targetEpInfo.EnableInfraVnet = true
			targetEpInfo.InfraVnetAddressSpace = nwCfg.InfraVnetAddressSpace
			log.Printf("Saving infravnet address space %s for [%s-%s]",
				targetEpInfo.InfraVnetAddressSpace, existingEpInfo.PODNameSpace, existingEpInfo.PODName)
			break
		}
	}

	// Update the endpoint.
	log.Printf("Now updating existing endpoint %v with targetNetworkConfig %+v.", existingEpInfo.Id, targetNetworkConfig)
	if err = plugin.nm.UpdateEndpoint(networkID, existingEpInfo, targetEpInfo); err != nil {
		err = plugin.Errorf("Failed to update endpoint: %v", err)
		return err
	}

	msg := fmt.Sprintf("CNI UPDATE succeeded : Updated %+v podname %v namespace %v", targetNetworkConfig, k8sPodName, k8sNamespace)
	plugin.setCNIReportDetails(nwCfg, CNI_UPDATE, msg)

	return nil
}

// Temporary function to determine whether we need to disable SNAT due to NMAgent support
func determineSnat() (bool, bool, error) {
	var (
		snatConfig            snatConfiguration
		retrieveSnatConfigErr error
		jsonFile              *os.File
		httpClient            = &http.Client{Timeout: time.Second * 5}
		snatConfigFile        = snatConfigFileName + jsonFileExtension
	)

	// Check if we've already retrieved NMAgent version and determined whether to disable snat on host
	if jsonFile, retrieveSnatConfigErr = os.Open(snatConfigFile); retrieveSnatConfigErr == nil {
		bytes, _ := ioutil.ReadAll(jsonFile)
		jsonFile.Close()
		if retrieveSnatConfigErr = json.Unmarshal(bytes, &snatConfig); retrieveSnatConfigErr != nil {
			log.Errorf("[cni-net] failed to unmarshal to snatConfig with error %v",
				retrieveSnatConfigErr)
		}
	}

	// If we weren't able to retrieve snatConfiguration, query NMAgent
	if retrieveSnatConfigErr != nil {
		var resp *http.Response
		resp, retrieveSnatConfigErr = httpClient.Get(nmAgentSupportedApisURL)
		if retrieveSnatConfigErr == nil {
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				var bodyBytes []byte
				// if the list of APIs (strings) contains the nmAgentSnatSupportAPI we will disable snat on host
				if bodyBytes, retrieveSnatConfigErr = ioutil.ReadAll(resp.Body); retrieveSnatConfigErr == nil {
					bodyStr := string(bodyBytes)
					if !strings.Contains(bodyStr, nmAgentSnatAndDnsSupportAPI) {
						snatConfig.EnableSnatForDns = true
						snatConfig.EnableSnatOnHost = !strings.Contains(bodyStr, nmAgentSnatSupportAPI)
					}

					jsonStr, _ := json.Marshal(snatConfig)
					fp, err := os.OpenFile(snatConfigFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(0664))
					if err == nil {
						fp.Write(jsonStr)
						fp.Close()
					} else {
						log.Errorf("[cni-net] failed to save snat settings to %s with error: %+v", snatConfigFile, err)
					}
				}
			} else {
				retrieveSnatConfigErr = fmt.Errorf("nmagent request status code %d", resp.StatusCode)
			}
		}
	}

	// Log and return the error when we fail acquire snat configuration for host and dns
	if retrieveSnatConfigErr != nil {
		log.Errorf("[cni-net] failed to acquire SNAT configuration with error %v",
			retrieveSnatConfigErr)
		return snatConfig.EnableSnatForDns, snatConfig.EnableSnatOnHost, retrieveSnatConfigErr
	}

	log.Printf("[cni-net] saved snat settings %+v to %s", snatConfig, snatConfigFile)
	if snatConfig.EnableSnatOnHost {
		log.Printf("[cni-net] enabling SNAT on container host for outbound connectivity")
	} else if snatConfig.EnableSnatForDns {
		log.Printf("[cni-net] enabling SNAT on container host for DNS traffic")
	} else {
		log.Printf("[cni-net] disabling SNAT on container host")
	}

	return snatConfig.EnableSnatForDns, snatConfig.EnableSnatOnHost, nil
}

func convertNnsToCniResult(
	netRes *nnscontracts.ConfigureContainerNetworkingResponse,
	ifName string,
	podName string,
	operationName string) *cniTypesCurr.Result {

	// This function does not add interfaces to CNI result. Reason being CRI (containerD in baremetal case)
	// only looks for default interface named "eth0" and this default interface is added in the defer
	// method of ADD method
	result := &cniTypesCurr.Result{}
	var resultIpconfigs []*cniTypesCurr.IPConfig

	if netRes.Interfaces != nil {
		for i, ni := range netRes.Interfaces {

			intIndex := i
			for _, ip := range ni.Ipaddresses {

				ipWithPrefix := fmt.Sprintf("%s/%s", ip.Ip, ip.PrefixLength)
				_, ipNet, err := net.ParseCIDR(ipWithPrefix)
				if err != nil {
					log.Printf("Error while converting to cni result for %s operation on pod %s. %s",
						operationName, podName, err)
					continue
				}

				gateway := net.ParseIP(ip.DefaultGateway)
				ipConfig := &cniTypesCurr.IPConfig{
					Address:   *ipNet,
					Gateway:   gateway,
					Version:   ip.Version,
					Interface: &intIndex,
				}

				resultIpconfigs = append(resultIpconfigs, ipConfig)
			}
		}
	}

	result.IPs = resultIpconfigs

	return result
}
