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

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/api"
<<<<<<< Updated upstream
	"github.com/Azure/azure-container-networking/cns"
	cnsclient "github.com/Azure/azure-container-networking/cns/client"
=======
>>>>>>> Stashed changes
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/platform"
	nnscontracts "github.com/Azure/azure-container-networking/proto/nodenetworkservice/3.302.0.744"
	"github.com/Azure/azure-container-networking/store"
	"github.com/Azure/azure-container-networking/telemetry"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
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

	nl := netlink.NewNetlink()
	// Setup network manager.
	nm, err := network.NewNetworkManager(nl)
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

	// Initialize network manager. rehyrdration not required on reboot for cni plugin
	err = plugin.nm.Initialize(config, false)
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
					fp, err := os.OpenFile(snatConfigFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(0o664))
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
