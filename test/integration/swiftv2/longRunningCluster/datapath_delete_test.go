//go:build delete_test

package longRunningCluster

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/test/integration/swiftv2/helpers"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func TestDatapathDelete(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	suiteConfig, reporterConfig := ginkgo.GinkgoConfiguration()
	suiteConfig.Timeout = 0
	ginkgo.RunSpecs(t, "Datapath Delete Suite", suiteConfig, reporterConfig)
}

var _ = ginkgo.Describe("Datapath Delete Tests", func() {
	ginkgo.It("deletes PodNetwork, PodNetworkInstance, and Pods", ginkgo.NodeTimeout(0), func() {
		rg := os.Getenv("RG")
		buildId := os.Getenv("BUILD_ID")
		if rg == "" || buildId == "" {
			ginkgo.Fail(fmt.Sprintf("Missing required environment variables: RG='%s', BUILD_ID='%s'", rg, buildId))
		}
		// Define all test scenarios (same as create)
		scenarios := []PodScenario{
			// Customer 2 scenarios on aks-2 with cx_vnet_v4
			{
				Name:          "Customer2-AKS2-VnetV4-S1-LowNic",
				Cluster:       "aks-2",
				VnetName:      "cx_vnet_v4",
				SubnetName:    "s1",
				NodeSelector:  "low-nic",
				PodNameSuffix: "c2-aks2-v4s1-low",
			},
			{
				Name:          "Customer2-AKS2-VnetV4-S1-HighNic",
				Cluster:       "aks-2",
				VnetName:      "cx_vnet_v4",
				SubnetName:    "s1",
				NodeSelector:  "high-nic",
				PodNameSuffix: "c2-aks2-v4s1-high",
			},
			// Customer 1 scenarios
			{
				Name:          "Customer1-AKS1-VnetV1-S1-LowNic",
				Cluster:       "aks-1",
				VnetName:      "cx_vnet_v1",
				SubnetName:    "s1",
				NodeSelector:  "low-nic",
				PodNameSuffix: "c1-aks1-v1s1-low",
			},
			{
				Name:          "Customer1-AKS1-VnetV1-S2-LowNic",
				Cluster:       "aks-1",
				VnetName:      "cx_vnet_v1",
				SubnetName:    "s2",
				NodeSelector:  "low-nic",
				PodNameSuffix: "c1-aks1-v1s2-low",
			},
			{
				Name:          "Customer1-AKS1-VnetV1-S2-HighNic",
				Cluster:       "aks-1",
				VnetName:      "cx_vnet_v1",
				SubnetName:    "s2",
				NodeSelector:  "high-nic",
				PodNameSuffix: "c1-aks1-v1s2-high",
			},
			{
				Name:          "Customer1-AKS1-VnetV2-S1-HighNic",
				Cluster:       "aks-1",
				VnetName:      "cx_vnet_v2",
				SubnetName:    "s1",
				NodeSelector:  "high-nic",
				PodNameSuffix: "c1-aks1-v2s1-high",
			},
			{
				Name:          "Customer1-AKS2-VnetV2-S1-LowNic",
				Cluster:       "aks-2",
				VnetName:      "cx_vnet_v2",
				SubnetName:    "s1",
				NodeSelector:  "low-nic",
				PodNameSuffix: "c1-aks2-v2s1-low",
			},
			{
				Name:          "Customer1-AKS2-VnetV3-S1-HighNic",
				Cluster:       "aks-2",
				VnetName:      "cx_vnet_v3",
				SubnetName:    "s1",
				NodeSelector:  "high-nic",
				PodNameSuffix: "c1-aks2-v3s1-high",
			},
		}

		// Initialize test scenarios with cache
		testScenarios := TestScenarios{
			ResourceGroup:   rg,
			BuildID:         buildId,
			PodImage:        "nicolaka/netshoot:latest",
			Scenarios:       scenarios,
			VnetSubnetCache: make(map[string]VnetSubnetInfo),
			UsedNodes:       make(map[string]bool),
		}

		// Note: Scale test now cleans up after itself (deletes PNI and namespace, keeps shared PodNetwork)
		// This delete test only needs to clean up connectivity test resources

		// Delete all connectivity test scenario resources
		ginkgo.By("Deleting all connectivity test scenarios")
		err := DeleteAllScenarios(testScenarios)
		gomega.Expect(err).To(gomega.BeNil(), "Failed to delete test scenarios")

		ginkgo.By("Successfully deleted all connectivity test scenarios")
	})
})
