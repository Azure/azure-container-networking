package middlewares

import (
	"context"
	"fmt"

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
	virtualGW        = "169.254.2.1"
	overlayGatewayV6 = "fe80::1234:5678:9abc"
)

type K8sSWIFTv2Middleware struct {
	Cli client.Client
}

// Verify interface compliance at compile time
var _ cns.IPConfigsHandlerMiddleware = (*K8sSWIFTv2Middleware)(nil)

func (k *K8sSWIFTv2Middleware) GetPodInfoForIPConfigsRequest(ctx context.Context, req *cns.IPConfigsRequest) (podInfo cns.PodInfo, respCode types.ResponseCode, message string) {
	// gets pod info for the specified request
	podInfo, pod, respCode, message := k.GetPodInfo(ctx, req)
	if respCode != types.Success {
		return nil, respCode, message
	}

	// validates if pod is swiftv2
	isSwiftv2 := ValidateSwiftv2Pod(pod)

	var mtpnc v1alpha1.MultitenantPodNetworkConfig
	// if swiftv2 is enabled, check if mtpnc is ready
	if isSwiftv2 {
		mtpnc, respCode, message = k.getMTPNC(ctx, podInfo)
		if respCode != types.Success {
			return nil, respCode, message
		}

		// update ipConfigRequest
		respCode, message = k.UpdateIPConfigRequest(mtpnc, req)
		if respCode != types.Success {
			return nil, respCode, message
		}
	}
	logger.Printf("[SWIFTv2Middleware] pod %s has secondary interface : %v", podInfo.Name(), req.SecondaryInterfacesExist)
	logger.Printf("[SWIFTv2Middleware] pod %s has backend interface : %v", podInfo.Name(), req.BackendInterfaceExist)

	return podInfo, types.Success, ""
}

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
		for _, interfaceInfo := range mtpnc.Status.InterfaceInfos {
			var (
				nicType    cns.NICType
				ip         string
				prefixSize int
				err        error
			)
			switch {
			case interfaceInfo.DeviceType == v1alpha1.DeviceTypeVnetNIC:
				nicType = cns.DelegatedVMNIC
			case interfaceInfo.DeviceType == v1alpha1.DeviceTypeInfiniBandNIC:
				nicType = cns.NodeNetworkInterfaceBackendNIC
			default:
				nicType = cns.DelegatedVMNIC
			}
			if nicType != cns.NodeNetworkInterfaceBackendNIC {
				// Parse MTPNC primaryIP to get the IP address and prefix length
				ip, prefixSize, err = utils.ParseIPAndPrefix(interfaceInfo.PrimaryIP)
				if err != nil {
					return nil, errors.Wrap(err, "failed to parse mtpnc primary IP and prefix")
				}
				if prefixSize != prefixLength {
					return nil, errors.Wrapf(errInvalidMTPNCPrefixLength, "mtpnc primaryIP prefix length is %d", prefixSize)
				}

				podIPInfo := cns.PodIpInfo{
					PodIPConfig: cns.IPSubnet{
						IPAddress:    ip,
						PrefixLength: uint8(prefixSize),
					},
					MacAddress:        interfaceInfo.MacAddress,
					NICType:           nicType,
					SkipDefaultRoutes: false,
					// InterfaceName is empty for DelegatedVMNIC and AccelnetFrontendNIC
				}
				// for windows scenario, it is required to add additional fields with the exact subnetAddressSpace
				// received from MTPNC, this function assigns them for windows while linux is a no-op
				err = k.assignSubnetPrefixLengthFields(&podIPInfo, interfaceInfo, ip)
				if err != nil {
					return nil, errors.Wrap(err, "failed to parse mtpnc subnetAddressSpace prefix")
				}
				podIPInfos = append(podIPInfos, podIPInfo)
				// for windows scenario, it is required to add default route with gatewayIP from CNS
				k.addDefaultRoute(&podIPInfo, interfaceInfo.GatewayIP)
			}
		}
	}

	return podIPInfos, nil
}

func (k *K8sSWIFTv2Middleware) Type() cns.SWIFTV2Mode {
	return cns.K8sSWIFTV2
}

