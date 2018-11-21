package policy

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/Microsoft/hcsshim"
)

// SerializePolicies serializes policies to json.
func SerializePolicies(policyType CNIPolicyType, policies []Policy, epInfoData map[string]interface{}) []json.RawMessage {
	var jsonPolicies []json.RawMessage
	for _, policy := range policies {
		if policy.Type == policyType {
			if isPolicyTypeOutBoundNAT := IsPolicyTypeOutBoundNAT(policy); isPolicyTypeOutBoundNAT {
				if serializedOutboundNatPolicy, err := SerializeOutBoundNATPolicy(policies, epInfoData); err != nil {
					log.Printf("Failed to serialize OutBoundNAT policy")
				} else {
					jsonPolicies = append(jsonPolicies, serializedOutboundNatPolicy)
				}
			} else {
				jsonPolicies = append(jsonPolicies, policy.Data)
			}
		}
	}
	return jsonPolicies
}

// GetOutBoundNatExceptionList returns exception list for outbound nat policy
func GetOutBoundNatExceptionList(policies []Policy) ([]string, error) {
	type KVPair struct {
		Type          CNIPolicyType   `json:"Type"`
		ExceptionList json.RawMessage `json:"ExceptionList"`
	}

	for _, policy := range policies {
		if policy.Type == EndpointPolicy {
			var data KVPair
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
		}
	}

	log.Printf("OutBoundNAT policy not set")
	return nil, nil
}

// IsPolicyTypeOutBoundNAT return true if the policy type is OutBoundNAT
func IsPolicyTypeOutBoundNAT(policy Policy) bool {
	if policy.Type == EndpointPolicy {
		type KVPair struct {
			Type          CNIPolicyType   `json:"Type"`
			ExceptionList json.RawMessage `json:"ExceptionList"`
		}
		var data KVPair
		if err := json.Unmarshal(policy.Data, &data); err != nil {
			return false
		}

		if data.Type == OutBoundNatPolicy {
			return true
		}
	}

	return false
}

// SerializeOutBoundNATPolicy formulates OutBoundNAT policy and returns serialized json
func SerializeOutBoundNATPolicy(policies []Policy, epInfoData map[string]interface{}) (json.RawMessage, error) {
	outBoundNatPolicy := hcsshim.OutboundNatPolicy{}
	outBoundNatPolicy.Policy.Type = hcsshim.OutboundNat

	exceptionList, err := GetOutBoundNatExceptionList(policies)
	if err != nil {
		log.Printf("Failed to parse outbound NAT policy %v", err)
		return nil, err
	}

	if exceptionList != nil {
		for _, ipAddress := range exceptionList {
			outBoundNatPolicy.Exceptions = append(outBoundNatPolicy.Exceptions, ipAddress)
		}
	}

	if epInfoData["cnetAddressSpace"] != nil {
		if cnetAddressSpace := epInfoData["cnetAddressSpace"].([]string); cnetAddressSpace != nil {
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
