package middlewares

import (
	"net/netip"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/middlewares/utils"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/pkg/errors"
)

const (
	defaultGateway = "0.0.0.0"
)

// for AKS L1VH, do not set default route on infraNIC to avoid customer pod reaching all infra vnet services
// default route is set for secondary interface NIC(i.e,delegatedNIC)
func (k *K8sSWIFTv2Middleware) setRoutes(podIPInfo *cns.PodIpInfo) error {
	if podIPInfo.NICType == cns.InfraNIC {
		// as a workaround, HNS will not set this dummy default route(0.0.0.0/0, nexthop:0.0.0.0) on infraVnet interface eth0
		// the only usage for this dummy default is to bypass HNS setting default route on eth0
		// TODO: Remove this once HNS fix is ready
		route := cns.Route{
			IPAddress:        "0.0.0.0/0",
			GatewayIPAddress: defaultGateway,
		}
		podIPInfo.Routes = append(podIPInfo.Routes, route)

		// set routes(pod/node/service cidrs) for infraNIC interface
		// Swiftv2 Windows does not support IPv6
		infraRoutes, err := k.getInfraRoutes(podIPInfo)
		if err != nil {
			return errors.Wrap(err, "failed to set routes for infraNIC interface")
		}
		podIPInfo.Routes = append(podIPInfo.Routes, infraRoutes...)
		podIPInfo.SkipDefaultRoutes = true
	}
	return nil
}

// assignSubnetPrefixLengthFields will assign the subnet-prefix length to some fields of podipinfo
// this is required for the windows scenario so that HNS programming is successful for pods
func (k *K8sSWIFTv2Middleware) assignSubnetPrefixLengthFields(podIPInfo *cns.PodIpInfo, interfaceInfo v1alpha1.InterfaceInfo, ip string) error {
	// Parse MTPNC SubnetAddressSpace to get the subnet prefix length
	subnet, subnetPrefix, err := utils.ParseIPAndPrefix(interfaceInfo.SubnetAddressSpace)
	if err != nil {
		return errors.Wrap(err, "failed to parse mtpnc subnetAddressSpace prefix")
	}
	// assign the subnet-prefix length to all fields in podipinfo
	podIPInfo.PodIPConfig.PrefixLength = uint8(subnetPrefix)
	podIPInfo.HostPrimaryIPInfo = cns.HostIPInfo{
		Gateway:   interfaceInfo.GatewayIP,
		PrimaryIP: ip,
		Subnet:    interfaceInfo.SubnetAddressSpace,
	}
	podIPInfo.NetworkContainerPrimaryIPConfig = cns.IPConfiguration{
		IPSubnet: cns.IPSubnet{
			IPAddress:    subnet,
			PrefixLength: uint8(subnetPrefix),
		},
		GatewayIPAddress: interfaceInfo.GatewayIP,
	}
	return nil
}

// add default route with gateway IP to podIPInfo for delegated interface
func (k *K8sSWIFTv2Middleware) addDefaultRoute(podIPInfo *cns.PodIpInfo, gatewayIP string) {
	route := cns.Route{
		IPAddress:        "0.0.0.0/0",
		GatewayIPAddress: gatewayIP,
	}
	podIPInfo.Routes = append(podIPInfo.Routes, route)
}

// add routes to podIPInfo for the given CIDRs and gateway IP
// always use default gateway IP for containerd to configure routes;
// containerd will set route with default gateway ip like 10.0.0.0/16 via 0.0.0.0 dev eth0
func (k *K8sSWIFTv2Middleware) addRoutes(cidrs []string) []cns.Route {
	routes := make([]cns.Route, len(cidrs))
	for i, cidr := range cidrs {
		routes[i] = cns.Route{
			IPAddress:        cidr,
			GatewayIPAddress: defaultGateway,
		}
	}
	return routes
}

func (k *K8sSWIFTv2Middleware) getInfraRoutes(podIPInfo *cns.PodIpInfo) ([]cns.Route, error) {
	var routes []cns.Route

	ip, err := netip.ParseAddr(podIPInfo.PodIPConfig.IPAddress)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse podIPConfig IP address %s", podIPInfo.PodIPConfig.IPAddress)
	}

	v4IPs, v6IPs, err := k.GetCidrs()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get CIDRs")
	}

	if ip.Is4() {
		routes = append(routes, k.addRoutes(v4IPs)...)
	} else {
		routes = append(routes, k.addRoutes(v6IPs)...)
	}

	return routes, nil
}
