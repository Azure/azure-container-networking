//go:build delete_windows_test
// +build delete_windows_test

package longrunningcluster

import (
	"fmt"
	"os"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

func TestDatapathDeleteWindows(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Datapath Delete Windows Suite")
}

var _ = ginkgo.Describe("Datapath Delete Windows Tests", func() {
	ginkgo.It("deletes Windows PodNetwork, PodNetworkInstance, and Pods", func() {
		rg := os.Getenv("RG")
		buildId := os.Getenv("BUILD_ID")
		if rg == "" || buildId == "" {
			ginkgo.Fail(fmt.Sprintf("Missing required environment variables: RG='%s', BUILD_ID='%s'", rg, buildId))
		}

		windowsImage := os.Getenv("WINDOWS_POD_IMAGE")
		if windowsImage == "" {
			windowsImage = "mcr.microsoft.com/powershell:nanoserver-ltsc2022"
		}

		// Same scenarios as the windows create test (must match for delete to find pods).
		scenarios := []PodScenario{
			{
				Name:          "Customer2-AKS2-VnetV4-S1-LowNic-Win",
				Cluster:       "aks-2",
				VnetName:      "cx_vnet_v4",
				SubnetName:    "s1",
				NodeSelector:  "low-nic",
				PodNameSuffix: "c2-aks2-v4s1-low-win",
				OS:            "windows",
			},
			{
				Name:          "Customer2-AKS2-VnetV4-S1-HighNic-Win",
				Cluster:       "aks-2",
				VnetName:      "cx_vnet_v4",
				SubnetName:    "s1",
				NodeSelector:  "high-nic",
				PodNameSuffix: "c2-aks2-v4s1-high-win",
				OS:            "windows",
			},
			{
				Name:          "Customer1-AKS1-VnetV1-S1-LowNic-Win",
				Cluster:       "aks-1",
				VnetName:      "cx_vnet_v1",
				SubnetName:    "s1",
				NodeSelector:  "low-nic",
				PodNameSuffix: "c1-aks1-v1s1-low-win",
				OS:            "windows",
			},
			{
				Name:          "Customer1-AKS1-VnetV1-S2-LowNic-Win",
				Cluster:       "aks-1",
				VnetName:      "cx_vnet_v1",
				SubnetName:    "s2",
				NodeSelector:  "low-nic",
				PodNameSuffix: "c1-aks1-v1s2-low-win",
				OS:            "windows",
			},
			{
				Name:          "Customer1-AKS1-VnetV1-S2-HighNic-Win",
				Cluster:       "aks-1",
				VnetName:      "cx_vnet_v1",
				SubnetName:    "s2",
				NodeSelector:  "high-nic",
				PodNameSuffix: "c1-aks1-v1s2-high-win",
				OS:            "windows",
			},
			{
				Name:          "Customer1-AKS1-VnetV2-S1-HighNic-Win",
				Cluster:       "aks-1",
				VnetName:      "cx_vnet_v2",
				SubnetName:    "s1",
				NodeSelector:  "high-nic",
				PodNameSuffix: "c1-aks1-v2s1-high-win",
				OS:            "windows",
			},
			{
				Name:          "Customer1-AKS2-VnetV2-S1-LowNic-Win",
				Cluster:       "aks-2",
				VnetName:      "cx_vnet_v2",
				SubnetName:    "s1",
				NodeSelector:  "low-nic",
				PodNameSuffix: "c1-aks2-v2s1-low-win",
				OS:            "windows",
			},
			{
				Name:          "Customer1-AKS2-VnetV3-S1-HighNic-Win",
				Cluster:       "aks-2",
				VnetName:      "cx_vnet_v3",
				SubnetName:    "s1",
				NodeSelector:  "high-nic",
				PodNameSuffix: "c1-aks2-v3s1-high-win",
				OS:            "windows",
			},
		}

		// Initialize test scenarios with cache
		testScenarios := TestScenarios{
			ResourceGroup:   rg,
			BuildID:         buildId,
			PodImage:        windowsImage,
			Scenarios:       scenarios,
			VnetSubnetCache: make(map[string]VnetSubnetInfo),
			UsedNodes:       make(map[string]bool),
		}

		// Delete all scenario resources
		ginkgo.By("Deleting all windows test scenarios")
		err := DeleteAllScenarios(testScenarios)
		gomega.Expect(err).To(gomega.BeNil(), "Failed to delete windows test scenarios")

		ginkgo.By("Successfully deleted all windows test scenarios")
	})
})
