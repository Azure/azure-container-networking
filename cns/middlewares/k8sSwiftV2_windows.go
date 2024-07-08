package middlewares

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
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
		for _, interfaceInfo := range mtpnc.Status.InterfaceInfos {
			var (
				nicType    cns.NICType
				ip         string
				prefixSize int
				err        error
			)
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
			if nicType != cns.NodeNetworkInterfaceBackendNIC {
				// Parse MTPNC primaryIP to get the IP address and prefix length
				ip, prefixSize, err = utils.ParseIPAndPrefix(interfaceInfo.PrimaryIP)
				if err != nil {
					return nil, errors.Wrap(err, "failed to parse mtpnc primary IP and prefix")
				}
				if prefixSize != prefixLength {
					return nil, errors.Wrapf(errInvalidMTPNCPrefixLength, "mtpnc primaryIP prefix length is %d", prefixSize)
				}
				// Parse MTPNC SubnetAddressSpace to get the subnet prefix length
				subnet, subnetPrefix, err := utils.ParseIPAndPrefix(interfaceInfo.PrimaryIP)
				if err != nil {
					return nil, errors.Wrap(err, "failed to parse mtpnc subnetAddressSpace prefix")
				}

				podIPInfos = append(podIPInfos, cns.PodIpInfo{
					PodIPConfig: cns.IPSubnet{
						IPAddress:    ip,
						PrefixLength: uint8(subnetPrefix),
					},
					MacAddress:        interfaceInfo.MacAddress,
					NICType:           nicType,
					SkipDefaultRoutes: false,
					HostPrimaryIPInfo: cns.HostIPInfo{
						Gateway:   interfaceInfo.GatewayIP,
						PrimaryIP: ip,
						Subnet:    interfaceInfo.SubnetAddressSpace,
					},
					NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
						IPSubnet: cns.IPSubnet{
							IPAddress:    subnet,
							PrefixLength: uint8(subnetPrefix),
						},
						GatewayIPAddress: interfaceInfo.GatewayIP,
					},
				})
			}
		}
	}

	return podIPInfos, nil
}

// for AKS L1VH, do not set default route on infraNIC to avoid customer pod reaching all infra vnet services
// default route is set for secondary interface NIC(i.e,delegatedNIC)
func (k *K8sSWIFTv2Middleware) setRoutes(podIPInfo *cns.PodIpInfo) error {
	if podIPInfo.NICType == cns.InfraNIC {
		logger.Printf("[SWIFTv2Middleware] skip setting default route on InfraNIC interface")
		podIPInfo.SkipDefaultRoutes = true
	}
	return nil
}
