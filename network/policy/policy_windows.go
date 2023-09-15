package policy

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/network/log"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Microsoft/hcsshim"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

var (
	loggerName = "policy"
	logger     = log.InitZapLogNet(loggerName)
)

const (
	// ProtocolTcp indicates tcp protocol id for portmapping
	ProtocolTcp = 6

	// ProtocolUdp indicates udp protocol id for portmapping
	ProtocolUdp = 17

	// CnetAddressSpace indicates constant for the key string
	CnetAddressSpace = "cnetAddressSpace"
)

type KVPairRoutePolicy struct {
	Type              CNIPolicyType   `json:"Type"`
	DestinationPrefix json.RawMessage `json:"DestinationPrefix"`
	NeedEncap         json.RawMessage `json:"NeedEncap"`
}

type KVPairOutBoundNAT struct {
	Type          CNIPolicyType   `json:"Type"`
	ExceptionList json.RawMessage `json:"ExceptionList"`
}

type KVPairRoute struct {
	Type              CNIPolicyType `json:"Type"`
	DestinationPrefix string        `json:"DestinationPrefix"`
	NeedEncap         bool          `json:"NeedEncap"`
}

type KVPairL4WfpProxyPolicy struct {
	Type               CNIPolicyType   `json:"Type"`
	OutboundProxyPort  string          `json:"OutboundProxyPort"`
	InboundProxyPort   string          `json:"InboundProxyPort"`
	UserSID            string          `json:"UserSID"`
	FilterTuple        json.RawMessage `json:"FilterTuple"`
	InboundExceptions  json.RawMessage `json:"InboundExceptions"`
	OutboundExceptions json.RawMessage `json:"OutboundExceptions"`
}

type LoopbackDSR struct {
	Type      CNIPolicyType `json:"Type"`
	IPAddress net.IP        `json:"IPAddress"`
}

var ValidWinVerForDnsNat bool

// SerializePolicies serializes policies to json.
func SerializePolicies(policyType CNIPolicyType, policies []Policy, epInfoData map[string]interface{}, enableSnatForDns, enableMultiTenancy bool) []json.RawMessage {
	var (
		jsonPolicies     []json.RawMessage
		snatAndSerialize = enableMultiTenancy && enableSnatForDns
	)

	for _, policy := range policies {
		if policy.Type == policyType {
			endpointPolicyType := GetPolicyType(policy)
			switch endpointPolicyType {
			case OutBoundNatPolicy:
				if snatAndSerialize || !enableMultiTenancy {
					if serializedOutboundNatPolicy, err := SerializeOutBoundNATPolicy(policy, epInfoData); err != nil {
						logger.Error("Failed to serialize OutBoundNAT policy", zap.Error(err))
					} else {
						jsonPolicies = append(jsonPolicies, serializedOutboundNatPolicy)
					}
				}
			case PortMappingPolicy:
				// NATPolicy comes as a HNSv2 type, it needs to be converted to HNSv1
				if serializedNatPolicy, err := SerializeNATPolicy(policy); err != nil {
					logger.Error("Failed to serialize NatPolicy", zap.Error(err))
				} else {
					jsonPolicies = append(jsonPolicies, serializedNatPolicy)
				}
			case LoopbackDSRPolicy:
				if dsrPolicy, err := SerializeLoopbackDSRPolicy(policy); err != nil {
					logger.Error("Failed to serialize DSR policy", zap.Error(err))
				} else {
					jsonPolicies = append(jsonPolicies, dsrPolicy)
				}
			default:
				jsonPolicies = append(jsonPolicies, policy.Data)
			}
		}
	}

	if snatAndSerialize && ValidWinVerForDnsNat {
		// SerializePolicies is only called for HnsV1 operations
		if serializedDnsNatPolicy, err := AddDnsNATPolicyV1(); err != nil {
			logger.Error("Failed to serialize DnsNAT policy", zap.Error(err))
		} else {
			jsonPolicies = append(jsonPolicies, serializedDnsNatPolicy)
		}
	}
	return jsonPolicies
}

