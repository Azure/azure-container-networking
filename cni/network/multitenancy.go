package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	cnscli "github.com/Azure/azure-container-networking/cns/client"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
)

const (
	filePerm    = 0664
	httpTimeout = 5
)

// MultitenancyClient interface
type MultitenancyClient interface {
	SetupRoutingForMultitenancy(
		nwCfg *cni.NetworkConfig,
		cnsNetworkConfig *cns.GetNetworkContainerResponse,
		azIpamResult *cniTypesCurr.Result,
		epInfo *network.EndpointInfo,
		result *cniTypesCurr.Result)
	DetermineSnatFeatureOnHost(
		snatFile string,
		nmAgentSupportedApisURL string) (bool, bool, error)

	GetContainerNetworkConfiguration(
		ctx context.Context,
		nwCfg *cni.NetworkConfig,
		podName string,
		podNamespace string,
		ifName string) (*cniTypesCurr.Result, *cns.GetNetworkContainerResponse, net.IPNet, error)
}

type Multitenancy struct{}

var errNmaResponse = errors.New("nmagent request status code")

// DetermineSnatFeatureOnHost - Temporary function to determine whether we need to disable SNAT due to NMAgent support
func (m *Multitenancy) DetermineSnatFeatureOnHost(snatFile, nmAgentSupportedApisURL string) (snatForDNS, snatOnHost bool, err error) {
	var (
		snatConfig            snatConfiguration
		retrieveSnatConfigErr error
		jsonFile              *os.File
		httpClient            = &http.Client{Timeout: time.Second * httpTimeout}
		snatConfigFile        = snatConfigFileName + jsonFileExtension
	)

	// Check if we've already retrieved NMAgent version and determined whether to disable snat on host
	if jsonFile, retrieveSnatConfigErr = os.Open(snatFile); retrieveSnatConfigErr == nil {
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
		req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, nmAgentSnatAndDnsSupportAPI, nil)
		if err != nil {
			log.Errorf("failed creating http request:%+v", err)
			return false, false, fmt.Errorf("%w", err)
		}
		resp, retrieveSnatConfigErr = httpClient.Do(req)
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
					fp, err := os.OpenFile(snatConfigFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(filePerm))
					if err == nil {
						_, err = fp.Write(jsonStr)
						if err != nil {
							log.Errorf("DetermineSnatFeatureOnHost: Write to json failed:%+v", err)
						}
						fp.Close()
					} else {
						log.Errorf("[cni-net] failed to save snat settings to %s with error: %+v", snatConfigFile, err)
					}
				}
			} else {
				retrieveSnatConfigErr = fmt.Errorf("%w:%d", errNmaResponse, resp.StatusCode)
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
	}
	if snatConfig.EnableSnatForDns {
		log.Printf("[cni-net] enabling SNAT on container host for DNS traffic")
	}
	if !snatConfig.EnableSnatForDns && !snatConfig.EnableSnatOnHost {
		log.Printf("[cni-net] disabling SNAT on container host")
	}

	return snatConfig.EnableSnatForDns, snatConfig.EnableSnatOnHost, nil
}

func (m *Multitenancy) SetupRoutingForMultitenancy(
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

func (m *Multitenancy) GetContainerNetworkConfiguration(
	ctx context.Context, nwCfg *cni.NetworkConfig, podName string, podNamespace string, ifName string) (*cniTypesCurr.Result, *cns.GetNetworkContainerResponse, net.IPNet, error) {
	var podNameWithoutSuffix string

	if !nwCfg.EnableExactMatchForPodName {
		podNameWithoutSuffix = network.GetPodNameWithoutSuffix(podName)
	} else {
		podNameWithoutSuffix = podName
	}

	log.Printf("Podname without suffix %v", podNameWithoutSuffix)
	return getContainerNetworkConfigurationInternal(ctx, nwCfg.CNSUrl, podNamespace, podNameWithoutSuffix, ifName)
}

func getContainerNetworkConfigurationInternal(
	ctx context.Context, cnsURL string, namespace string, podName string, ifName string) (*cniTypesCurr.Result, *cns.GetNetworkContainerResponse, net.IPNet, error) {
	client, err := cnscli.New(cnsURL, cnscli.DefaultTimeout)
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

	networkConfig, err := client.GetNetworkConfiguration(ctx, orchestratorContext)
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
	plugin *NetPlugin,
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
	plugin *NetPlugin) {

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

var (
	errSnatIP        = errors.New("Snat IP not populated")
	errInfraVnet     = errors.New("infravnet not populated")
	errSubnetOverlap = errors.New("subnet overlap error")
)

// GetMultiTenancyCNIResult retrieves network goal state of a container from CNS
func (plugin *NetPlugin) GetMultiTenancyCNIResult(
	ctx context.Context,
	enableInfraVnet bool,
	nwCfg *cni.NetworkConfig,
	k8sPodName string,
	k8sNamespace string,
	ifName string) (res *cniTypesCurr.Result, resp *cns.GetNetworkContainerResponse, prefix net.IPNet, infraRes *cniTypesCurr.Result, err error) {

	result, cnsNetworkConfig, subnetPrefix, err := plugin.multitenancyClient.GetContainerNetworkConfiguration(ctx, nwCfg, k8sPodName, k8sNamespace, ifName)
	if err != nil {
		log.Printf("GetContainerNetworkConfiguration failed for podname %v namespace %v with error %v", k8sPodName, k8sNamespace, err)
		return nil, nil, net.IPNet{}, nil, fmt.Errorf("%w", err)
	}

	log.Printf("PrimaryInterfaceIdentifier :%v", subnetPrefix.IP.String())

	if checkIfSubnetOverlaps(enableInfraVnet, nwCfg, cnsNetworkConfig) {
		buf := fmt.Sprintf("InfraVnet %v overlaps with customerVnet %+v", nwCfg.InfraVnetAddressSpace, cnsNetworkConfig.CnetAddressSpace)
		log.Printf(buf)
		return nil, nil, net.IPNet{}, nil, errSubnetOverlap
	}

	if nwCfg.EnableSnatOnHost {
		if cnsNetworkConfig.LocalIPConfiguration.IPSubnet.IPAddress == "" {
			log.Printf("Snat IP is not populated. Got empty string")
			return nil, nil, net.IPNet{}, nil, errSnatIP
		}
	}

	if enableInfraVnet {
		if nwCfg.InfraVnetAddressSpace == "" {
			log.Printf("InfraVnetAddressSpace is not populated. Got empty string")
			return nil, nil, net.IPNet{}, nil, errInfraVnet
		}
	}

	azIpamResult, err := getInfraVnetIP(enableInfraVnet, subnetPrefix.String(), nwCfg, plugin)
	if err != nil {
		log.Printf("GetInfraVnetIP failed with error %v", err)
		return nil, nil, net.IPNet{}, nil, err
	}

	return result, cnsNetworkConfig, subnetPrefix, azIpamResult, nil
}

func CleanupMultitenancyResources(enableInfraVnet bool, nwCfg *cni.NetworkConfig, azIpamResult *cniTypesCurr.Result, plugin *NetPlugin) {
	if azIpamResult != nil && azIpamResult.IPs != nil {
		cleanupInfraVnetIP(enableInfraVnet, &azIpamResult.IPs[0].Address, nwCfg, plugin)
	}
}
