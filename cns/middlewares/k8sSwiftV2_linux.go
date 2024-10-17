package middlewares

import (
	"fmt"
	"net/netip"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/middlewares/utils"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/pkg/errors"
)

// setRoutes sets the routes for podIPInfo used in SWIFT V2 scenario.
func (k *K8sSWIFTv2Middleware) setRoutes(podIPInfo *cns.PodIpInfo) error {
	logger.Printf("[SWIFTv2Middleware] set routes for pod with nic type : %s", podIPInfo.NICType)
	var routes []cns.Route

	switch podIPInfo.NICType {
	// TODO: We may want to create a new type of NIC. Currently we are using delegated NICs but this set routes method only
	// takes in for the multitenant scenario. We should have a new case that behaves similarly to InfraNIC but for a delegated NIC
	// Q: Does the code currently see the kube pods as Infra or Delegated? They currently use the delegated pod subnet for IPs
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
		// Get and parse infraVNETCIDRs from env
		infraVNETCIDRs, err := configuration.InfraVNETCIDRs()
		if err != nil {
			return errors.Wrapf(err, "failed to get infraVNETCIDRs from env")
		}
		infraVNETCIDRsv4, infraVNETCIDRsv6, err := utils.ParseCIDRs(infraVNETCIDRs)
		if err != nil {
			return errors.Wrapf(err, "failed to parse infraVNETCIDRs")
		}

		// Get and parse podCIDRs from env
		podCIDRs, err := configuration.PodCIDRs()
		if err != nil {
			return errors.Wrapf(err, "failed to get podCIDRs from env")
		}
		podCIDRsV4, podCIDRv6, err := utils.ParseCIDRs(podCIDRs)
		if err != nil {
			return errors.Wrapf(err, "failed to parse podCIDRs")
		}

		// Get and parse serviceCIDRs from env
		serviceCIDRs, err := configuration.ServiceCIDRs()
		if err != nil {
			return errors.Wrapf(err, "failed to get serviceCIDRs from env")
		}
		serviceCIDRsV4, serviceCIDRsV6, err := utils.ParseCIDRs(serviceCIDRs)
		if err != nil {
			return errors.Wrapf(err, "failed to parse serviceCIDRs")
		}

		ip, err := netip.ParseAddr(podIPInfo.PodIPConfig.IPAddress)
		if err != nil {
			return errors.Wrapf(err, "failed to parse podIPConfig IP address %s", podIPInfo.PodIPConfig.IPAddress)
		}

		//This function is called per IP so we shouldn't have to worry about adding both v4 and v6 at once
		if ip.Is4() {
			routes = append(routes, addRoutes(podCIDRsV4, overlayGatewayv4)...)
			routes = append(routes, addRoutes(serviceCIDRsV4, overlayGatewayv4)...)
			routes = append(routes, addRoutes(infraVNETCIDRsv4, overlayGatewayv4)...)
		} else {
			routes = append(routes, addRoutes(podCIDRv6, overlayGatewayV6)...)
			routes = append(routes, addRoutes(serviceCIDRsV6, overlayGatewayV6)...)
			routes = append(routes, addRoutes(infraVNETCIDRsv6, overlayGatewayV6)...)
		}
		podIPInfo.SkipDefaultRoutes = true

	case cns.NodeNetworkInterfaceBackendNIC: //nolint:exhaustive // ignore exhaustive types check
		// No-op NIC types.
	default:
		return errInvalidSWIFTv2NICType
	}

	podIPInfo.Routes = routes
	return nil
}

func addRoutes(cidrs []string, gatewayIP string) []cns.Route {
	routes := make([]cns.Route, len(cidrs))
	for i, cidr := range cidrs {
		routes[i] = cns.Route{
			IPAddress:        cidr,
			GatewayIPAddress: gatewayIP,
		}
	}
	return routes
}

// assignSubnetPrefixLengthFields is a no-op for linux swiftv2 as the default prefix-length is sufficient
func (k *K8sSWIFTv2Middleware) assignSubnetPrefixLengthFields(_ *cns.PodIpInfo, _ v1alpha1.InterfaceInfo, _ string) error {
	return nil
}