// GetOutBoundNatExceptionList returns exception list for outbound nat policy
func GetOutBoundNatExceptionList(policy Policy) ([]string, error) {
	var data KVPairOutBoundNAT
	if err := json.Unmarshal(policy.Data, &data); err != nil {
		return nil, err
	}

	if data.Type == OutBoundNatPolicy {
		var exceptionList []string
		if err := json.Unmarshal(data.ExceptionList, &exceptionList); err != nil {
			return nil, err
		}

		return exceptionList, nil
	}

	logger.Info("OutBoundNAT policy not set")
	return nil, nil
}

func SerializeNATPolicy(policy Policy) (json.RawMessage, error) {
	var (
		endpointPolicy    hcn.EndpointPolicy
		portMappingPolicy hcn.PortMappingPolicySetting
	)
	if err := json.Unmarshal(policy.Data, &endpointPolicy); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(endpointPolicy.Settings, &portMappingPolicy); err != nil {
		return nil, err
	}
	natPolicy := hcsshim.NatPolicy{
		Type:         "NAT",
		InternalPort: portMappingPolicy.InternalPort,
		ExternalPort: portMappingPolicy.ExternalPort,
	}
	switch portMappingPolicy.Protocol {
	case ProtocolTcp:
		natPolicy.Protocol = "TCP"
	case ProtocolUdp:
		natPolicy.Protocol = "UDP"
	}
	return json.Marshal(natPolicy)
}

// SerializeOutBoundNATPolicy formulates OutBoundNAT policy and returns serialized json
func SerializeOutBoundNATPolicy(policy Policy, epInfoData map[string]interface{}) (json.RawMessage, error) {
	outBoundNatPolicy := hcsshim.OutboundNatPolicy{}
	outBoundNatPolicy.Policy.Type = hcsshim.OutboundNat

	exceptionList, err := GetOutBoundNatExceptionList(policy)
	if err != nil {
		logger.Error("Failed to parse outbound NAT policy", zap.Error(err))
		return nil, err
	}

	if exceptionList != nil {
		for _, ipAddress := range exceptionList {
			outBoundNatPolicy.Exceptions = append(outBoundNatPolicy.Exceptions, ipAddress)
		}
	}

	if epInfoData[CnetAddressSpace] != nil {
		if cnetAddressSpace := epInfoData[CnetAddressSpace].([]string); cnetAddressSpace != nil {
			for _, ipAddress := range cnetAddressSpace {
				outBoundNatPolicy.Exceptions = append(outBoundNatPolicy.Exceptions, ipAddress)
			}
		}
	}

	if outBoundNatPolicy.Exceptions != nil {
		serializedOutboundNatPolicy, _ := json.Marshal(outBoundNatPolicy)
		return serializedOutboundNatPolicy, nil
	}

	return nil, fmt.Errorf("OutBoundNAT policy not set")
}

func SerializeLoopbackDSRPolicy(policy Policy) (json.RawMessage, error) {
	var dsrData LoopbackDSR

	if err := hcn.DSRSupported(); err != nil {
		return []byte{}, errors.Wrap(err, "hcn doesn't support DSR")
	}

	if err := json.Unmarshal(policy.Data, &dsrData); err != nil {
		return []byte{}, errors.Wrap(err, "unmarshal dsr data failed")
	}

	logger.Info("DSR policy for", zap.String("ip", dsrData.IPAddress.String()))

	hnsLoopbackRoute := hcsshim.OutboundNatPolicy{
		Policy:       hcsshim.Policy{Type: hcsshim.OutboundNat},
		Destinations: []string{dsrData.IPAddress.String()},
	}

	rawPolicy, err := json.Marshal(hnsLoopbackRoute)
	if err != nil {
		return []byte{}, errors.Wrap(err, "marshal error for loopbackdsr policy")
	}

	return rawPolicy, nil
}

