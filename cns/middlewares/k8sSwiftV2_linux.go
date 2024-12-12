package middlewares

import (
	"fmt"
	"net/netip"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/pkg/errors"
)

// setRoutes sets the routes for podIPInfo used in SWIFT V2 scenario.
func (k *K8sSWIFTv2Middleware) setRoutes(podIPInfo *cns.PodIpInfo) error {
	logger.Printf("[SWIFTv2Middleware] set routes for pod with nic type : %s", podIPInfo.NICType)
	var routes []cns.Route

	switch podIPInfo.NICType {
	case cns.DelegatedVMNIC:
		virtualGWRoute := cns.Route{
			IPAddress: fmt.Sprintf("%s/%d", virtualGW, prefixLength),
		}
		// default route via SWIFT v2 interface
		route := cns.Route{
			IPAddress:        "0.0.0.0/0",
			GatewayIPAddress: virtualGW,
		}
		routes = append(routes, virtualGWRoute, route)

	case cns.InfraNIC:
		// Linux uses 169.254.1.1 as the default ipv4 gateway and fe80::1234:5678:9abc as the default ipv6 gateway
		infraRoutes, err := k.setInfraRoutes(podIPInfo)
		if err != nil {
			return errors.Wrap(err, "failed to set routes for infraNIC interface")
		}
		routes = infraRoutes
		podIPInfo.SkipDefaultRoutes = true

	case cns.NodeNetworkInterfaceBackendNIC: //nolint:exhaustive // ignore exhaustive types check
		// No-op NIC types.
	default:
		return errInvalidSWIFTv2NICType
	}

	podIPInfo.Routes = routes
	return nil
}

// assignSubnetPrefixLengthFields is a no-op for linux swiftv2 as the default prefix-length is sufficient
func (k *K8sSWIFTv2Middleware) assignSubnetPrefixLengthFields(_ *cns.PodIpInfo, _ v1alpha1.InterfaceInfo, _ string) error {
	return nil
}

func (k *K8sSWIFTv2Middleware) addDefaultRoute(*cns.PodIpInfo, string) {}

func (k *K8sSWIFTv2Middleware) addRoutes(cidrs []string, gatewayIP string) []cns.Route {
	routes := make([]cns.Route, len(cidrs))
	for i, cidr := range cidrs {
		routes[i] = cns.Route{
			IPAddress:        cidr,
			GatewayIPAddress: gatewayIP,
		}
	}
	return routes
}

func (k *K8sSWIFTv2Middleware) setInfraRoutes(podIPInfo *cns.PodIpInfo) ([]cns.Route, error) {
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
		routes = append(routes, k.addRoutes(v4IPs, overlayGatewayv4)...)
	} else {
		routes = append(routes, k.addRoutes(v6IPs, overlayGatewayV6)...)
	}

	return routes, nil
}
