//go:build connectivity_test
// +build connectivity_test

package longrunningcluster

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

func TestDatapathConnectivity(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Datapath Connectivity Suite")
}

var _ = ginkgo.Describe("Datapath Connectivity Tests", func() {

	ginkgo.It("tests TCP connectivity between pods", func() {
		rg := os.Getenv("RG")
		buildId := os.Getenv("BUILD_ID")
		if rg == "" || buildId == "" {
			ginkgo.Fail(fmt.Sprintf("Missing required environment variables: RG='%s', BUILD_ID='%s'", rg, buildId))
		}
		// Helper function to generate namespace from vnet and subnet
		getNamespace := func(vnetName, subnetName string) string {
			vnetPrefix := strings.TrimPrefix(vnetName, "cx_vnet_")
			return fmt.Sprintf("pn-%s-%s-%s", rg, vnetPrefix, subnetName)
		}

		// Define connectivity test cases
		connectivityTests := []ConnectivityTest{
			{
				Name:            "SameVNetSameSubnet",
				SourcePod:       "pod-c1-aks1-v1s2-low",
				SourceNamespace: getNamespace("cx_vnet_v1", "s2"),
				DestinationPod:  "pod-c1-aks1-v1s2-high",
				DestNamespace:   getNamespace("cx_vnet_v1", "s2"),
				Cluster:         "aks-1",
				Description:     "Test connectivity between low-NIC and high-NIC pods in same VNet/Subnet (cx_vnet_v1/s2)",
				ShouldFail:      false,
			},
			{
				Name:            "NSGBlocked_S1toS2",
				SourcePod:       "pod-c1-aks1-v1s1-low",
				SourceNamespace: getNamespace("cx_vnet_v1", "s1"),
				DestinationPod:  "pod-c1-aks1-v1s2-high",
				DestNamespace:   getNamespace("cx_vnet_v1", "s2"),
				Cluster:         "aks-1",
				Description:     "Test NSG isolation: s1 -> s2 in cx_vnet_v1 (should be blocked by NSG rule)",
				ShouldFail:      true,
			},
			{
				Name:            "NSGBlocked_S2toS1",
				SourcePod:       "pod-c1-aks1-v1s2-low",
				SourceNamespace: getNamespace("cx_vnet_v1", "s2"),
				DestinationPod:  "pod-c1-aks1-v1s1-low",
				DestNamespace:   getNamespace("cx_vnet_v1", "s1"),
				Cluster:         "aks-1",
				Description:     "Test NSG isolation: s2 -> s1 in cx_vnet_v1 (should be blocked by NSG rule)",
				ShouldFail:      true,
			},
			{
				Name:            "DifferentClusters_SameVNet",
				SourcePod:       "pod-c1-aks1-v2s1-high",
				SourceNamespace: getNamespace("cx_vnet_v2", "s1"),
				DestinationPod:  "pod-c1-aks2-v2s1-low",
				DestNamespace:   getNamespace("cx_vnet_v2", "s1"),
				Cluster:         "aks-1",
				DestCluster:     "aks-2",
				Description:     "Test connectivity across different clusters, same customer VNet (cx_vnet_v2)",
				ShouldFail:      false,
			},
			{
				Name:            "PeeredVNets",
				SourcePod:       "pod-c1-aks1-v1s2-low",
				SourceNamespace: getNamespace("cx_vnet_v1", "s2"),
				DestinationPod:  "pod-c1-aks1-v2s1-high",
				DestNamespace:   getNamespace("cx_vnet_v2", "s1"),
				Cluster:         "aks-1",
				Description:     "Test connectivity between peered VNets (cx_vnet_v1/s2 <-> cx_vnet_v2/s1)",
				ShouldFail:      false,
			},
			{
				Name:            "PeeredVNets_v2tov3",
				SourcePod:       "pod-c1-aks1-v2s1-high",
				SourceNamespace: getNamespace("cx_vnet_v2", "s1"),
				DestinationPod:  "pod-c1-aks2-v3s1-high",
				DestNamespace:   getNamespace("cx_vnet_v3", "s1"),
				Cluster:         "aks-1",
				DestCluster:     "aks-2",
				Description:     "Test connectivity between peered VNets across clusters (cx_vnet_v2 <-> cx_vnet_v3)",
				ShouldFail:      false,
			},
			{
				Name:            "DifferentCustomers_v1tov4",
				SourcePod:       "pod-c1-aks1-v1s2-low",
				SourceNamespace: getNamespace("cx_vnet_v1", "s2"),
				DestinationPod:  "pod-c2-aks2-v4s1-low",
				DestNamespace:   getNamespace("cx_vnet_v4", "s1"),
				Cluster:         "aks-1",
				DestCluster:     "aks-2",
				Description:     "Test isolation: Customer 1 to Customer 2 should fail (cx_vnet_v1 -> cx_vnet_v4)",
				ShouldFail:      true,
			},
			{
				Name:            "DifferentCustomers_v2tov4",
				SourcePod:       "pod-c1-aks1-v2s1-high",
				SourceNamespace: getNamespace("cx_vnet_v2", "s1"),
				DestinationPod:  "pod-c2-aks2-v4s1-high",
				DestNamespace:   getNamespace("cx_vnet_v4", "s1"),
				Cluster:         "aks-1",
				DestCluster:     "aks-2",
				Description:     "Test isolation: Customer 1 to Customer 2 should fail (cx_vnet_v2 -> cx_vnet_v4)",
				ShouldFail:      true,
			},
		}

		ginkgo.By(fmt.Sprintf("Running %d connectivity tests", len(connectivityTests)))

		successCount := 0
		failureCount := 0

		for _, test := range connectivityTests {
			ginkgo.By(fmt.Sprintf("Test: %s - %s", test.Name, test.Description))

			err := RunConnectivityTest(test)

			if test.ShouldFail {
				if err == nil {
					fmt.Printf("Test %s: UNEXPECTED SUCCESS (expected to be blocked!)\n", test.Name)
					failureCount++
					ginkgo.Fail(fmt.Sprintf("Test %s: Expected failure but succeeded (blocking not working!)", test.Name))
				} else {
					fmt.Printf("Test %s: Correctly blocked (connection failed as expected)\n", test.Name)
					successCount++
				}
			} else {
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
	})
})
