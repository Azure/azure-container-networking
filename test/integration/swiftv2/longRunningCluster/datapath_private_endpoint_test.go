//go:build private_endpoint_test
// +build private_endpoint_test

package longrunningcluster

import (
	"fmt"
	"os"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

func TestDatapathPrivateEndpoint(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Datapath Private Endpoint Suite")
}

var _ = ginkgo.Describe("Private Endpoint Tests", func() {
	rg := os.Getenv("RG")
	buildId := os.Getenv("BUILD_ID")
	storageAccount1 := os.Getenv("STORAGE_ACCOUNT_1")
	storageAccount2 := os.Getenv("STORAGE_ACCOUNT_2")

	ginkgo.It("tests private endpoint access and isolation", func() {
		if rg == "" || buildId == "" {
			ginkgo.Fail(fmt.Sprintf("Missing required environment variables: RG='%s', BUILD_ID='%s'", rg, buildId))
		}

		if storageAccount1 == "" || storageAccount2 == "" {
			ginkgo.Fail(fmt.Sprintf("Missing storage account environment variables: STORAGE_ACCOUNT_1='%s', STORAGE_ACCOUNT_2='%s'", storageAccount1, storageAccount2))
		}

		testScenarios := TestScenarios{
			ResourceGroup:   rg,
			BuildID:         buildId,
			PodImage:        "nicolaka/netshoot:latest",
			VnetSubnetCache: make(map[string]VnetSubnetInfo),
			UsedNodes:       make(map[string]bool),
		}

		storageAccountName := storageAccount1
		ginkgo.By(fmt.Sprintf("Getting private endpoint for storage account: %s", storageAccountName))

		storageEndpoint, err := GetStoragePrivateEndpoint(storageAccountName)
		gomega.Expect(err).To(gomega.BeNil(), "Failed to get storage account private endpoint")
		gomega.Expect(storageEndpoint).NotTo(gomega.BeEmpty(), "Storage account private endpoint is empty")
		ginkgo.By(fmt.Sprintf("Storage account private endpoint: %s", storageEndpoint))

		privateEndpointTests := []ConnectivityTest{
			// Test 1: Private Endpoint Access (Tenant A) - Pod from VNet-V1 Subnet 1
			{
				Name:          "Private Endpoint Access: VNet-V1-S1 to Storage-A",
				SourceCluster: "aks-1",
				SourcePodName: "pod-c1-aks1-v1s1-low",
				SourceNS:      "pn-" + testScenarios.BuildID + "-v1-s1",
				DestEndpoint:  storageEndpoint,
				ShouldFail:    false,
				TestType:      "storage-access",
				Purpose:       "Verify Tenant A pod can access Storage-A via private endpoint",
			},
			// Test 2: Private Endpoint Access (Tenant A) - Pod from VNet-V1 Subnet 2
			{
				Name:          "Private Endpoint Access: VNet-V1-S2 to Storage-A",
				SourceCluster: "aks-1",
				SourcePodName: "pod-c1-aks1-v1s2-low",
				SourceNS:      "pn-" + testScenarios.BuildID + "-v1-s2",
				DestEndpoint:  storageEndpoint,
				ShouldFail:    false,
				TestType:      "storage-access",
				Purpose:       "Verify Tenant A pod can access Storage-A via private endpoint",
			},
			// Test 3: Private Endpoint Access (Tenant A) - Pod from VNet-V2
			{
				Name:          "Private Endpoint Access: VNet-V2-S1 to Storage-A",
				SourceCluster: "aks-1",
				SourcePodName: "pod-c1-aks1-v2s1-high",
				SourceNS:      "pn-" + testScenarios.BuildID + "-v2-s1",
				DestEndpoint:  storageEndpoint,
				ShouldFail:    false,
				TestType:      "storage-access",
				Purpose:       "Verify Tenant A pod from peered VNet can access Storage-A",
			},
			// Test 4: Private Endpoint Access (Tenant A) - Pod from VNet-V3 (cross-cluster)
			{
				Name:          "Private Endpoint Access: VNet-V3-S1 to Storage-A (cross-cluster)",
				SourceCluster: "aks-2",
				SourcePodName: "pod-c1-aks2-v3s1-high",
				SourceNS:      "pn-" + testScenarios.BuildID + "-v3-s1",
				DestEndpoint:  storageEndpoint,
				ShouldFail:    false,
				TestType:      "storage-access",
				Purpose:       "Verify Tenant A pod from different cluster can access Storage-A",
			},
		}

		ginkgo.By(fmt.Sprintf("Running %d Private Endpoint connectivity tests", len(privateEndpointTests)))

		successCount := 0
		failureCount := 0

		for _, test := range privateEndpointTests {
			ginkgo.By(fmt.Sprintf("\n=== Test: %s ===", test.Name))
			ginkgo.By(fmt.Sprintf("Purpose: %s", test.Purpose))
			ginkgo.By(fmt.Sprintf("Expected: %s", func() string {
				if test.ShouldFail {
					return "BLOCKED"
				}
				return "SUCCESS"
			}()))

			err := RunPrivateEndpointTest(test)

			if test.ShouldFail {
				if err != nil {
					ginkgo.By(fmt.Sprintf("Test correctly BLOCKED as expected: %s", test.Name))
					successCount++
				} else {
					ginkgo.By(fmt.Sprintf("Test FAILED: Expected connection to be blocked but it succeeded: %s", test.Name))
					failureCount++
				}
			} else {
				if err != nil {
					ginkgo.By(fmt.Sprintf("Test FAILED: %s - Error: %v", test.Name, err))
					failureCount++
				} else {
					ginkgo.By(fmt.Sprintf("Test PASSED: %s", test.Name))
					successCount++
				}
			}
		}

		ginkgo.By(fmt.Sprintf("\n=== Private Endpoint Test Summary ==="))
		ginkgo.By(fmt.Sprintf("Total tests: %d", len(privateEndpointTests)))
		ginkgo.By(fmt.Sprintf("Successful connections: %d", successCount))
		ginkgo.By(fmt.Sprintf("Unexpected failures: %d", failureCount))

		gomega.Expect(failureCount).To(gomega.Equal(0), "Some private endpoint tests failed unexpectedly")
	})
})
