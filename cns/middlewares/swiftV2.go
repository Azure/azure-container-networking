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
	v1 "k8s.io/api/core/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	errMTPNCNotReady            = errors.New("mtpnc is not ready")
	errInvalidSWIFTv2NICType    = errors.New("invalid NIC type for SWIFT v2 scenario")
	errInvalidMTPNCPrefixLength = errors.New("invalid prefix length for MTPNC primaryIP, must be 32")
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
		errBuf := errors.Wrapf(err, "failed to unmarshalling pod info from ipconfigs request %+v", req)
		return types.UnexpectedError, errBuf.Error()
	}
	logger.Printf("[SWIFTv2Middleware] validate ipconfigs request for pod %s", podInfo.Name())
	podNamespacedName := k8stypes.NamespacedName{Namespace: podInfo.Namespace(), Name: podInfo.Name()}
	pod := v1.Pod{}
	if err := m.Cli.Get(ctx, podNamespacedName, &pod); err != nil {
		errBuf := errors.Wrapf(err, "failed to get pod %+v", podNamespacedName)
		return types.UnexpectedError, errBuf.Error()
	}

	// check the pod labels for Swift V2, set the request's SecondaryInterfaceSet flag to true and check if its MTPNC CRD is ready
	if _, ok := pod.Labels[configuration.LabelPodSwiftV2]; ok {
		req.SecondaryInterfacesExist = true
		// Check if the MTPNC CRD exists for the pod, if not, return error
		mtpnc := v1alpha1.MultitenantPodNetworkConfig{}
		mtpncNamespacedName := k8stypes.NamespacedName{Namespace: podInfo.Namespace(), Name: podInfo.Name()}
		if err := m.Cli.Get(ctx, mtpncNamespacedName, &mtpnc); err != nil {
			return types.UnexpectedError, fmt.Errorf("failed to get pod's mtpnc from cache : %w", err).Error()
		}
		// Check if the MTPNC CRD is ready. If one of the fields is empty, return error
		if mtpnc.Status.PrimaryIP == "" || mtpnc.Status.MacAddress == "" || mtpnc.Status.NCID == "" || mtpnc.Status.GatewayIP == "" {
			return types.UnexpectedError, errMTPNCNotReady.Error()
		}
	}
	logger.Printf("[SWIFTv2Middleware] pod %s has secondary interface : %v", podInfo.Name(), req.SecondaryInterfacesExist)
	return types.Success, ""
}

// GetIPConfig returns the pod's SWIFT V2 IP configuration.
func (m *SWIFTv2Middleware) GetIPConfig(ctx context.Context, podInfo cns.PodInfo) (cns.PodIpInfo, error) {
	// Check if the MTPNC CRD exists for the pod, if not, return error
	mtpnc := v1alpha1.MultitenantPodNetworkConfig{}
	mtpncNamespacedName := k8stypes.NamespacedName{Namespace: podInfo.Namespace(), Name: podInfo.Name()}
	if err := m.Cli.Get(ctx, mtpncNamespacedName, &mtpnc); err != nil {
		return cns.PodIpInfo{}, errors.Wrapf(err, "failed to get pod's mtpnc from cache")
	}

	// Check if the MTPNC CRD is ready. If one of the fields is empty, return error
	if mtpnc.Status.PrimaryIP == "" || mtpnc.Status.MacAddress == "" || mtpnc.Status.NCID == "" || mtpnc.Status.GatewayIP == "" {
		return cns.PodIpInfo{}, errMTPNCNotReady
	}
	logger.Printf("[SWIFTv2Middleware] mtpnc for pod %s is : %+v", podInfo.Name(), mtpnc)

	// Parse MTPNC primaryIP to get the IP address and prefix length
	p, err := netip.ParsePrefix(mtpnc.Status.PrimaryIP)
	if err != nil {
		return cns.PodIpInfo{}, errors.Wrapf(err, "failed to parse mtpnc primaryIP %s", mtpnc.Status.PrimaryIP)
	}
	// Get the IP address and prefix length
	ip := p.Addr()
	prefixSize := p.Bits()
	if prefixSize != prefixLength {
		return cns.PodIpInfo{}, errors.Wrapf(errInvalidMTPNCPrefixLength, "mtpnc primaryIP prefix length is %d", prefixSize)
	}
	podIPInfo := cns.PodIpInfo{
		PodIPConfig: cns.IPSubnet{
			IPAddress:    ip.String(),
			PrefixLength: uint8(prefixSize),
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
	logger.Printf("[SWIFTv2Middleware] set routes for pod with nic type : %s", podIPInfo.NICType)
	podIPInfo.Routes = []cns.Route{}
	switch podIPInfo.NICType {
	case cns.DelegatedVMNIC:
		// default route via SWIFT v2 interface
		route := cns.Route{
			IPAddress: "0.0.0.0/0",
		}
		podIPInfo.Routes = []cns.Route{route}
	case cns.InfraNIC:
		// Get and parse nodeCIDRs from env
		nodeCIDRs, err := configuration.NodeCIDRs()
		if err != nil {
			return errors.Wrapf(err, "failed to get nodeCIDR from env")
		}
		nodeCIDRsv4, nodeCIDRsv6, err := utils.ParseCIDRs(nodeCIDRs)
		if err != nil {
			return errors.Wrapf(err, "failed to parse nodeCIDRs")
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
			// route for IPv4 nodeCIDR traffic
			for _, nodeCIDRv4 := range nodeCIDRsv4 {
				nodeCIDRv4Route := cns.Route{
					IPAddress:        nodeCIDRv4,
					GatewayIPAddress: overlayGatewayv4,
				}
				podIPInfo.Routes = append(podIPInfo.Routes, nodeCIDRv4Route)
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
			// route for IPv6 nodeCIDR traffic
			for _, nodeCIDRv6 := range nodeCIDRsv6 {
				nodeCIDRv6Route := cns.Route{
					IPAddress:        nodeCIDRv6,
					GatewayIPAddress: overlayGatewayV6,
				}
				podIPInfo.Routes = append(podIPInfo.Routes, nodeCIDRv6Route)
			}
		}
		podIPInfo.SkipDefaultRoutes = true
	case cns.BackendNIC:
	default:
		return errInvalidSWIFTv2NICType
	}
	return nil
}
