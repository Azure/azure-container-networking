package policy

import (
	"encoding/json"

	"github.com/Microsoft/hcsshim/hcn"
)

type CNIPolicyType string

type Policy struct {
	Type CNIPolicyType
	Data json.RawMessage
}

// TODO: RS5 at release does not process V2 Policy Schema, using V1 for now.
// SerializePolicies serializes policies to json.
func SerializePolicies(policyType CNIPolicyType, policies []Policy) []json.RawMessage {
	var jsonPolicies []json.RawMessage
	for _, policy := range policies {
		if policy.Type == policyType {
			jsonPolicies = append(jsonPolicies, policy.Data)
		}
	}
	return jsonPolicies
}

// SerializeHostComputeNetworkPolicies converts Policy objects into HCN Policy objects.
func SerializeHostComputeNetworkPolicies(policies []Policy) []hcn.NetworkPolicy {
	var hcnPolicies []hcn.NetworkPolicy
	for _, policy := range policies {
		if policy.Type == NetworkPolicy {
			var netPolicy hcn.NetworkPolicy
			if err := json.Unmarshal([]byte(policy.Data), &netPolicy); err != nil {
				panic(err)
			}
			hcnPolicies = append(hcnPolicies, netPolicy)
		}
	}

	return hcnPolicies
}

// SerializeHostComputeEndpointPolicies converts Policy objects into HCN Policy objects.
func SerializeHostComputeEndpointPolicies(policies []Policy) []hcn.EndpointPolicy {
	var hcnPolicies []hcn.EndpointPolicy
	for _, policy := range policies {
		if policy.Type == EndpointPolicy {
			var epPolicy hcn.EndpointPolicy
			if err := json.Unmarshal([]byte(policy.Data), &epPolicy); err != nil {
				panic(err)
			}
			hcnPolicies = append(hcnPolicies, epPolicy)
		}
	}

	return hcnPolicies
}

// CreateNetworkAdapterNamePolicySetting builds a NetAdapterNameNetworkPolicySetting.
func CreateNetworkAdapterNamePolicySetting(networkAdapterName string) hcn.NetworkPolicy {
	netAdapterPolicy := hcn.NetAdapterNameNetworkPolicySetting{
		NetworkAdapterName: networkAdapterName,
	}
	policyJSON, err := json.Marshal(netAdapterPolicy)
	if err != nil {
		panic(err)
	}

	return hcn.NetworkPolicy{
		Type:     hcn.NetAdapterName,
		Settings: policyJSON,
	}
}