// GetPolicyType parses the policy and returns the policy type
func GetPolicyType(policy Policy) CNIPolicyType {
	// Check if the type is OutBoundNAT
	var dataOutBoundNAT KVPairOutBoundNAT
	if err := json.Unmarshal(policy.Data, &dataOutBoundNAT); err == nil {
		if dataOutBoundNAT.Type == OutBoundNatPolicy {
			return OutBoundNatPolicy
		}
	}
	// Check if the type is Route
	var dataRoute KVPairRoute
	if err := json.Unmarshal(policy.Data, &dataRoute); err == nil {
		if dataRoute.Type == RoutePolicy {
			return RoutePolicy
		}
	}

	// Check if the type is L4WFPProxy
	var l4WfpProxyPolicy KVPairL4WfpProxyPolicy
	if err := json.Unmarshal(policy.Data, &l4WfpProxyPolicy); err == nil {
		if l4WfpProxyPolicy.Type == L4WFPProxyPolicy {
			return L4WFPProxyPolicy
		}
	}

	// Check if the type if Port mapping / NAT
	var dataPortMapping hcn.EndpointPolicy
	if err := json.Unmarshal(policy.Data, &dataPortMapping); err == nil {
		if dataPortMapping.Type == hcn.PortMapping {
			return PortMappingPolicy
		}
	}

	// Check if the type is ACLPolicy
	var aclPolicy hcn.AclPolicySetting
	if err := json.Unmarshal(policy.Data, &aclPolicy); err == nil {
		if aclPolicy.Action != "" {
			return ACLPolicy
		}
	}

	// Check if the type is LoopbackDSRPolicy
	var loopbackDsr LoopbackDSR
	if err := json.Unmarshal(policy.Data, &loopbackDsr); err == nil {
		if loopbackDsr.Type == LoopbackDSRPolicy {
			return LoopbackDSRPolicy
		}
	}
	// Return empty string if the policy type is invalid
	logger.Info("Returning policyType INVALID")
	return ""
}

// SerializeHcnSubnetVlanPolicy serializes subnet policy for VLAN to json.
func SerializeHcnSubnetVlanPolicy(vlanID uint32) ([]byte, error) {
	vlanPolicySetting := &hcn.VlanPolicySetting{
		IsolationId: vlanID,
	}

	vlanPolicySettingBytes, err := json.Marshal(vlanPolicySetting)
	if err != nil {
		return nil, err
	}

	vlanSubnetPolicy := &hcn.SubnetPolicy{
		Type:     hcn.VLAN,
		Settings: vlanPolicySettingBytes,
	}

	vlanSubnetPolicyBytes, err := json.Marshal(vlanSubnetPolicy)
	if err != nil {
		return nil, err
	}

	return vlanSubnetPolicyBytes, nil
}

// GetHcnNetAdapterPolicy returns network adapter name policy.
func GetHcnNetAdapterPolicy(networkAdapterName string) (hcn.NetworkPolicy, error) {
	networkAdapterNamePolicy := hcn.NetworkPolicy{
		Type: hcn.NetAdapterName,
	}

	netAdapterNamePolicySetting := &hcn.NetAdapterNameNetworkPolicySetting{
		NetworkAdapterName: networkAdapterName,
	}

	netAdapterNamePolicySettingBytes, err := json.Marshal(netAdapterNamePolicySetting)
	if err != nil {
		return networkAdapterNamePolicy, err
	}

	networkAdapterNamePolicy.Settings = netAdapterNamePolicySettingBytes

	return networkAdapterNamePolicy, nil
}

// GetHcnOutBoundNATPolicy returns outBoundNAT policy.
func GetHcnOutBoundNATPolicy(policy Policy, epInfoData map[string]interface{}) (hcn.EndpointPolicy, error) {
	outBoundNATPolicy := hcn.EndpointPolicy{
		Type: hcn.OutBoundNAT,
	}

	outBoundNATPolicySetting := hcn.OutboundNatPolicySetting{}
	exceptionList, err := GetOutBoundNatExceptionList(policy)
	if err != nil {
		logger.Error("Failed to parse outbound NAT policy", zap.Error(err))
		return outBoundNATPolicy, err
	}

	if exceptionList != nil {
		for _, ipAddress := range exceptionList {
			outBoundNATPolicySetting.Exceptions = append(outBoundNATPolicySetting.Exceptions, ipAddress)
		}
	}

	if epInfoData[CnetAddressSpace] != nil {
		if cnetAddressSpace := epInfoData[CnetAddressSpace].([]string); cnetAddressSpace != nil {
			for _, ipAddress := range cnetAddressSpace {
				outBoundNATPolicySetting.Exceptions = append(outBoundNATPolicySetting.Exceptions, ipAddress)
			}
		}
	}

	if outBoundNATPolicySetting.Exceptions != nil {
		outBoundNATPolicySettingBytes, err := json.Marshal(outBoundNATPolicySetting)
		if err != nil {
			return outBoundNATPolicy, err
		}

		outBoundNATPolicy.Settings = outBoundNATPolicySettingBytes
		return outBoundNATPolicy, nil
	}

	return outBoundNATPolicy, fmt.Errorf("OutBoundNAT policy not set")
}

