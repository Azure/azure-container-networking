package policy

import (
	"encoding/json"
	"log"
)

const (
	NetworkPolicy     CNIPolicyType = "NetworkPolicy"
	EndpointPolicy    CNIPolicyType = "EndpointPolicy"
	OutBoundNatPolicy CNIPolicyType = "OutBoundNAT"
)

type CNIPolicyType string

type Policy struct {
	Type CNIPolicyType
	Data json.RawMessage
}

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

	log.Printf("OutBoundNAT policy not set.")
	return nil, nil
}
