package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/log"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/client"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/network"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/100"
	"go.uber.org/zap"
)

const (
	filePerm    = 0o664
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
	GetAllNetworkContainers(
		ctx context.Context,
		nwCfg *cni.NetworkConfig,
		podName string,
		podNamespace string,
		ifName string) ([]IPAMAddResult, error)
	Init(cnsclient cnsclient, netioshim netioshim)
}

type Multitenancy struct {
	// cnsclient is used to communicate with CNS
	cnsclient cnsclient

	// netioshim is used to interact with networking syscalls
	netioshim netioshim
}

type netioshim interface {
	GetInterfaceSubnetWithSpecificIP(ipAddr string) *net.IPNet
}

type AzureNetIOShim struct{}

func (a AzureNetIOShim) GetInterfaceSubnetWithSpecificIP(ipAddr string) *net.IPNet {
	return common.GetInterfaceSubnetWithSpecificIP(ipAddr)
}

var errNmaResponse = errors.New("nmagent request status code")

func (m *Multitenancy) Init(cnsclient cnsclient, netioshim netioshim) {
	m.cnsclient = cnsclient
	m.netioshim = netioshim
}

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
		bytes, _ := io.ReadAll(jsonFile)
		jsonFile.Close()
		if retrieveSnatConfigErr = json.Unmarshal(bytes, &snatConfig); retrieveSnatConfigErr != nil {
			log.Logger.Error("failed to unmarshal to snatConfig with error %v",
				zap.Error(retrieveSnatConfigErr))
		}
	}

	// If we weren't able to retrieve snatConfiguration, query NMAgent
	if retrieveSnatConfigErr != nil {
		var resp *http.Response
		req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, nmAgentSupportedApisURL, nil)
		if err != nil {
			log.Logger.Error("failed creating http request", zap.Error(err))
			return false, false, fmt.Errorf("%w", err)
		}
		log.Logger.Info("Query nma for dns snat support", zap.String("query", nmAgentSupportedApisURL))
		resp, retrieveSnatConfigErr = httpClient.Do(req)
		if retrieveSnatConfigErr == nil {
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				var bodyBytes []byte
				// if the list of APIs (strings) contains the nmAgentSnatSupportAPI we will disable snat on host
				if bodyBytes, retrieveSnatConfigErr = io.ReadAll(resp.Body); retrieveSnatConfigErr == nil {
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
							log.Logger.Error("DetermineSnatFeatureOnHost: Write to json failed", zap.Error(err))
						}
						fp.Close()
					} else {
						log.Logger.Error("failed to save snat settings",
							zap.String("snatConfgFile", snatConfigFile),
							zap.Error(err))
					}
				}
			} else {
				retrieveSnatConfigErr = fmt.Errorf("%w:%d", errNmaResponse, resp.StatusCode)
			}
		}
	}

	// Log and return the error when we fail acquire snat configuration for host and dns
	if retrieveSnatConfigErr != nil {
		log.Logger.Error("failed to acquire SNAT configuration with error %v",
			zap.Error(retrieveSnatConfigErr))
		return snatConfig.EnableSnatForDns, snatConfig.EnableSnatOnHost, retrieveSnatConfigErr
	}

	log.Logger.Info("saved snat settings",
		zap.Any("snatConfig", snatConfig),
		zap.String("snatConfigfile", snatConfigFile))
	if snatConfig.EnableSnatOnHost {
		log.Logger.Info("enabling SNAT on container host for outbound connectivity")
	}
	if snatConfig.EnableSnatForDns {
		log.Logger.Info("enabling SNAT on container host for DNS traffic")
	}
	if !snatConfig.EnableSnatForDns && !snatConfig.EnableSnatOnHost {
		log.Logger.Info("disabling SNAT on container host")
	}

	return snatConfig.EnableSnatForDns, snatConfig.EnableSnatOnHost, nil
}

func (m *Multitenancy) SetupRoutingForMultitenancy(
	nwCfg *cni.NetworkConfig,
	cnsNetworkConfig *cns.GetNetworkContainerResponse,
	azIpamResult *cniTypesCurr.Result,
	epInfo *network.EndpointInfo,
	result *cniTypesCurr.Result,
) {
	// Adding default gateway
	// if snat enabled, add 169.254.128.1 as default gateway
	if nwCfg.EnableSnatOnHost {
		log.Logger.Info("add default route for multitenancy.snat on host enabled")
		addDefaultRoute(cnsNetworkConfig.LocalIPConfiguration.GatewayIPAddress, epInfo, result)
	} else {
		_, defaultIPNet, _ := net.ParseCIDR("0.0.0.0/0")
		dstIP := net.IPNet{IP: net.ParseIP("0.0.0.0"), Mask: defaultIPNet.Mask}
		gwIP := net.ParseIP(cnsNetworkConfig.IPConfiguration.GatewayIPAddress)
		epInfo.Routes = append(epInfo.Routes, network.RouteInfo{Dst: dstIP, Gw: gwIP})
		result.Routes = append(result.Routes, &cniTypes.Route{Dst: dstIP, GW: gwIP})

		if epInfo.EnableSnatForDns {
			log.Logger.Info("add SNAT for DNS enabled")
			addSnatForDNS(cnsNetworkConfig.LocalIPConfiguration.GatewayIPAddress, epInfo, result)
		}
	}

	setupInfraVnetRoutingForMultitenancy(nwCfg, azIpamResult, epInfo, result)
}

