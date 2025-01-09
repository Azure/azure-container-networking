package middlewares

import (
	"encoding/json"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/middlewares/utils"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/Microsoft/hcsshim/hcn"
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

// append the default deny acl's to the list defaultDenyACL field in podIpInfo
func addDefaultDenyACL(podIPInfo *cns.PodIpInfo) error {
	blockEgressACL, err := getDefaultDenyACLPolicy(hcn.DirectionTypeOut)
	if err != nil {
		return errors.Wrap(err, "Failed to create default deny ACL policy egress")
	}

	blockIngressACL, err := getDefaultDenyACLPolicy(hcn.DirectionTypeIn)
	if err != nil {
		return errors.Wrap(err, "Failed to create default deny ACL policy ingress")
	}

	additionalArgs := []policy.Policy{
		{
			Type: policy.ACLPolicy,
			Data: blockEgressACL,
		},
		{
			Type: policy.ACLPolicy,
			Data: blockIngressACL,
		},
	}

	podIPInfo.EndpointPolicies = append(podIPInfo.EndpointPolicies, additionalArgs...)

	return nil
}

// create the default deny acl's that need to be added to the list defaultDenyACL field in podIpInfo
func getDefaultDenyACLPolicy(direction hcn.DirectionType) ([]byte, error) {
	type DefaultDenyACL struct {
		Type      string            `json:"Type"`
		Action    hcn.ActionType    `json:"Action"`
		Direction hcn.DirectionType `json:"Direction"`
		Priority  int               `json:"Priority"`
	}

	denyACL := DefaultDenyACL{
		Type:      "ACL", // policy type is ACL
		Action:    hcn.ActionTypeBlock,
		Direction: direction,
		Priority:  10_000, // default deny priority will be 10_000
	}

	denyACLJSON, err := json.Marshal(denyACL)

	if err != nil {
		return nil, errors.Wrap(err, "error marshalling default deny policy to json")
	}

	return denyACLJSON, nil
}
