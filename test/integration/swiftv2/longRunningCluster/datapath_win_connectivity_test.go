//go:build connectivity_windows_test
// +build connectivity_windows_test

package longrunningcluster

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

func TestDatapathConnectivityWindows(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Datapath Connectivity Windows Suite")
}

var _ = ginkgo.Describe("Datapath Connectivity Windows Tests", func() {

	ginkgo.It("tests TCP connectivity between windows pods", func() {
		rg := os.Getenv("RG")
		buildId := os.Getenv("BUILD_ID")
		if rg == "" || buildId == "" {
			ginkgo.Fail(fmt.Sprintf("Missing required environment variables: RG='%s', BUILD_ID='%s'", rg, buildId))
		}
		// Helper function to generate namespace from vnet and subnet (matches Linux convention).
		getNamespace := func(vnetName, subnetName string) string {
			vnetPrefix := strings.TrimPrefix(vnetName, "cx_vnet_")
			return fmt.Sprintf("pn-%s-%s-%s", buildId, vnetPrefix, subnetName)
		}

		// Connectivity matrix mirrors the Linux datapath_connectivity_test.go, retargeted at
		// windows pods (suffix "-win") created by datapath_win_create_test.go.
		connectivityTests := []ConnectivityTest{
			{
				Name:            "SameVNetSameSubnet-Win",
				SourcePod:       "pod-c1-aks1-v1s2-low-win",
				SourceNamespace: getNamespace("cx_vnet_v1", "s2"),
				DestinationPod:  "pod-c1-aks1-v1s2-high-win",
				DestNamespace:   getNamespace("cx_vnet_v1", "s2"),
				Cluster:         "aks-1",
				Description:     "Test connectivity between low-NIC and high-NIC windows pods in same VNet/Subnet (cx_vnet_v1/s2)",
				ShouldFail:      false,
			},
			{
				Name:            "NSGBlocked_S1toS2-Win",
				SourcePod:       "pod-c1-aks1-v1s1-low-win",
				SourceNamespace: getNamespace("cx_vnet_v1", "s1"),
				DestinationPod:  "pod-c1-aks1-v1s2-high-win",
				DestNamespace:   getNamespace("cx_vnet_v1", "s2"),
				Cluster:         "aks-1",
				Description:     "Test NSG isolation: s1 -> s2 in cx_vnet_v1 (should be blocked by NSG rule)",
				ShouldFail:      true,
			},
			{
				Name:            "NSGBlocked_S2toS1-Win",
				SourcePod:       "pod-c1-aks1-v1s2-low-win",
				SourceNamespace: getNamespace("cx_vnet_v1", "s2"),
				DestinationPod:  "pod-c1-aks1-v1s1-low-win",
				DestNamespace:   getNamespace("cx_vnet_v1", "s1"),
				Cluster:         "aks-1",
				Description:     "Test NSG isolation: s2 -> s1 in cx_vnet_v1 (should be blocked by NSG rule)",
				ShouldFail:      true,
			},
			{
				Name:            "DifferentClusters_SameVNet-Win",
				SourcePod:       "pod-c1-aks1-v2s1-high-win",
				SourceNamespace: getNamespace("cx_vnet_v2", "s1"),
				DestinationPod:  "pod-c1-aks2-v2s1-low-win",
				DestNamespace:   getNamespace("cx_vnet_v2", "s1"),
				Cluster:         "aks-1",
				DestCluster:     "aks-2",
				Description:     "Test connectivity across different clusters, same customer VNet (cx_vnet_v2)",
				ShouldFail:      false,
			},
			{
				Name:            "PeeredVNets-Win",
				SourcePod:       "pod-c1-aks1-v1s2-low-win",
				SourceNamespace: getNamespace("cx_vnet_v1", "s2"),
				DestinationPod:  "pod-c1-aks1-v2s1-high-win",
				DestNamespace:   getNamespace("cx_vnet_v2", "s1"),
				Cluster:         "aks-1",
				Description:     "Test connectivity between peered VNets (cx_vnet_v1/s2 <-> cx_vnet_v2/s1)",
				ShouldFail:      false,
			},
			{
				Name:            "PeeredVNets_v2tov3-Win",
				SourcePod:       "pod-c1-aks1-v2s1-high-win",
				SourceNamespace: getNamespace("cx_vnet_v2", "s1"),
				DestinationPod:  "pod-c1-aks2-v3s1-high-win",
				DestNamespace:   getNamespace("cx_vnet_v3", "s1"),
				Cluster:         "aks-1",
				DestCluster:     "aks-2",
				Description:     "Test connectivity between peered VNets across clusters (cx_vnet_v2 <-> cx_vnet_v3)",
				ShouldFail:      false,
			},
			{
				Name:            "DifferentCustomers_v1tov4-Win",
				SourcePod:       "pod-c1-aks1-v1s2-low-win",
				SourceNamespace: getNamespace("cx_vnet_v1", "s2"),
				DestinationPod:  "pod-c2-aks2-v4s1-low-win",
				DestNamespace:   getNamespace("cx_vnet_v4", "s1"),
				Cluster:         "aks-1",
				DestCluster:     "aks-2",
				Description:     "Test isolation: Customer 1 to Customer 2 should fail (cx_vnet_v1 -> cx_vnet_v4)",
				ShouldFail:      true,
			},
			{
				Name:            "DifferentCustomers_v2tov4-Win",
				SourcePod:       "pod-c1-aks1-v2s1-high-win",
				SourceNamespace: getNamespace("cx_vnet_v2", "s1"),
				DestinationPod:  "pod-c2-aks2-v4s1-high-win",
				DestNamespace:   getNamespace("cx_vnet_v4", "s1"),
				Cluster:         "aks-1",
				DestCluster:     "aks-2",
				Description:     "Test isolation: Customer 1 to Customer 2 should fail (cx_vnet_v2 -> cx_vnet_v4)",
				ShouldFail:      true,
			},
		}

		ginkgo.By(fmt.Sprintf("Running %d windows connectivity tests", len(connectivityTests)))

		successCount := 0
		failureCount := 0

		for _, test := range connectivityTests {
			ginkgo.By(fmt.Sprintf("Test: %s - %s", test.Name, test.Description))

			err := RunWindowsConnectivityTest(test)

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

		ginkgo.By(fmt.Sprintf("Windows connectivity test summary: %d succeeded, %d failures", successCount, failureCount))
	})
})