// gets Pod Data
func (k *K8sSWIFTv2Middleware) GetPodInfo(ctx context.Context, req *cns.IPConfigsRequest) (podInfo cns.PodInfo, k8sPod v1.Pod, respCode types.ResponseCode, message string) {
	// Retrieve the pod from the cluster
	podInfo, err := cns.UnmarshalPodInfo(req.OrchestratorContext)
	if err != nil {
		errBuf := errors.Wrapf(err, "failed to unmarshalling pod info from ipconfigs request %+v", req)
		return nil, v1.Pod{}, types.UnexpectedError, errBuf.Error()
	}
	logger.Printf("[SWIFTv2Middleware] validate ipconfigs request for pod %s", podInfo.Name())
	podNamespacedName := k8stypes.NamespacedName{Namespace: podInfo.Namespace(), Name: podInfo.Name()}
	pod := v1.Pod{}
	if err := k.Cli.Get(ctx, podNamespacedName, &pod); err != nil {
		errBuf := errors.Wrapf(err, "failed to get pod %+v", podNamespacedName)
		return nil, v1.Pod{}, types.UnexpectedError, errBuf.Error()
	}
	return podInfo, pod, types.Success, ""
}

// validates if pod is multitenant by checking the pod labels, used in SWIFT V2 AKS scenario.
func ValidateSwiftv2Pod(pod v1.Pod) bool {
	// check the pod labels for Swift V2, set the request's SecondaryInterfaceSet flag to true and check if its MTPNC CRD is ready
	_, swiftV2PodNetworkLabel := pod.Labels[configuration.LabelPodSwiftV2]
	_, swiftV2PodNetworkInstanceLabel := pod.Labels[configuration.LabelPodNetworkInstanceSwiftV2]
	// check if mtpnc is nil here, if not nil then proceed
	if swiftV2PodNetworkLabel || swiftV2PodNetworkInstanceLabel {
		return true
	}
	return false
}

// checks if MTPNC is ready
func (k *K8sSWIFTv2Middleware) getMTPNC(ctx context.Context, podInfo cns.PodInfo) (mtpncResource v1alpha1.MultitenantPodNetworkConfig, respCode types.ResponseCode, message string) {
	// Check if the MTPNC CRD exists for the pod, if not, return error
	mtpnc := v1alpha1.MultitenantPodNetworkConfig{}
	mtpncNamespacedName := k8stypes.NamespacedName{Namespace: podInfo.Namespace(), Name: podInfo.Name()}
	if err := k.Cli.Get(ctx, mtpncNamespacedName, &mtpnc); err != nil {
		return v1alpha1.MultitenantPodNetworkConfig{}, types.UnexpectedError, fmt.Errorf("failed to get pod's mtpnc from cache : %w", err).Error()
	}
	// Check if the MTPNC CRD is ready. If one of the fields is empty, return error
	if !mtpnc.IsReady() {
		return v1alpha1.MultitenantPodNetworkConfig{}, types.UnexpectedError, errMTPNCNotReady.Error()
	}
	return mtpnc, types.Success, ""
}

// Updates Ip Config Request
func (k *K8sSWIFTv2Middleware) UpdateIPConfigRequest(mtpnc v1alpha1.MultitenantPodNetworkConfig, req *cns.IPConfigsRequest) (
	respCode types.ResponseCode,
	message string,
) {
	// If primary Ip is set in status field, it indicates the presence of secondary interfaces
	if mtpnc.Status.PrimaryIP != "" {
		req.SecondaryInterfacesExist = true
	}

	interfaceInfos := mtpnc.Status.InterfaceInfos
	for _, interfaceInfo := range interfaceInfos {
		if interfaceInfo.DeviceType == v1alpha1.DeviceTypeInfiniBandNIC {
			if interfaceInfo.MacAddress == "" || interfaceInfo.NCID == "" {
				return types.UnexpectedError, errMTPNCNotReady.Error()
			}
			req.BackendInterfaceExist = true
			req.BackendInterfaceMacAddresses = append(req.BackendInterfaceMacAddresses, interfaceInfo.MacAddress)

		}
		if interfaceInfo.DeviceType == v1alpha1.DeviceTypeVnetNIC {
			req.SecondaryInterfacesExist = true
		}
	}

	return types.Success, ""
}