// GetHcnRoutePolicy returns Route policy.
func GetHcnRoutePolicy(policy Policy) (hcn.EndpointPolicy, error) {
	routePolicy := hcn.EndpointPolicy{
		Type: hcn.SDNRoute,
	}

	var data KVPairRoutePolicy
	if err := json.Unmarshal(policy.Data, &data); err != nil {
		return routePolicy, err
	}

	if data.Type == RoutePolicy {
		var destinationPrefix string
		var needEncap bool

		if err := json.Unmarshal(data.DestinationPrefix, &destinationPrefix); err != nil {
			return routePolicy, err
		}

		if err := json.Unmarshal(data.NeedEncap, &needEncap); err != nil {
			return routePolicy, err
		}

		sdnRoutePolicySetting := &hcn.SDNRoutePolicySetting{
			DestinationPrefix: destinationPrefix,
			NeedEncap:         needEncap,
		}

		routePolicySettingBytes, err := json.Marshal(sdnRoutePolicySetting)
		if err != nil {
			return routePolicy, err
		}

		routePolicy.Settings = routePolicySettingBytes

		return routePolicy, nil
	}

	return routePolicy, fmt.Errorf("Invalid policy: %+v. Expecting Route policy", policy)
}

// GetHcnPortMappingPolicy returns port mapping policy.
func GetHcnPortMappingPolicy(policy Policy) (hcn.EndpointPolicy, error) {
	var portMappingPolicy hcn.EndpointPolicy
	if err := json.Unmarshal(policy.Data, &portMappingPolicy); err != nil {
		return portMappingPolicy,
			fmt.Errorf("Invalid policy: %+v. Expecting PortMapping policy. Error: %v", policy, err)
	}
	portMappingPolicy.Type = hcn.PortMapping

	return portMappingPolicy, nil
}

// GetHcnACLPolicy returns ACL policy.
func GetHcnACLPolicy(policy Policy) (hcn.EndpointPolicy, error) {
	aclEndpolicySetting := hcn.EndpointPolicy{
		Type: hcn.ACL,
	}

	// Check beforehand, the input meets the expected format
	// otherwise, endpoint creation will fail later on.
	var aclPolicySetting hcn.AclPolicySetting
	if err := json.Unmarshal(policy.Data, &aclPolicySetting); err != nil {
		return aclEndpolicySetting, err
	}

	aclPolicySettingBytes, err := json.Marshal(aclPolicySetting)
	if err != nil {
		return aclEndpolicySetting, err
	}

	aclEndpolicySetting.Settings = aclPolicySettingBytes
	return aclEndpolicySetting, nil
}

// GetHcnL4WFPProxyPolicy returns L4WFPProxy policy.
func GetHcnL4WFPProxyPolicy(policy Policy) (hcn.EndpointPolicy, error) {
	l4WfpEndpolicySetting := hcn.EndpointPolicy{
		Type: hcn.L4WFPPROXY,
	}

	// Check beforehand, the input meets the expected format
	// otherwise, endpoint creation will fail later on.
	var l4WfpProxyPolicySetting hcn.L4WfpProxyPolicySetting
	if err := json.Unmarshal(policy.Data, &l4WfpProxyPolicySetting); err != nil {
		return l4WfpEndpolicySetting, err
	}

	l4WfpProxyPolicySettingBytes, err := json.Marshal(l4WfpProxyPolicySetting)
	if err != nil {
		return l4WfpEndpolicySetting, err
	}

	l4WfpEndpolicySetting.Settings = l4WfpProxyPolicySettingBytes
	return l4WfpEndpolicySetting, nil
}

