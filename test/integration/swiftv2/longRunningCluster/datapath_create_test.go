//go:build create_test
// +build create_test

package longrunningcluster

import (
	"fmt"
	"os"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

func TestDatapathCreate(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Datapath Create Suite")
}

var _ = ginkgo.Describe("Datapath Create Tests", func() {

	ginkgo.It("creates PodNetwork, PodNetworkInstance, and Pods", func() {
		rg := os.Getenv("RG")
		buildId := os.Getenv("BUILD_ID")
		if rg == "" || buildId == "" {
			ginkgo.Fail(fmt.Sprintf("Missing required environment variables: RG='%s', BUILD_ID='%s'", rg, buildId))
		}
		// Define all test scenarios
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

		// Create all scenario resources
		ginkgo.By(fmt.Sprintf("Creating all test scenarios (%d scenarios)", len(scenarios)))
		err := CreateAllScenarios(testScenarios)
		gomega.Expect(err).To(gomega.BeNil(), "Failed to create test scenarios")

		ginkgo.By("Successfully created all test scenarios")
	})
})
