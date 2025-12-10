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

		// Delete all scenario resources
		ginkgo.By("Deleting all test scenarios")
		err := DeleteAllScenarios(testScenarios)
		gomega.Expect(err).To(gomega.BeNil(), "Failed to delete test scenarios")

		// Delete scale test resources
		ginkgo.By("Deleting scale test resources")
		scaleScenarios := []struct {
			cluster  string
			vnetName string
			subnet   string
			podCount int
		}{
			{cluster: "aks-1", vnetName: "cx_vnet_v1", subnet: "s1", podCount: 3},
			{cluster: "aks-2", vnetName: "cx_vnet_v2", subnet: "s1", podCount: 2},
		}

		podIndex := 0
		for _, scenario := range scaleScenarios {
			kubeconfig := fmt.Sprintf("/tmp/%s.kubeconfig", scenario.cluster)
			vnetShort := strings.TrimPrefix(scenario.vnetName, "cx_vnet_")
			vnetShort = strings.ReplaceAll(vnetShort, "_", "-")
			subnetNameSafe := strings.ReplaceAll(scenario.subnet, "_", "-")
			pnName := fmt.Sprintf("pn-scale-%s-%s-%s", buildId, vnetShort, subnetNameSafe)
			pniName := fmt.Sprintf("pni-scale-%s-%s-%s", buildId, vnetShort, subnetNameSafe)

			// Delete pods
			for j := 0; j < scenario.podCount; j++ {
				podName := fmt.Sprintf("scale-pod-%d", podIndex)
				ginkgo.By(fmt.Sprintf("Deleting scale test pod: %s from cluster %s", podName, scenario.cluster))
				err := helpers.DeletePod(kubeconfig, pnName, podName)
				if err != nil {
					fmt.Printf("Warning: Failed to delete scale pod %s: %v\n", podName, err)
				}
				podIndex++
			}

			// Delete PodNetworkInstance
			ginkgo.By(fmt.Sprintf("Deleting scale test PodNetworkInstance: %s from cluster %s", pniName, scenario.cluster))
			err = helpers.DeletePodNetworkInstance(kubeconfig, pnName, pniName)
			if err != nil {
				fmt.Printf("Warning: Failed to delete scale test PNI %s: %v\n", pniName, err)
			}

			// Delete PodNetwork
			ginkgo.By(fmt.Sprintf("Deleting scale test PodNetwork: %s from cluster %s", pnName, scenario.cluster))
			err = helpers.DeletePodNetwork(kubeconfig, pnName)
			if err != nil {
				fmt.Printf("Warning: Failed to delete scale test PodNetwork %s: %v\n", pnName, err)
			}

			// Delete namespace
			ginkgo.By(fmt.Sprintf("Deleting scale test namespace: %s from cluster %s", pnName, scenario.cluster))
			err = helpers.DeleteNamespace(kubeconfig, pnName)
			if err != nil {
				fmt.Printf("Warning: Failed to delete scale test namespace %s: %v\n", pnName, err)
			}
		}

		ginkgo.By("Successfully deleted all test scenarios and scale test resources")
	})
})