// GetHcnLoopbackDSRPolicy policy is for pod to reach itself by cluster service IP.
func GetHcnLoopbackDSRPolicy(policy Policy) (hcn.EndpointPolicy, error) {
	var dsrData LoopbackDSR

	if err := hcn.DSRSupported(); err != nil {
		return hcn.EndpointPolicy{}, errors.Wrap(err, "hcn doesn't support DSR")
	}

	if err := json.Unmarshal(policy.Data, &dsrData); err != nil {
		return hcn.EndpointPolicy{}, errors.Wrap(err, "unmarshal dsr data failed")
	}

	logger.Info("DSR policy for", zap.String("ip", dsrData.IPAddress.String()))

	hcnLoopbackRoute := hcn.OutboundNatPolicySetting{
		Destinations: []string{dsrData.IPAddress.String()},
	}
	rawPolicy, _ := json.Marshal(hcnLoopbackRoute)
	endpointPolicy := hcn.EndpointPolicy{
		Type:     hcn.OutBoundNAT,
		Settings: rawPolicy,
	}

	return endpointPolicy, nil
}

// GetHcnEndpointPolicies returns array of all endpoint policies.
func GetHcnEndpointPolicies(policyType CNIPolicyType, policies []Policy, epInfoData map[string]interface{}, enableSnatForDns, enableMultiTenancy bool, natInfo []NATInfo) ([]hcn.EndpointPolicy, error) {
	var hcnEndPointPolicies []hcn.EndpointPolicy

	for _, policy := range policies {
		if policy.Type == policyType {
			var err error
			var endpointPolicy hcn.EndpointPolicy
			var isOutboundNatPolicy bool

			switch GetPolicyType(policy) {
			case OutBoundNatPolicy:
				endpointPolicy, err = GetHcnOutBoundNATPolicy(policy, epInfoData)
				isOutboundNatPolicy = true
			case RoutePolicy:
				endpointPolicy, err = GetHcnRoutePolicy(policy)
			case PortMappingPolicy:
				endpointPolicy, err = GetHcnPortMappingPolicy(policy)
			case ACLPolicy:
				endpointPolicy, err = GetHcnACLPolicy(policy)
			case L4WFPProxyPolicy:
				endpointPolicy, err = GetHcnL4WFPProxyPolicy(policy)
			case LoopbackDSRPolicy:
				endpointPolicy, err = GetHcnLoopbackDSRPolicy(policy)
			default:
				// return error as we should be able to parse all the policies specified
				return hcnEndPointPolicies, fmt.Errorf("Failed to set Policy: Type: %s, Data: %s", policy.Type, policy.Data)
			}

			if err != nil {
				logger.Error("Failed to parse policy", zap.Any("data", policy.Data), zap.Error(err))
				return hcnEndPointPolicies, err
			}

			if !(isOutboundNatPolicy && enableMultiTenancy && !enableSnatForDns) {
				hcnEndPointPolicies = append(hcnEndPointPolicies, endpointPolicy)
				logger.Info("Successfully retrieve endpoint policy", zap.Any("type", endpointPolicy.Type))
			}
		}
	}

	if ValidWinVerForDnsNat {
		for _, natRule := range natInfo {
			natPolicy, err := AddNATPolicyV2(natRule.VirtualIP, natRule.Destinations)
			if err != nil {
				logger.Error("Failed to retrieve NAT endpoint policy due to error", zap.Error(err))
				return hcnEndPointPolicies, err
			}

			hcnEndPointPolicies = append(hcnEndPointPolicies, natPolicy)
			logger.Info("Successfully retrieve natInfo policy", zap.Any("type", natPolicy.Type))
		}
	}

	return hcnEndPointPolicies, nil
}

// AddDnsNATPolicyV1 returns serialized DNS NAT policy (json) for HNSv1
func AddDnsNATPolicyV1() (json.RawMessage, error) {
	outBoundNatPolicy := hcsshim.OutboundNatPolicy{
		Policy:       hcsshim.Policy{Type: hcsshim.OutboundNat},
		Destinations: []string{networkutils.AzureDNS},
	}
	serializedPolicy, err := json.Marshal(outBoundNatPolicy)
	return serializedPolicy, err
}

// AddNATPolicyV2 returns serialized endpoint policy based on vip (IP to snat to) and destination(s)
func AddNATPolicyV2(vip string, destinations []string) (hcn.EndpointPolicy, error) {
	outBoundNatPolicySettings := hcn.OutboundNatPolicySetting{VirtualIP: vip, Destinations: destinations}
	outBoundNatPolicySettingsBytes, err := json.Marshal(outBoundNatPolicySettings)
	endpointPolicy := hcn.EndpointPolicy{
		Type:     hcn.OutBoundNAT,
		Settings: outBoundNatPolicySettingsBytes,
	}
	return endpointPolicy, err
}
