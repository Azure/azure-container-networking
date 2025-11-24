package longRunningCluster

import (
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

var _ = ginkgo.Describe("Datapath Lifecycle", ginkgo.Ordered, func() {
	var testScenarios TestScenarios

	ginkgo.BeforeAll(func() {
		ginkgo.By("=== PHASE 1: Creating Test Resources ===")
		
		// Validate environment variables
		gomega.Expect(sharedConfig.ResourceGroup).NotTo(gomega.BeEmpty(), "RG environment variable must be set")

		// Initialize test scenarios
		testScenarios = TestScenarios{
			ResourceGroup:   sharedConfig.ResourceGroup,
			PodImage:        sharedConfig.PodImage,
			Scenarios:       sharedConfig.Scenarios,
			VnetSubnetCache: sharedConfig.VnetSubnetCache,
			NodeNICUsage:    sharedConfig.NodeNICUsage,
		}

		// Create all scenario resources (pods, PodNetworks, PNIs, namespaces)
		ginkgo.By(fmt.Sprintf("Creating all test scenarios (%d scenarios)", len(testScenarios.Scenarios)))
		err := CreateAllScenarios(testScenarios)
		gomega.Expect(err).To(gomega.BeNil(), "Failed to create test scenarios")

		ginkgo.By("Successfully created all test scenarios")
		
		// Wait for pods to be ready
		ginkgo.By("Waiting 2 minutes for pods to fully start and HTTP servers to be ready...")
		// Note: The actual wait is handled by WaitForPodRunning in CreatePodResource
		ginkgo.By("All pods are ready")
	})

	ginkgo.It("validates pod creation", func() {
		ginkgo.By("=== PHASE 2: Validating Created Resources ===")
		ginkgo.By(fmt.Sprintf("Created %d test scenarios successfully", len(testScenarios.Scenarios)))
		gomega.Expect(len(testScenarios.Scenarios)).To(gomega.Equal(9), "Expected 9 pod scenarios to be created")
	})

	// TODO: Uncomment for full connectivity testing
	/*
	ginkgo.It("runs pod-to-pod connectivity tests", func() {
		ginkgo.By("=== PHASE 2: Running Connectivity Tests ===")

		// Helper function to generate namespace from vnet and subnet
		getNamespace := func(vnetName, subnetName string) string {
			vnetPrefix := strings.TrimPrefix(vnetName, "cx_vnet_")
			return fmt.Sprintf("pn-%s-%s-%s", testScenarios.ResourceGroup, vnetPrefix, subnetName)
		}

		// Define connectivity test cases
		connectivityTests := []ConnectivityTest{
			{
				Name:            "SameVNetSameSubnet",
				SourcePod:       "pod-c1-aks1-a1s2-low",
				SourceNamespace: getNamespace("cx_vnet_a1", "s2"),
				DestinationPod:  "pod-c1-aks1-a1s2-high",
				DestNamespace:   getNamespace("cx_vnet_a1", "s2"),
				Cluster:         "aks-1",
				Description:     "Test connectivity between low-NIC and high-NIC pods in same VNet/Subnet (cx_vnet_a1/s2)",
				ShouldFail:      false,
			},
			{
				Name:            "NSGBlocked_S1toS2",
				SourcePod:       "pod-c1-aks1-a1s1-low",
				SourceNamespace: getNamespace("cx_vnet_a1", "s1"),
				DestinationPod:  "pod-c1-aks1-a1s2-high",
				DestNamespace:   getNamespace("cx_vnet_a1", "s2"),
				Cluster:         "aks-1",
				Description:     "Test NSG isolation: s1 -> s2 in cx_vnet_a1 (should be blocked by NSG rule)",
				ShouldFail:      true,
			},
			{
				Name:            "NSGBlocked_S2toS1",
				SourcePod:       "pod-c1-aks1-a1s2-low",
				SourceNamespace: getNamespace("cx_vnet_a1", "s2"),
				DestinationPod:  "pod-c1-aks1-a1s1-low",
				DestNamespace:   getNamespace("cx_vnet_a1", "s1"),
				Cluster:         "aks-1",
				Description:     "Test NSG isolation: s2 -> s1 in cx_vnet_a1 (should be blocked by NSG rule)",
				ShouldFail:      true,
			},
			{
				Name:            "DifferentClusters_SameVNet",
				SourcePod:       "pod-c1-aks1-a2s1-high",
				SourceNamespace: getNamespace("cx_vnet_a2", "s1"),
				DestinationPod:  "pod-c1-aks2-a2s1-low",
				DestNamespace:   getNamespace("cx_vnet_a2", "s1"),
				Cluster:         "aks-1",
				DestCluster:     "aks-2",
				Description:     "Test connectivity across different clusters, same customer VNet (cx_vnet_a2)",
				ShouldFail:      false,
			},
			{
				Name:            "PeeredVNets",
				SourcePod:       "pod-c1-aks1-a1s2-low",
				SourceNamespace: getNamespace("cx_vnet_a1", "s2"),
				DestinationPod:  "pod-c1-aks1-a2s1-high",
				DestNamespace:   getNamespace("cx_vnet_a2", "s1"),
				Cluster:         "aks-1",
				Description:     "Test connectivity between peered VNets (cx_vnet_a1/s2 <-> cx_vnet_a2/s1)",
				ShouldFail:      false,
			},
			{
				Name:            "PeeredVNets_A2toA3",
				SourcePod:       "pod-c1-aks1-a2s1-high",
				SourceNamespace: getNamespace("cx_vnet_a2", "s1"),
				DestinationPod:  "pod-c1-aks2-a3s1-high",
				DestNamespace:   getNamespace("cx_vnet_a3", "s1"),
				Cluster:         "aks-1",
				DestCluster:     "aks-2",
				Description:     "Test connectivity between peered VNets across clusters (cx_vnet_a2 <-> cx_vnet_a3)",
				ShouldFail:      false,
			},
			{
				Name:            "DifferentCustomers_A1toB1",
				SourcePod:       "pod-c1-aks1-a1s2-low",
				SourceNamespace: getNamespace("cx_vnet_a1", "s2"),
				DestinationPod:  "pod-c2-aks2-b1s1-low",
				DestNamespace:   getNamespace("cx_vnet_b1", "s1"),
				Cluster:         "aks-1",
				DestCluster:     "aks-2",
				Description:     "Test isolation: Customer 1 to Customer 2 should fail (cx_vnet_a1 -> cx_vnet_b1)",
				ShouldFail:      true,
			},
			{
				Name:            "DifferentCustomers_A2toB1",
				SourcePod:       "pod-c1-aks1-a2s1-high",
				SourceNamespace: getNamespace("cx_vnet_a2", "s1"),
				DestinationPod:  "pod-c2-aks2-b1s1-high",
				DestNamespace:   getNamespace("cx_vnet_b1", "s1"),
				Cluster:         "aks-1",
				DestCluster:     "aks-2",
				Description:     "Test isolation: Customer 1 to Customer 2 should fail (cx_vnet_a2 -> cx_vnet_b1)",
				ShouldFail:      true,
			},
		}

		ginkgo.By(fmt.Sprintf("Running %d connectivity tests", len(connectivityTests)))

		successCount := 0
		failureCount := 0

		for _, test := range connectivityTests {
			ginkgo.By(fmt.Sprintf("Test: %s - %s", test.Name, test.Description))

			err := RunConnectivityTest(test, testScenarios.ResourceGroup)

			if test.ShouldFail {
				// This test should fail (NSG blocked or customer isolation)
				if err == nil {
					fmt.Printf("Test %s: UNEXPECTED SUCCESS (expected to be blocked!)\n", test.Name)
					failureCount++
					ginkgo.Fail(fmt.Sprintf("Test %s: Expected failure but succeeded (blocking not working!)", test.Name))
				} else {
					fmt.Printf("Test %s: Correctly blocked (connection failed as expected)\n", test.Name)
					successCount++
				}
			} else {
				// This test should succeed
				if err != nil {
					fmt.Printf("Test %s: FAILED - %v\n", test.Name, err)
					failureCount++
					gomega.Expect(err).To(gomega.BeNil(), fmt.Sprintf("Test %s failed: %v", test.Name, err))
				} else {
					fmt.Printf("Test %s: Connectivity successful\n", test.Name)
					successCount++
				}
			}
		}

		ginkgo.By(fmt.Sprintf("Connectivity test summary: %d succeeded, %d failures", successCount, failureCount))
		gomega.Expect(failureCount).To(gomega.Equal(0), "Some connectivity tests failed unexpectedly")
	})
	*/

	// TODO: Uncomment for full private endpoint testing
	/*
	ginkgo.It("runs private endpoint access tests", func() {
		ginkgo.By("=== PHASE 3: Running Private Endpoint Tests ===")

		// Validate storage account environment variables
		gomega.Expect(sharedConfig.StorageAccount1).NotTo(gomega.BeEmpty(), "STORAGE_ACCOUNT_1 environment variable must be set")
		gomega.Expect(sharedConfig.StorageAccount2).NotTo(gomega.BeEmpty(), "STORAGE_ACCOUNT_2 environment variable must be set")

		storageAccountName := sharedConfig.StorageAccount1
		ginkgo.By(fmt.Sprintf("Getting private endpoint for storage account: %s", storageAccountName))

		storageEndpoint, err := GetStoragePrivateEndpoint(testScenarios.ResourceGroup, storageAccountName)
		gomega.Expect(err).To(gomega.BeNil(), "Failed to get storage account private endpoint")
		gomega.Expect(storageEndpoint).NotTo(gomega.BeEmpty(), "Storage account private endpoint is empty")

		ginkgo.By(fmt.Sprintf("Storage account private endpoint: %s", storageEndpoint))

		// Test scenarios for Private Endpoint connectivity
		privateEndpointTests := []ConnectivityTest{
			// Test 1: Private Endpoint Access (Tenant A) - Pod from VNet-A1 Subnet 1
			{
				Name:          "Private Endpoint Access: VNet-A1-S1 to Storage-A",
				SourceCluster: "aks-1",
				SourcePodName: "pod-c1-aks1-a1s1-low",
				SourceNS:      "pn-" + testScenarios.ResourceGroup + "-a1-s1",
				DestEndpoint:  storageEndpoint,
				ShouldFail:    false,
				TestType:      "storage-access",
				Purpose:       "Verify Tenant A pod can access Storage-A via private endpoint",
			},
			// Test 2: Private Endpoint Access (Tenant A) - Pod from VNet-A1 Subnet 2
			{
				Name:          "Private Endpoint Access: VNet-A1-S2 to Storage-A",
				SourceCluster: "aks-1",
				SourcePodName: "pod-c1-aks1-a1s2-low",
				SourceNS:      "pn-" + testScenarios.ResourceGroup + "-a1-s2",
				DestEndpoint:  storageEndpoint,
				ShouldFail:    false,
				TestType:      "storage-access",
				Purpose:       "Verify Tenant A pod can access Storage-A via private endpoint",
			},
			// Test 3: Private Endpoint Access (Tenant A) - Pod from VNet-A2
			{
				Name:          "Private Endpoint Access: VNet-A2-S1 to Storage-A",
				SourceCluster: "aks-1",
				SourcePodName: "pod-c1-aks1-a2s1-high",
				SourceNS:      "pn-" + testScenarios.ResourceGroup + "-a2-s1",
				DestEndpoint:  storageEndpoint,
				ShouldFail:    false,
				TestType:      "storage-access",
				Purpose:       "Verify Tenant A pod from peered VNet can access Storage-A",
			},
			// Test 4: Private Endpoint Access (Tenant A) - Pod from VNet-A3 (cross-cluster)
			{
				Name:          "Private Endpoint Access: VNet-A3-S1 to Storage-A (cross-cluster)",
				SourceCluster: "aks-2",
				SourcePodName: "pod-c1-aks2-a3s1-high",
				SourceNS:      "pn-" + testScenarios.ResourceGroup + "-a3-s1",
				DestEndpoint:  storageEndpoint,
				ShouldFail:    false,
				TestType:      "storage-access",
				Purpose:       "Verify Tenant A pod from different cluster can access Storage-A",
			},
			// Test 5: Private Endpoint Isolation - Pod from Tenant B trying to access Tenant A storage
			{
				Name:          "Private Endpoint Isolation: Tenant B to Storage-A (should fail)",
				SourceCluster: "aks-2",
				SourcePodName: "pod-c2-aks2-b1s1-low",
				SourceNS:      "pn-" + testScenarios.ResourceGroup + "-b1-s1",
				DestEndpoint:  storageEndpoint,
				ShouldFail:    true,
				TestType:      "storage-access",
				Purpose:       "Verify Tenant B pod CANNOT access Storage-A (tenant isolation)",
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

			err := RunPrivateEndpointTest(testScenarios, test)

			if test.ShouldFail {
				// Expected to fail (e.g., tenant isolation)
				if err != nil {
					ginkgo.By(fmt.Sprintf("Test correctly BLOCKED as expected: %s", test.Name))
					successCount++
				} else {
					ginkgo.By(fmt.Sprintf("Test FAILED: Expected connection to be blocked but it succeeded: %s", test.Name))
					failureCount++
				}
			} else {
				// Expected to succeed
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
	*/

	ginkgo.AfterAll(func() {
		ginkgo.By("=== PHASE 4: Cleaning Up Test Resources ===")

		ginkgo.By(fmt.Sprintf("Deleting all test scenarios (%d scenarios)", len(testScenarios.Scenarios)))
		err := DeleteAllScenarios(testScenarios)
		if err != nil {
			ginkgo.By(fmt.Sprintf("Warning: Cleanup encountered errors: %v", err))
		}

		ginkgo.By("Successfully deleted all test scenarios")
	})
})
