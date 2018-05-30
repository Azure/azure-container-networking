package network

import (
	"net"

	"github.com/Azure/azure-container-networking/cni"
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
	hnsEndpoint, _ := hcsshim.GetHNSEndpointByName(endpointId)
	if hnsEndpoint != nil {
		log.Printf("[net] Found existing endpoint through hcsshim: %+v", hnsEndpoint)
		log.Printf("[net] Attaching ep %v to container %v", hnsEndpoint.Id, containerId)

		err := hcsshim.HotAttachEndpoint(containerId, hnsEndpoint.Id)
		if err != nil {
			log.Printf("[cni-net] Failed to hot attach shared endpoint to container [%s], err:%v.", hnsEndpoint.Id, err)
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

	return nil, nil
}
