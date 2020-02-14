// Copyright 2019 Microsoft. All rights reserved.
// MIT License

package network

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"testing"
)


func TestEndpoint(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Network Suite")
}

var (
	_ = Describe("Test Endpoint", func() {


		Describe("Test GetPodName", func() {
			Context("GetPodName", func() {
				It("GetPodName successfully", func() {

					testData := map[string]string{
						"nginx-deployment-5c689d88bb":       "nginx",
						"nginx-deployment-5c689d88bb-qwq47": "nginx-deployment",
						"nginx": "nginx",
					}

					for testValue, expectedPodName := range testData {
						podName := GetPodNameWithoutSuffix(testValue)
						Expect(podName).To(Equal(expectedPodName))
					}
				})
			})
		})
	})
)
