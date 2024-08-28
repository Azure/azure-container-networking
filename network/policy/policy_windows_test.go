// Copyright 2021 Microsoft. All rights reserved.
// MIT License

package policy

import (
	"encoding/json"
	"testing"

	"github.com/Microsoft/hcsshim/hcn"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestEndpoint(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Endpoint Suite")
}

var _ = Describe("Windows Policies", func() {
	Describe("Test GetHcnL4WFPProxyPolicy", func() {
		It("Should raise error for invalid json", func() {
			policy := Policy{
				Type: L4WFPProxyPolicy,
				Data: []byte(`invalid json`),
			}

			_, err := GetHcnL4WFPProxyPolicy(policy)
			Expect(err).NotTo(BeNil())
		})

		It("Should marshall the policy correctly", func() {
			policy := Policy{
				Type: L4WFPProxyPolicy,
				Data: []byte(`{
					"Type": "L4WFPPROXY",
					"OutboundProxyPort": "15001",
					"InboundProxyPort": "15003",
					"UserSID": "S-1-5-32-556",
					"FilterTuple": {
						"Protocols": "6"
					}}`),
			}

			expected_policy := `{"InboundProxyPort":"15003","OutboundProxyPort":"15001","FilterTuple":{"Protocols":"6"},"UserSID":"S-1-5-32-556","InboundExceptions":{},"OutboundExceptions":{}}`

			generatedPolicy, err := GetHcnL4WFPProxyPolicy(policy)
			Expect(err).To(BeNil())
			Expect(string(generatedPolicy.Settings)).To(Equal(expected_policy))
		})
	})

	Describe("Test GetHcnACLPolicy", func() {
		It("Should raise error for invalid json", func() {
			policy := Policy{
				Type: ACLPolicy,
				Data: []byte(`invalid json`),
			}

			_, err := GetHcnACLPolicy(policy)
			Expect(err).NotTo(BeNil())
		})

		It("Should marshall the ACL policy correctly", func() {
			policy := Policy{
				Type: ACLPolicy,
				Data: []byte(`{
					"Type": "ACL",
					"Protocols": "TCP",
					"Direction": "In",
					"Action": "Allow"
					}`),
			}
			expected_policy := `{"Protocols":"TCP","Action":"Allow","Direction":"In"}`

			generatedPolicy, err := GetHcnACLPolicy(policy)
			Expect(err).To(BeNil())
			Expect(string(generatedPolicy.Settings)).To(Equal(expected_policy))
		})
	})

	Describe("Test AddAccelnetPolicySetting", func() {
		It("Should marshall the policy correctly", func() {
			expectedPolicy := `{"IovOffloadWeight":100,"QueuePairsRequested":1}`

			generatedPolicy, err := AddAccelnetPolicySetting()
			Expect(err).To(BeNil())
			Expect(string(generatedPolicy.Settings)).To(Equal(expectedPolicy))
		})
	})

	Describe("Test AddNATPolicyV1", func() {
		It("Should marshall the NAT policy v1 correctly", func() {
			expectedPolicy := `{"Type":"OutBoundNAT","Destinations":["168.63.129.16"]}`

			generatedPolicy, err := AddDnsNATPolicyV1()
			Expect(err).To(BeNil())
			Expect(string(generatedPolicy)).To(Equal(expectedPolicy))
		})
	})

	Describe("Test AddNATPolicyV2", func() {
		It("Should marshall the NAT policy v2 correctly", func() {
			vip := "vip"
			destinations := []string{"192.168.1.1", "192.169.1.1"}

			expectedPolicy := `{"VirtualIP":"vip","Destinations":["192.168.1.1","192.169.1.1"]}`

			generatedPolicy, err := AddNATPolicyV2(vip, destinations)
			Expect(err).To(BeNil())
			Expect(string(generatedPolicy.Settings)).To(Equal(expectedPolicy))
		})
	})

	Describe("Test GetHcnEndpointPolicies", func() {
		It("Should marshall the policy correctly", func() {
			testPolicies := []Policy{}

			rawPortMappingPolicy, _ := json.Marshal(&hcn.PortMappingPolicySetting{
				ExternalPort: 8008,
				InternalPort: 8080,
			})

			portMappingPolicy, _ := json.Marshal(&hcn.EndpointPolicy{
				Type:     hcn.PortMapping,
				Settings: rawPortMappingPolicy,
			})

			hnsPolicy := Policy{
				Type: PortMappingPolicy,
				Data: portMappingPolicy,
			}

			testPolicies = append(testPolicies, hnsPolicy)

			generatedPolicy, err := GetHcnEndpointPolicies(PortMappingPolicy, testPolicies, nil, false, true, nil)
			Expect(err).To(BeNil())
			Expect(string(generatedPolicy[0].Settings)).To(Equal(string(rawPortMappingPolicy)))
		})
	})

	Describe("Test GetHcnEndpointPolicies with invalid policy type", func() {
		It("Should return error with invalid policy type", func() {
			testPolicies := []Policy{}

			rawPortMappingPolicy, _ := json.Marshal(&hcn.PortMappingPolicySetting{
				ExternalPort: 8008,
				InternalPort: 8080,
			})

			portMappingPolicy, _ := json.Marshal(&hcn.EndpointPolicy{
				Type:     "invalidType",
				Settings: rawPortMappingPolicy,
			})

			hnsPolicy := Policy{
				Type: PortMappingPolicy,
				Data: portMappingPolicy,
			}

			testPolicies = append(testPolicies, hnsPolicy)

			_, err := GetHcnEndpointPolicies(PortMappingPolicy, testPolicies, nil, false, true, nil)
			Expect(err).NotTo(BeNil())
		})
	})
})
