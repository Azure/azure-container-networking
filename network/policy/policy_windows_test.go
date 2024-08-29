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

			expectedPolicy := `{"InboundProxyPort":"15003","OutboundProxyPort":"15001","FilterTuple":{"Protocols":"6"},"UserSID":"S-1-5-32-556","InboundExceptions":{},"OutboundExceptions":{}}`

			generatedPolicy, err := GetHcnL4WFPProxyPolicy(policy)
			Expect(err).To(BeNil())
			Expect(string(generatedPolicy.Settings)).To(Equal(expectedPolicy))
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
			expectedPolicy := `{"Protocols":"TCP","Action":"Allow","Direction":"In"}`

			generatedPolicy, err := GetHcnACLPolicy(policy)
			Expect(err).To(BeNil())
			Expect(string(generatedPolicy.Settings)).To(Equal(expectedPolicy))
		})
	})

	Describe("Test GetHcnOutBoundNATPolicy", func() {
		It("Should raise error for invalid json", func() {
			policy := Policy{
				Type: OutBoundNatPolicy,
				Data: []byte(`invalid json`),
			}

			_, err := GetHcnOutBoundNATPolicy(policy, nil)
			Expect(err).NotTo(BeNil())
		})

		It("Should marshall the OutBoundNAT policy correctly", func() {
			policy := Policy{
				Type: OutBoundNatPolicy,
				Data: []byte(`{
					"Type": "OutBoundNAT",
					"ExceptionList": ["10.240.0.0/16","10.0.0.0/8"]
					}`),
			}
			expectedPolicy := `{"Exceptions":["10.240.0.0/16","10.0.0.0/8"]}`

			generatedPolicy, err := GetHcnOutBoundNATPolicy(policy, nil)
			Expect(err).To(BeNil())
			Expect(string(generatedPolicy.Settings)).To(Equal(expectedPolicy))

			// test getHncOutBoundNATPolicy with epInfoData
			expectedPolicy = `{"Exceptions":["10.240.0.0/16","10.0.0.0/8","50.1.1.1","60.1.1.1"]}`

			epInfoData := make(map[string]interface{})
			epInfoData[CnetAddressSpace] = []string{"50.1.1.1", "60.1.1.1"}
			generatedPolicy, err = GetHcnOutBoundNATPolicy(policy, epInfoData)
			Expect(err).To(BeNil())
			Expect(string(generatedPolicy.Settings)).To(Equal(expectedPolicy))
		})
	})

	Describe("Test GetHcnRoutePolicy", func() {
		It("Should raise error for invalid json", func() {
			policy := Policy{
				Type: RoutePolicy,
				Data: []byte(`invalid json`),
			}

			_, err := GetHcnRoutePolicy(policy)
			Expect(err).NotTo(BeNil())
		})

		It("Should marshall the Route policy correctly", func() {
			policy := Policy{
				Type: RoutePolicy,
				Data: []byte(`{
					"Type": "ROUTE",
					"DestinationPrefix": "10.0.0.0/8",
					"NeedEncap": true
					}`),
			}
			expectedPolicy := `{"DestinationPrefix":"10.0.0.0/8","NeedEncap":true}`

			generatedPolicy, err := GetHcnRoutePolicy(policy)
			Expect(err).To(BeNil())
			Expect(string(generatedPolicy.Settings)).To(Equal(expectedPolicy))
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
				Type:     "invalidType", // should return error with invalid policy type
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

	Describe("Test GetHcnEndpointPolicies with multiple policies", func() {
		It("Should marshall all policies correctly", func() {
			testPolicies := []Policy{}

			// add first portMapping policy to testPolicies
			rawPortMappingPolicyOne, _ := json.Marshal(&hcn.PortMappingPolicySetting{
				ExternalPort: 8008,
				InternalPort: 8080,
			})

			portMappingPolicyOne, _ := json.Marshal(&hcn.EndpointPolicy{
				Type:     hcn.PortMapping,
				Settings: rawPortMappingPolicyOne,
			})

			portMappinghnsPolicyOne := Policy{
				Type: PortMappingPolicy,
				Data: portMappingPolicyOne,
			}

			testPolicies = append(testPolicies, portMappinghnsPolicyOne)

			// add second portMapping policy to testPolicies
			rawPortMappingPolicyTwo, _ := json.Marshal(&hcn.PortMappingPolicySetting{
				ExternalPort: 9008,
				InternalPort: 9090,
			})

			portMappingPolicyTwo, _ := json.Marshal(&hcn.EndpointPolicy{
				Type:     hcn.PortMapping,
				Settings: rawPortMappingPolicyTwo,
			})

			portMappinghnsPolicyTwo := Policy{
				Type: PortMappingPolicy,
				Data: portMappingPolicyTwo,
			}

			testPolicies = append(testPolicies, portMappinghnsPolicyTwo)

			generatedPolicy, err := GetHcnEndpointPolicies(PortMappingPolicy, testPolicies, nil, false, true, nil)
			Expect(err).To(BeNil())

			expectedPolicy := []hcn.EndpointPolicy{
				{
					Type:     "PortMapping",
					Settings: []byte(`{"InternalPort":8080,"ExternalPort":8008}`),
				},
				{
					Type:     "PortMapping",
					Settings: []byte(`{"InternalPort":9090,"ExternalPort":9008}`),
				},
			}

			Expect(generatedPolicy).To(Equal(expectedPolicy))
		})
	})
})
