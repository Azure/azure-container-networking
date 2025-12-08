//go:build delete_test

package longRunningCluster

import (
	"fmt"
	"os"
	"testing"

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
			// Customer 2 scenarios on aks-2 with cx_vnet_b1
			{
				Name:          "Customer2-AKS2-VnetB1-S1-LowNic",
				Cluster:       "aks-2",
				VnetName:      "cx_vnet_b1",
				SubnetName:    "s1",
				NodeSelector:  "low-nic",
				PodNameSuffix: "c2-aks2-b1s1-low",
			},
			{
				Name:          "Customer2-AKS2-VnetB1-S1-HighNic",
				Cluster:       "aks-2",
				VnetName:      "cx_vnet_b1",
				SubnetName:    "s1",
				NodeSelector:  "high-nic",
				PodNameSuffix: "c2-aks2-b1s1-high",
			},
			// Customer 1 scenarios
			{
				Name:          "Customer1-AKS1-VnetA1-S1-LowNic",
				Cluster:       "aks-1",
				VnetName:      "cx_vnet_a1",
				SubnetName:    "s1",
				NodeSelector:  "low-nic",
				PodNameSuffix: "c1-aks1-a1s1-low",
			},
			{
				Name:          "Customer1-AKS1-VnetA1-S2-LowNic",
				Cluster:       "aks-1",
				VnetName:      "cx_vnet_a1",
				SubnetName:    "s2",
				NodeSelector:  "low-nic",
				PodNameSuffix: "c1-aks1-a1s2-low",
			},
			{
				Name:          "Customer1-AKS1-VnetA1-S2-HighNic",
				Cluster:       "aks-1",
				VnetName:      "cx_vnet_a1",
				SubnetName:    "s2",
				NodeSelector:  "high-nic",
				PodNameSuffix: "c1-aks1-a1s2-high",
			},
			{
				Name:          "Customer1-AKS1-VnetA2-S1-HighNic",
				Cluster:       "aks-1",
				VnetName:      "cx_vnet_a2",
				SubnetName:    "s1",
				NodeSelector:  "high-nic",
				PodNameSuffix: "c1-aks1-a2s1-high",
			},
			{
				Name:          "Customer1-AKS2-VnetA2-S1-LowNic",
				Cluster:       "aks-2",
				VnetName:      "cx_vnet_a2",
				SubnetName:    "s1",
				NodeSelector:  "low-nic",
				PodNameSuffix: "c1-aks2-a2s1-low",
			},
			{
				Name:          "Customer1-AKS2-VnetA3-S1-HighNic",
				Cluster:       "aks-2",
				VnetName:      "cx_vnet_a3",
				SubnetName:    "s1",
				NodeSelector:  "high-nic",
				PodNameSuffix: "c1-aks2-a3s1-high",
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

		// Delete all scenario resources
		ginkgo.By("Deleting all test scenarios")
		err := DeleteAllScenarios(testScenarios)
		gomega.Expect(err).To(gomega.BeNil(), "Failed to delete test scenarios")

		ginkgo.By("Successfully deleted all test scenarios")
	})
})
