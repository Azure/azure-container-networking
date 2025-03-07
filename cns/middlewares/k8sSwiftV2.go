package middlewares

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/middlewares/utils"
	"github.com/Azure/azure-container-networking/cns/types"
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
		// Linux CNS middleware sets the infra routes(pod, infravnet and service cidrs) to infraNIC interface for the podIPInfo used in SWIFT V2 Linux scenario
		infraRoutes, err := k.getInfraRoutes(podIPInfo)
		if err != nil {
			return errors.Wrap(err, "failed to get infra routes for infraNIC interface")
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

// Linux CNS gets pod CIDRs from configuration env
// Containerd reassigns the IP to the adapter and kernel configures the pod cidr route by default on Windows VM
// Hence the windows swiftv2 scenario does not require pod cidr
// GetPodCidrs() will return v4PodCidrs as first []string and v6PodCidrs as second []string
func (k *K8sSWIFTv2Middleware) GetPodCidrs() ([]string, []string, error) { //nolint
	v4PodCidrs := []string{}
	v6PodCidrs := []string{}

	// Get and parse podCIDRs from env
	podCIDRs, err := configuration.PodCIDRs()
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to get podCIDRs from env")
	}
	podCIDRsV4, podCIDRv6, err := utils.ParseCIDRs(podCIDRs)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to parse podCIDRs")
	}

	v4PodCidrs = append(v4PodCidrs, podCIDRsV4...)
	v6PodCidrs = append(v6PodCidrs, podCIDRv6...)

	return v4PodCidrs, v6PodCidrs, nil
}

// getInfraRoutes() returns the infra routes including infravnet/pod/service cidrs for the podIPInfo used in SWIFT V2 Linux scenario
// Linux uses 169.254.1.1 as the default ipv4 gateway and fe80::1234:5678:9abc as the default ipv6 gateway
func (k *K8sSWIFTv2Middleware) getInfraRoutes(podIPInfo *cns.PodIpInfo) ([]cns.Route, error) {
	var routes []cns.Route

	ip, err := netip.ParseAddr(podIPInfo.PodIPConfig.IPAddress)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse podIPConfig IP address %s", podIPInfo.PodIPConfig.IPAddress)
	}

	v4IPs, v6IPs, err := k.GetInfravnetAndServiceCidrs()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get infravnet and service CIDRs")
	}

	v4PodIPs, v6PodIPs, err := k.GetPodCidrs()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get pod CIDRs")
	}

	v4IPs = append(v4IPs, v4PodIPs...)
	v6IPs = append(v6IPs, v6PodIPs...)

	if ip.Is4() {
		routes = append(routes, k.AddRoutes(v4IPs, overlayGatewayv4)...)
	} else {
		routes = append(routes, k.AddRoutes(v6IPs, overlayGatewayV6)...)
	}

	return routes, nil
}

// assignSubnetPrefixLengthFields is a no-op for linux swiftv2 as the default prefix-length is sufficient
func (k *K8sSWIFTv2Middleware) assignSubnetPrefixLengthFields(_ *cns.PodIpInfo, _ v1alpha1.InterfaceInfo, _ string) error {
	return nil
}

// add default route is done on setRoutes() for Linux swiftv2
func (k *K8sSWIFTv2Middleware) addDefaultRoute(*cns.PodIpInfo, string) {}

// IPConfigsRequestHandlerWrapper is the middleware function for handling SWIFT v2 IP configs requests for AKS-SWIFT. This function wrapped the default SWIFT request
// and release IP configs handlers.
func (k *K8sSWIFTv2Middleware) IPConfigsRequestHandlerWrapper(defaultHandler, failureHandler cns.IPConfigsHandlerFunc) cns.IPConfigsHandlerFunc {
	return func(ctx context.Context, req cns.IPConfigsRequest) (*cns.IPConfigsResponse, error) {
		podInfo, respCode, message := k.GetPodInfoForIPConfigsRequest(ctx, &req)

		if respCode != types.Success {
			return &cns.IPConfigsResponse{
				Response: cns.Response{
					ReturnCode: respCode,
					Message:    message,
				},
			}, errors.New("failed to validate IP configs request")
		}
		ipConfigsResp, err := defaultHandler(ctx, req)
		// If the pod is not v2, return the response from the handler
		if !req.SecondaryInterfacesExist {
			return ipConfigsResp, err
		}
		// If the pod is v2, get the infra IP configs from the handler first and then add the SWIFTv2 IP config
		defer func() {
			// Release the default IP config if there is an error
			if err != nil {
				_, err = failureHandler(ctx, req)
				if err != nil {
					logger.Errorf("failed to release default IP config : %v", err)
				}
			}
		}()
		if err != nil {
			return ipConfigsResp, err
		}
		SWIFTv2PodIPInfos, err := k.getIPConfig(ctx, podInfo)
		if err != nil {
			return &cns.IPConfigsResponse{
				Response: cns.Response{
					ReturnCode: types.FailedToAllocateIPConfig,
					Message:    fmt.Sprintf("AllocateIPConfig failed: %v, IP config request is %v", err, req),
				},
				PodIPInfo: []cns.PodIpInfo{},
			}, errors.Wrapf(err, "failed to get SWIFTv2 IP config : %v", req)
		}
		ipConfigsResp.PodIPInfo = append(ipConfigsResp.PodIPInfo, SWIFTv2PodIPInfos...)
		// Set routes for the pod
		for i := range ipConfigsResp.PodIPInfo {
			ipInfo := &ipConfigsResp.PodIPInfo[i]
			// Backend nics doesn't need routes to be set
			if ipInfo.NICType != cns.BackendNIC {
				err = k.setRoutes(ipInfo)
				if err != nil {
					return &cns.IPConfigsResponse{
						Response: cns.Response{
							ReturnCode: types.FailedToAllocateIPConfig,
							Message:    fmt.Sprintf("AllocateIPConfig failed: %v, IP config request is %v", err, req),
						},
						PodIPInfo: []cns.PodIpInfo{},
					}, errors.Wrapf(err, "failed to set routes for pod %s", podInfo.Name())
				}
			}
		}
		return ipConfigsResp, nil
	}
}