// get all network container configuration(s) for given orchestratorContext
func (m *Multitenancy) GetAllNetworkContainers(
	ctx context.Context, nwCfg *cni.NetworkConfig, podName, podNamespace, ifName string,
) ([]IPAMAddResult, error) {
	var podNameWithoutSuffix string

	if !nwCfg.EnableExactMatchForPodName {
		podNameWithoutSuffix = network.GetPodNameWithoutSuffix(podName)
	} else {
		podNameWithoutSuffix = podName
	}

	log.Logger.Info("Podname without suffix", zap.String("podName", podNameWithoutSuffix))

	ncResponses, hostSubnetPrefixes, err := m.getNetworkContainersInternal(ctx, podNamespace, podNameWithoutSuffix)
	if err != nil {
		return []IPAMAddResult{}, fmt.Errorf("%w", err)
	}

	for i := 0; i < len(ncResponses); i++ {
		if nwCfg.EnableSnatOnHost {
			if ncResponses[i].LocalIPConfiguration.IPSubnet.IPAddress == "" {
				log.Logger.Info("Snat IP is not populated for ncs. Got empty string",
					zap.Any("response", ncResponses))
				return []IPAMAddResult{}, errSnatIP
			}
		}
	}

	ipamResults := make([]IPAMAddResult, len(ncResponses))

	for i := 0; i < len(ncResponses); i++ {
		ipamResults[i].ncResponse = &ncResponses[i]
		ipamResults[i].hostSubnetPrefix = hostSubnetPrefixes[i]
		ipamResults[i].ipv4Result = convertToCniResult(ipamResults[i].ncResponse, ifName)
	}

	return ipamResults, err
}

// get all network containers configuration for given orchestratorContext
func (m *Multitenancy) getNetworkContainersInternal(
	ctx context.Context, namespace, podName string,
) ([]cns.GetNetworkContainerResponse, []net.IPNet, error) {
	podInfo := cns.KubernetesPodInfo{
		PodName:      podName,
		PodNamespace: namespace,
	}

	orchestratorContext, err := json.Marshal(podInfo)
	if err != nil {
		log.Logger.Error("Marshalling KubernetesPodInfo failed", zap.Error(err))
		return nil, []net.IPNet{}, fmt.Errorf("%w", err)
	}

	// First try the new CNS API that returns slice of nc responses. If CNS doesn't support the new API, an error will be returned and as a result
	// try using the old CNS API that returns single nc response.
	ncConfigs, err := m.cnsclient.GetAllNetworkContainers(ctx, orchestratorContext)
	if err != nil && client.IsUnsupportedAPI(err) {
		ncConfig, errGetNC := m.cnsclient.GetNetworkContainer(ctx, orchestratorContext)
		if errGetNC != nil {
			return nil, []net.IPNet{}, fmt.Errorf("%w", errGetNC)
		}
		ncConfigs = append(ncConfigs, *ncConfig)
	} else if err != nil {
		return nil, []net.IPNet{}, fmt.Errorf("%w", err)
	}

	log.Logger.Info("Network config received from cns", zap.Any("nconfig", ncConfigs))

	subnetPrefixes := []net.IPNet{}
	for i := 0; i < len(ncConfigs); i++ {
		subnetPrefix := m.netioshim.GetInterfaceSubnetWithSpecificIP(ncConfigs[i].PrimaryInterfaceIdentifier)
		if subnetPrefix == nil {
			log.Logger.Error(errIfaceNotFound.Error(),
				zap.String("nodeIP", ncConfigs[i].PrimaryInterfaceIdentifier))
			return nil, []net.IPNet{}, errIfaceNotFound
		}
		subnetPrefixes = append(subnetPrefixes, *subnetPrefix)
	}

	return ncConfigs, subnetPrefixes, nil
}

func convertToCniResult(networkConfig *cns.GetNetworkContainerResponse, ifName string) *cniTypesCurr.Result {
	result := &cniTypesCurr.Result{}
	resultIpconfig := &cniTypesCurr.IPConfig{}

	ipconfig := networkConfig.IPConfiguration
	ipAddr := net.ParseIP(ipconfig.IPSubnet.IPAddress)

	if ipAddr.To4() != nil {
		resultIpconfig.Address = net.IPNet{IP: ipAddr, Mask: net.CIDRMask(int(ipconfig.IPSubnet.PrefixLength), 32)}
	} else {
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

	for _, ipRouteSubnet := range networkConfig.CnetAddressSpace {
		routeIPnet := net.IPNet{IP: net.ParseIP(ipRouteSubnet.IPAddress), Mask: net.CIDRMask(int(ipRouteSubnet.PrefixLength), 32)}
		gwIP := net.ParseIP(ipconfig.GatewayIPAddress)
		result.Routes = append(result.Routes, &cniTypes.Route{Dst: routeIPnet, GW: gwIP})
	}

	iface := &cniTypesCurr.Interface{Name: ifName}
	result.Interfaces = append(result.Interfaces, iface)

	return result
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
	errIfaceNotFound = errors.New("Interface not found for this ip")
)
