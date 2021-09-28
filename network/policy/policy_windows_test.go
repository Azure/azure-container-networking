// Copyright 2021 Microsoft. All rights reserved.
// MIT License

package policy

import (
	"testing"

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
})
