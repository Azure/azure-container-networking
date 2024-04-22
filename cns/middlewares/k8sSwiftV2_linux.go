package middlewares

import (
	"fmt"
	"net/netip"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/middlewares/utils"
	"github.com/pkg/errors"
)

// setRoutes sets the routes for podIPInfo used in SWIFT V2 scenario.
func (k *K8sSWIFTv2Middleware) setRoutes(podIPInfo *cns.PodIpInfo) error {
	logger.Printf("[SWIFTv2Middleware] set routes for pod with nic type : %s", podIPInfo.NICType)
	podIPInfo.Routes = []cns.Route{}
	switch podIPInfo.NICType {
	case cns.NodeNetworkInterfaceDelegatedVMNIC:
		virtualGWRoute := cns.Route{
			IPAddress: fmt.Sprintf("%s/%d", virtualGW, prefixLength),
		}
		// default route via SWIFT v2 interface
		route := cns.Route{
			IPAddress:        "0.0.0.0/0",
			GatewayIPAddress: virtualGW,
		}
		podIPInfo.Routes = []cns.Route{virtualGWRoute, route}
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
		// Check if the podIPInfo is IPv4 or IPv6
		ip, err := netip.ParseAddr(podIPInfo.PodIPConfig.IPAddress)
		if err != nil {
			return errors.Wrapf(err, "failed to parse podIPConfig IP address %s", podIPInfo.PodIPConfig.IPAddress)
		}
		if ip.Is4() {
			// routes for IPv4 podCIDR traffic
			addRoutes(&podIPInfo.Routes, podCIDRsV4, overlayGatewayv4)
			// route for IPv4 serviceCIDR traffic
			addRoutes(&podIPInfo.Routes, serviceCIDRsV4, overlayGatewayv4)
			// route for IPv4 infraVNETCIDR traffic
			addRoutes(&podIPInfo.Routes, infraVNETCIDRsv4, overlayGatewayv4)
		} else {
			// routes for IPv6 podCIDR traffic
			addRoutes(&podIPInfo.Routes, podCIDRv6, overlayGatewayV6)
			// route for IPv6 serviceCIDR traffic
			addRoutes(&podIPInfo.Routes, serviceCIDRsV6, overlayGatewayV6)
			// route for IPv6 infraVNETCIDR traffic
			addRoutes(&podIPInfo.Routes, infraVNETCIDRsv6, overlayGatewayV6)
		}
		podIPInfo.SkipDefaultRoutes = true
	case cns.NodeNetworkInterfaceBackendNIC:
		// TODO: Set routes for NodeNetworkInterfaceBackendNIC
	case cns.NodeNetworkInterfaceAccelnetFrontendNIC:
		// TODO: Set routes for NodeNetworkInterfaceAccelnetFrontendNIC
	default:
		return errInvalidSWIFTv2NICType
	}
	return nil
}

func addRoutes(routes *[]cns.Route, cidrs []string, gatewayIP string) {
	for _, cidr := range cidrs {
		route := cns.Route{
			IPAddress:        cidr,
			GatewayIPAddress: gatewayIP,
		}
		*routes = append(*routes, route)
	}
}
