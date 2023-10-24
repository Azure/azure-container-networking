package middlewares

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	errMTPNCNotReady         = errors.New("mtpnc is not ready")
	errInvalidSWIFTv2NICType = errors.New("invalid NIC type for SWIFT v2 scenario")
)

const (
	prefixLength     = 32
	overlayGatewayv4 = "169.254.1.1"
	overlayGatewayV6 = "fe80::1234:5678:9abc"
)

type SWIFTv2Middleware struct {
	Cli client.Client
}

// ValidateIPConfigsRequest validates if pod is multitenant by checking the pod labels, used in SWIFT V2 scenario.
// nolint
func (m *SWIFTv2Middleware) ValidateIPConfigsRequest(ctx context.Context, req *cns.IPConfigsRequest) (respCode types.ResponseCode, message string) {
	// Retrieve the pod from the cluster
	podInfo, err := cns.UnmarshalPodInfo(req.OrchestratorContext)
	if err != nil {
		errBuf := fmt.Sprintf("unmarshalling pod info from ipconfigs request %v failed with error %v", req, err)
		return types.UnexpectedError, errBuf
	}
	podNamespacedName := k8stypes.NamespacedName{Namespace: podInfo.Namespace(), Name: podInfo.Name()}
	pod := v1.Pod{}
	if err := m.Cli.Get(ctx, podNamespacedName, &pod); err != nil {
		errBuf := fmt.Sprintf("failed to get pod %v with error %v", podNamespacedName, err)
		return types.UnexpectedError, errBuf
	}

	// check the pod labels for Swift V2, set the request's SecondaryInterfaceSet flag to true.
	if _, ok := pod.Labels[configuration.LabelPodSwiftV2]; ok {
		req.SecondaryInterfacesExist = true
	}
	return types.Success, ""
}

// GetIPConfig returns the pod's SWIFT V2 IP configuration.
func (m *SWIFTv2Middleware) GetIPConfig(ctx context.Context, podInfo cns.PodInfo) (cns.PodIpInfo, error) {
	// Check if the MTPNC CRD exists for the pod, if not, return error
	mtpnc := v1alpha1.MultitenantPodNetworkConfig{}
	mtpncNamespacedName := k8stypes.NamespacedName{Namespace: podInfo.Namespace(), Name: podInfo.Name()}
	if err := m.Cli.Get(ctx, mtpncNamespacedName, &mtpnc); err != nil {
		return cns.PodIpInfo{}, fmt.Errorf("failed to get pod's mtpnc from cache : %w", err)
	}

	// Check if the MTPNC CRD is ready. If one of the fields is empty, return error
	if mtpnc.Status.PrimaryIP == "" || mtpnc.Status.MacAddress == "" || mtpnc.Status.NCID == "" || mtpnc.Status.GatewayIP == "" {
		return cns.PodIpInfo{}, errMTPNCNotReady
	}
	podIPInfo := cns.PodIpInfo{
		PodIPConfig: cns.IPSubnet{
			IPAddress:    mtpnc.Status.PrimaryIP,
			PrefixLength: prefixLength,
		},
		MacAddress:        mtpnc.Status.MacAddress,
		NICType:           cns.DelegatedVMNIC,
		SkipDefaultRoutes: false,
		// InterfaceName is empty for DelegatedVMNIC
	}

	return podIPInfo, nil
}

// SetRoutes sets the routes for podIPInfo used in SWIFT V2 scenario.
func (m *SWIFTv2Middleware) SetRoutes(podIPInfo *cns.PodIpInfo) error {
	podIPInfo.Routes = []cns.Route{}
	switch podIPInfo.NICType {
	case cns.DelegatedVMNIC:
		// default route via SWIFT v2 interface
		route := cns.Route{
			IPAddress: "0.0.0.0/0",
		}
		podIPInfo.Routes = []cns.Route{route}
	case cns.InfraNIC:
		podCIDRs, err := configuration.PodCIDRs()
		if err != nil {
			return fmt.Errorf("failed to get podCIDRs from env : %w", err)
		}
		podCIDRsV4, podCIDRv6, err := parseCIDRs(podCIDRs)
		if err != nil {
			return fmt.Errorf("failed to parse podCIDRs : %w", err)
		}

		serviceCIDRs, err := configuration.ServiceCIDRs()
		if err != nil {
			return fmt.Errorf("failed to get serviceCIDRs from env : %w", err)
		}
		serviceCIDRsV4, serviceCIDRsV6, err := parseCIDRs(serviceCIDRs)
		if err != nil {
			return fmt.Errorf("failed to parse serviceCIDRs : %w", err)
		}
		// Check if the podIPInfo is IPv4 or IPv6
		if net.ParseIP(podIPInfo.PodIPConfig.IPAddress).To4() != nil {
			// routes for IPv4 podCIDR traffic
			for _, podCIDRv4 := range podCIDRsV4 {
				podCIDRv4Route := cns.Route{
					IPAddress:        podCIDRv4,
					GatewayIPAddress: overlayGatewayv4,
				}
				podIPInfo.Routes = append(podIPInfo.Routes, podCIDRv4Route)
			}
			// route for IPv4 serviceCIDR traffic
			for _, serviceCIDRv4 := range serviceCIDRsV4 {
				serviceCIDRv4Route := cns.Route{
					IPAddress:        serviceCIDRv4,
					GatewayIPAddress: overlayGatewayv4,
				}
				podIPInfo.Routes = append(podIPInfo.Routes, serviceCIDRv4Route)
			}

		} else {
			// routes for IPv6 podCIDR traffic
			for _, podCIDRv6 := range podCIDRv6 {
				podCIDRv6Route := cns.Route{
					IPAddress:        podCIDRv6,
					GatewayIPAddress: overlayGatewayV6,
				}
				podIPInfo.Routes = append(podIPInfo.Routes, podCIDRv6Route)
			}
			// route for IPv6 serviceCIDR traffic
			for _, serviceCIDRv6 := range serviceCIDRsV6 {
				serviceCIDRv6Route := cns.Route{
					IPAddress:        serviceCIDRv6,
					GatewayIPAddress: overlayGatewayV6,
				}
				podIPInfo.Routes = append(podIPInfo.Routes, serviceCIDRv6Route)
			}
		}
	default:
		return errInvalidSWIFTv2NICType
	}
	return nil
}

// parseCIDRs parses the semicolons separated CIDRs string and returns the IPv4 and IPv6 CIDRs.
func parseCIDRs(cidrs string) (v4IPs, v6IPs []string, err error) {
	for _, cidr := range strings.Split(cidrs, ",") {
		ip, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse cidr %s : %w", cidr, err)
		}
		if ip.To4() != nil {
			v4IPs = append(v4IPs, cidr)
		} else {
			v6IPs = append(v6IPs, cidr)
		}
	}
	return v4IPs, v6IPs, nil
}
