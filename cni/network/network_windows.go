package network

import (
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Microsoft/hcsshim"

	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
)

/* handleConsecutiveAdd handles consecutive add calls for infrastructure containers on Windows platform.
 * This is a temporary work around for issue #57253 of Kubernetes.
 * We can delete this if statement once they fix it.
 * Issue link: https://github.com/kubernetes/kubernetes/issues/57253
 */
func handleConsecutiveAdd(containerId, endpointId string, nwInfo *network.NetworkInfo, nwCfg *cni.NetworkConfig) (*cniTypesCurr.Result, error) {
	hnsEndpoint, err := hcsshim.GetHNSEndpointByName(endpointId)
	if hnsEndpoint != nil {
		log.Printf("[net] Found existing endpoint through hcsshim: %+v", hnsEndpoint)
		log.Printf("[net] Attaching ep %v to container %v", hnsEndpoint.Id, containerId)

		err := hcsshim.HotAttachEndpoint(containerId, hnsEndpoint.Id)
		if err != nil {
			log.Printf("[cni-net] Failed to hot attach shared endpoint[%v] to container [%v], err:%v.", hnsEndpoint.Id, containerId, err)
			return nil, err
		}

		// Populate result.
		address := nwInfo.Subnets[0].Prefix
		address.IP = hnsEndpoint.IPAddress
		result := &cniTypesCurr.Result{
			IPs: []*cniTypesCurr.IPConfig{
				{
					Version: "4",
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

		// Populate DNS servers.
		result.DNS.Nameservers = nwCfg.DNS.Nameservers

		return result, nil
	}

	err = fmt.Errorf("GetHNSEndpointByName for %v returned nil with err %v", endpointId, err)
	return nil, err
}

func addDefaultRoute(gwIPString string, epInfo *network.EndpointInfo, result *cniTypesCurr.Result) {
}

func addInfraRoutes(azIpamResult *cniTypesCurr.Result, result *cniTypesCurr.Result, epInfo *network.EndpointInfo) {
}

func setNetworkOptions(cnsNwConfig *cns.GetNetworkContainerResponse, nwInfo *network.NetworkInfo) {
}

func setEndpointOptions(cnsNwConfig *cns.GetNetworkContainerResponse, epInfo *network.EndpointInfo, vethName string) {
}

func addSnatInterface(nwCfg *cni.NetworkConfig, result *cniTypesCurr.Result) {
}

func setupInfraVnetRoutingForMultitenancy(
	nwCfg *cni.NetworkConfig,
	azIpamResult *cniTypesCurr.Result,
	epInfo *network.EndpointInfo,
	result *cniTypesCurr.Result) {
}

func getNetworkDNSSettings(nwCfg *cni.NetworkConfig, result *cniTypesCurr.Result, namespace string) (network.DNSInfo, error) {
	var nwDNS network.DNSInfo

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

	if (len(nwCfg.DNS.Search) == 0) != (len(nwCfg.DNS.Nameservers) == 0) {
		err := fmt.Errorf("Wrong DNS configuration: %+v", nwCfg.DNS)
		return epDNS, err
	}

	if len(nwCfg.DNS.Search) > 0 {
		epDNS = network.DNSInfo{
			Servers: nwCfg.DNS.Nameservers,
			Suffix:  namespace + "." + strings.Join(nwCfg.DNS.Search, ","),
		}
	} else {
		epDNS = network.DNSInfo{
			Suffix:  result.DNS.Domain,
			Servers: result.DNS.Nameservers,
		}
	}

	return epDNS, nil
}
