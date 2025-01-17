package middlewares

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/middlewares/utils"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/pkg/errors"
)

// for AKS L1VH, do not set default route on infraNIC to avoid customer pod reaching all infra vnet services
// default route is set for secondary interface NIC(i.e,delegatedNIC)
func (k *K8sSWIFTv2Middleware) setRoutes(podIPInfo *cns.PodIpInfo) error {
	if podIPInfo.NICType == cns.InfraNIC {
		// as a workaround, HNS will not set this dummy default route(0.0.0.0/0, nexthop:0.0.0.0) on infraVnet interface eth0
		// the only usage for this dummy default is to bypass HNS setting default route on eth0
		// TODO: Remove this once HNS fix is ready
		route := cns.Route{
			IPAddress:        "0.0.0.0/0",
			GatewayIPAddress: "0.0.0.0",
		}
		podIPInfo.Routes = append(podIPInfo.Routes, route)

		podIPInfo.SkipDefaultRoutes = true
	}
	return nil
}

// assignSubnetPrefixLengthFields will assign the subnet-prefix length to some fields of podipinfo
// this is required for the windows scenario so that HNS programming is successful for pods
func (k *K8sSWIFTv2Middleware) assignSubnetPrefixLengthFields(podIPInfo *cns.PodIpInfo, interfaceInfo v1alpha1.InterfaceInfo, ip string) error {
	// Parse MTPNC SubnetAddressSpace to get the subnet prefix length
	subnet, subnetPrefix, err := utils.ParseIPAndPrefix(interfaceInfo.SubnetAddressSpace)
	if err != nil {
		return errors.Wrap(err, "failed to parse mtpnc subnetAddressSpace prefix")
	}
	// assign the subnet-prefix length to all fields in podipinfo
	podIPInfo.PodIPConfig.PrefixLength = uint8(subnetPrefix)
	podIPInfo.HostPrimaryIPInfo = cns.HostIPInfo{
		Gateway:   interfaceInfo.GatewayIP,
		PrimaryIP: ip,
		Subnet:    interfaceInfo.SubnetAddressSpace,
	}
	podIPInfo.NetworkContainerPrimaryIPConfig = cns.IPConfiguration{
		IPSubnet: cns.IPSubnet{
			IPAddress:    subnet,
			PrefixLength: uint8(subnetPrefix),
		},
		GatewayIPAddress: interfaceInfo.GatewayIP,
	}
	return nil
}

// add default route with gateway IP to podIPInfo
func (k *K8sSWIFTv2Middleware) addDefaultRoute(podIPInfo *cns.PodIpInfo, gwIP string) {
	route := cns.Route{
		IPAddress:        "0.0.0.0/0",
		GatewayIPAddress: gwIP,
	}
	podIPInfo.Routes = append(podIPInfo.Routes, route)
}

// get policy of type endpoint policy given the params
func getEndpointPolicy(policyType, action, direction string, priority int) (policy.Policy, error) {
	endpointPolicy, err := createEndpointPolicy(policyType, action, direction, priority)
	if err != nil {
		return policy.Policy{}, errors.Wrap(err, "failed to create endpoint policy")
	}

	additionalArgs := policy.Policy{
		Type: policy.EndpointPolicy,
		Data: endpointPolicy,
	}

	return additionalArgs, nil
}

// create policy given the params
func createEndpointPolicy(policyType, action, direction string, priority int) ([]byte, error) {
	endpointPolicy := struct {
		Type      string `json:"Type"`
		Action    string `json:"Action"`
		Direction string `json:"Direction"`
		Priority  int    `json:"Priority"`
	}{
		Type:      policyType,
		Action:    action,
		Direction: direction,
		Priority:  priority,
	}

	rawPolicy, err := json.Marshal(endpointPolicy)
	if err != nil {
		return nil, errors.Wrap(err, "error marshalling policy to json")
	}

	return rawPolicy, nil
}

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

		// Get MTPNC
		mtpnc, respCode, message := k.getMTPNC(ctx, podInfo)
		if respCode != types.Success {
			return &cns.IPConfigsResponse{
				Response: cns.Response{
					ReturnCode: respCode,
					Message:    message,
				},
			}, errors.New("failed to validate IP configs request")
		}

		// checks for default deny bool variable
		defaultDenyACLbool, err := GetDefaultDenyBool(mtpnc)

		// ipConfigsResp has infra IP configs -> if defaultDenyACLbool is enabled, add the default deny endpoint policies as a property in PodIpInfo
		for i := range ipConfigsResp.PodIPInfo {
			ipInfo := &ipConfigsResp.PodIPInfo[i]
			// there will be no pod connectivity to and from those pods
			var defaultDenyEgressPolicy, defaultDenyIngressPolicy policy.Policy

			if defaultDenyACLbool && ipInfo.NICType == cns.InfraNIC {
				defaultDenyEgressPolicy, err = getEndpointPolicy(string(policy.ACLPolicy), cns.ActionTypeBlock, cns.DirectionTypeOut, 10_000)
				if err != nil {
					logger.Errorf("failed to add default deny acl's for pod %v with err %v", podInfo.Name(), err)
				}

				defaultDenyIngressPolicy, err = getEndpointPolicy(string(policy.ACLPolicy), cns.ActionTypeBlock, cns.DirectionTypeIn, 10_000)
				if err != nil {
					logger.Errorf("failed to add default deny acl's for pod %v with err %v", podInfo.Name(), err)
				}

				ipInfo.EndpointPolicies = append(ipInfo.EndpointPolicies, defaultDenyEgressPolicy, defaultDenyIngressPolicy)
				logger.Printf("Created endpoint policies for defaultDenyEgressPolicy and defaultDenyIngressPolicy")

				break
			}
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

func GetDefaultDenyBool(mtpnc v1alpha1.MultitenantPodNetworkConfig) (bool, error) {

	// Check if the MTPNC CRD is ready. If one of the fields is empty, return error
	if !mtpnc.IsReady() {
		return false, errMTPNCNotReady
	}

	// returns the value of DefaultDenyACL from mtpnc
	return mtpnc.Status.DefaultDenyACL, nil
}
