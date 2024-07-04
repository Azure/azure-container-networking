package middlewares

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/middlewares/utils"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/pkg/errors"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

// getIPConfig returns the pod's SWIFT V2 IP configuration.
func (k *K8sSWIFTv2Middleware) getIPConfig(ctx context.Context, podInfo cns.PodInfo) ([]cns.PodIpInfo, error) {
	// Check if the MTPNC CRD exists for the pod, if not, return error
	mtpnc := v1alpha1.MultitenantPodNetworkConfig{}
	mtpncNamespacedName := k8stypes.NamespacedName{Namespace: podInfo.Namespace(), Name: podInfo.Name()}
	if err := k.Cli.Get(ctx, mtpncNamespacedName, &mtpnc); err != nil {
		return nil, errors.Wrapf(err, "failed to get pod's mtpnc from cache")
	}

	// Check if the MTPNC CRD is ready. If one of the fields is empty, return error
	if !mtpnc.IsReady() {
		return nil, errMTPNCNotReady
	}
	logger.Printf("[SWIFTv2Middleware] mtpnc for pod %s is : %+v", podInfo.Name(), mtpnc)

	var podIPInfos []cns.PodIpInfo

	if len(mtpnc.Status.InterfaceInfos) == 0 {
		// Use fields from mtpnc.Status if InterfaceInfos is empty
		ip, prefixSize, err := utils.ParseIPAndPrefix(mtpnc.Status.PrimaryIP)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse mtpnc primary IP and prefix")
		}
		if prefixSize != prefixLength {
			return nil, errors.Wrapf(errInvalidMTPNCPrefixLength, "mtpnc primaryIP prefix length is %d", prefixSize)
		}

		podIPInfos = append(podIPInfos, cns.PodIpInfo{
			PodIPConfig: cns.IPSubnet{
				IPAddress:    ip,
				PrefixLength: uint8(prefixSize),
			},
			MacAddress:        mtpnc.Status.MacAddress,
			NICType:           cns.DelegatedVMNIC,
			SkipDefaultRoutes: false,
			// InterfaceName is empty for DelegatedVMNIC
		})
	} else {
		// Use InterfaceInfos if not empty
		podIPInfos = make([]cns.PodIpInfo, len(mtpnc.Status.InterfaceInfos))
		for i, interfaceInfo := range mtpnc.Status.InterfaceInfos {
			// Parse MTPNC primaryIP to get the IP address and prefix length
			ip, prefixSize, err := utils.ParseIPAndPrefix(interfaceInfo.PrimaryIP)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse mtpnc primary IP and prefix")
			}
			if prefixSize != prefixLength {
				return nil, errors.Wrapf(errInvalidMTPNCPrefixLength, "mtpnc primaryIP prefix length is %d", prefixSize)
			}

			var nicType cns.NICType
			switch {
			case interfaceInfo.DeviceType == v1alpha1.DeviceTypeVnetNIC && !interfaceInfo.AccelnetEnabled:
				nicType = cns.DelegatedVMNIC
			case interfaceInfo.DeviceType == v1alpha1.DeviceTypeVnetNIC && interfaceInfo.AccelnetEnabled:
				nicType = cns.NodeNetworkInterfaceAccelnetFrontendNIC
			case interfaceInfo.DeviceType == v1alpha1.DeviceTypeInfiniBandNIC:
				nicType = cns.NodeNetworkInterfaceBackendNIC
			default:
				nicType = cns.DelegatedVMNIC
			}

			podIPInfos[i] = cns.PodIpInfo{
				PodIPConfig: cns.IPSubnet{
					IPAddress:    ip,
					PrefixLength: uint8(prefixSize),
				},
				MacAddress:        interfaceInfo.MacAddress,
				NICType:           nicType,
				SkipDefaultRoutes: false,
				HostPrimaryIPInfo: cns.HostIPInfo{
					Gateway: interfaceInfo.GatewayIP,
				},
			}
		}
	}

	return podIPInfos, nil
}

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

	case cns.NodeNetworkInterfaceBackendNIC, cns.NodeNetworkInterfaceAccelnetFrontendNIC: //nolint:exhaustive // ignore exhaustive types check
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
