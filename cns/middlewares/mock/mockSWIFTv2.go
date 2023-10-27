package mock

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/middlewares/utils"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	k8types "k8s.io/apimachinery/pkg/types"
)

var (
	errMTPNCNotReady         = errors.New("mtpnc is not ready")
	errFailedToGetPod        = errors.New("failed to get pod")
	errInvalidSWIFTv2NICType = errors.New("invalid NIC type for SWIFT v2 scenario")
)

const (
	prefixLength     = 32
	overlayGatewayv4 = "169.254.1.1"
	overlayGatewayV6 = "fe80::1234:5678:9abc"
)

type SWIFTv2Middleware struct {
	mtPodState map[string]*v1.Pod
	mtpncState map[string]*v1alpha1.MultitenantPodNetworkConfig
}

func NewMockSWIFTv2Middleware() *SWIFTv2Middleware {
	testPod1 := v1.Pod{}
	testPod1.Labels = make(map[string]string)
	testPod1.Labels[configuration.LabelPodSwiftV2] = "true"

	testMTPNC1 := v1alpha1.MultitenantPodNetworkConfig{}

	return &SWIFTv2Middleware{
		mtPodState: map[string]*v1.Pod{"testpod1namespace/testpod1": &testPod1},
		mtpncState: map[string]*v1alpha1.MultitenantPodNetworkConfig{"testpod1namespace/testpod1": &testMTPNC1},
	}
}

func (m *SWIFTv2Middleware) SetMTPNCReady() {
	m.mtpncState["testpod1namespace/testpod1"].Status.PrimaryIP = "192.168.0.1"
	m.mtpncState["testpod1namespace/testpod1"].Status.MacAddress = "00:00:00:00:00:00"
	m.mtpncState["testpod1namespace/testpod1"].Status.GatewayIP = "10.0.0.1"
	m.mtpncState["testpod1namespace/testpod1"].Status.NCID = "testncid"
}

func (m *SWIFTv2Middleware) SetEnvVar() {
	os.Setenv(configuration.EnvPodCIDRs, "10.0.1.10/24")
	os.Setenv(configuration.EnvServiceCIDRs, "10.0.2.10/24")
	os.Setenv(configuration.EnvNodeCIDRs, "10.0.3.10/24")
}

func (m *SWIFTv2Middleware) UnsetEnvVar() error {
	if err := os.Unsetenv(configuration.EnvPodCIDRs); err != nil {
		return fmt.Errorf("failed to unset env var %s : %w", configuration.EnvPodCIDRs, err)
	}
	if err := os.Unsetenv(configuration.EnvServiceCIDRs); err != nil {
		return fmt.Errorf("failed to unset env var %s : %w", configuration.EnvServiceCIDRs, err)
	}
	if err := os.Unsetenv(configuration.EnvNodeCIDRs); err != nil {
		return fmt.Errorf("failed to unset env var %s : %w", configuration.EnvNodeCIDRs, err)
	}
	return nil
}

// validateMultitenantIPConfigsRequest validates if pod is multitenant
// nolint
func (m *SWIFTv2Middleware) ValidateIPConfigsRequest(_ context.Context, req *cns.IPConfigsRequest) (respCode types.ResponseCode, message string) {
	// Retrieve the pod from the cluster
	podInfo, err := cns.UnmarshalPodInfo(req.OrchestratorContext)
	if err != nil {
		errBuf := fmt.Sprintf("unmarshalling pod info from ipconfigs request %v failed with error %v", req, err)
		return types.UnexpectedError, errBuf
	}
	podNamespacedName := k8types.NamespacedName{Namespace: podInfo.Namespace(), Name: podInfo.Name()}
	pod, ok := m.mtPodState[podNamespacedName.String()]
	if !ok {
		errBuf := fmt.Sprintf("failed to get pod %v with error %v", podNamespacedName, err)
		return types.UnexpectedError, errBuf
	}
	// check the pod labels for Swift V2, enrich the request with the multitenant flag.
	if _, ok := pod.Labels[configuration.LabelPodSwiftV2]; ok {
		req.SecondaryInterfacesExist = true
	}
	return types.Success, ""
}

// GetSWIFTv2IPConfig(podInfo PodInfo) (*PodIpInfo, error)
// GetMultitenantIPConfig returns the IP config for a multitenant pod from the MTPNC CRD
func (m *SWIFTv2Middleware) GetIPConfig(_ context.Context, podInfo cns.PodInfo) (cns.PodIpInfo, error) {
	// Check if the MTPNC CRD exists for the pod, if not, return error
	mtpncNamespacedName := k8types.NamespacedName{Namespace: podInfo.Namespace(), Name: podInfo.Name()}
	mtpnc, ok := m.mtpncState[mtpncNamespacedName.String()]
	if !ok {
		return cns.PodIpInfo{}, errFailedToGetPod
	}

	// Check if the MTPNC CRD is ready. If one of the fields is empty, return error
	if mtpnc.Status.PrimaryIP == "" || mtpnc.Status.MacAddress == "" || mtpnc.Status.NCID == "" || mtpnc.Status.GatewayIP == "" {
		return cns.PodIpInfo{}, errMTPNCNotReady
	}
	podIPInfo := cns.PodIpInfo{}
	podIPInfo.PodIPConfig = cns.IPSubnet{
		IPAddress: mtpnc.Status.PrimaryIP,
	}
	podIPInfo.MacAddress = mtpnc.Status.MacAddress
	podIPInfo.NICType = cns.DelegatedVMNIC
	podIPInfo.SkipDefaultRoutes = false

	return podIPInfo, nil
}

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
		podCIDRsV4, podCIDRv6, err := utils.ParseCIDRs(podCIDRs)
		if err != nil {
			return fmt.Errorf("failed to parse podCIDRs : %w", err)
		}

		serviceCIDRs, err := configuration.ServiceCIDRs()
		if err != nil {
			return fmt.Errorf("failed to get serviceCIDRs from env : %w", err)
		}
		serviceCIDRsV4, serviceCIDRsV6, err := utils.ParseCIDRs(serviceCIDRs)
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
