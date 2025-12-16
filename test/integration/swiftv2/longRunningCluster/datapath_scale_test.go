//go:build scale_test
// +build scale_test

package longrunningcluster

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/test/integration/swiftv2/helpers"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

func TestDatapathScale(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Datapath Scale Suite")
}

var _ = ginkgo.Describe("Datapath Scale Tests", func() {
	rg := os.Getenv("RG")
	buildId := os.Getenv("BUILD_ID")

	if rg == "" || buildId == "" {
		ginkgo.Fail(fmt.Sprintf("Missing required environment variables: RG='%s', BUILD_ID='%s'", rg, buildId))
	}

	ginkgo.It("creates and deletes 15 pods in a burst using device plugin", func() {
		// NOTE: Maximum pods per PodNetwork/PodNetworkInstance is limited by:
		// 1. Subnet IP address capacity
		// 2. Node capacity (typically 250 pods per node)
		// 3. Available NICs on nodes (device plugin resources)
		// For this test: Creating 15 pods across aks-1 and aks-2
		// Device plugin and Kubernetes scheduler automatically place pods on nodes with available NICs

		// Define scenarios for both clusters - 8 pods on aks-1, 7 pods on aks-2 (15 total for testing)
		// IMPORTANT: Reuse existing PodNetworks from connectivity tests to avoid "duplicate podnetwork with same network id" error
		scenarios := []struct {
			cluster  string
			vnetName string
			subnet   string
			podCount int
		}{
			{cluster: "aks-1", vnetName: "cx_vnet_v1", subnet: "s1", podCount: 8},
			{cluster: "aks-2", vnetName: "cx_vnet_v3", subnet: "s1", podCount: 7},
		} // Initialize test scenarios with cache
		testScenarios := TestScenarios{
			ResourceGroup:   rg,
			BuildID:         buildId,
			VnetSubnetCache: make(map[string]VnetSubnetInfo),
			UsedNodes:       make(map[string]bool),
			PodImage:        "nicolaka/netshoot:latest",
		}

		startTime := time.Now()
		var allResources []TestResources
		for _, scenario := range scenarios {
			kubeconfig := getKubeconfigPath(scenario.cluster)

			ginkgo.By(fmt.Sprintf("Getting network info for %s/%s in cluster %s", scenario.vnetName, scenario.subnet, scenario.cluster))
			netInfo, err := GetOrFetchVnetSubnetInfo(testScenarios.ResourceGroup, scenario.vnetName, scenario.subnet, testScenarios.VnetSubnetCache)
			gomega.Expect(err).To(gomega.BeNil(), fmt.Sprintf("Failed to get network info for %s/%s", scenario.vnetName, scenario.subnet))

			vnetShort := strings.TrimPrefix(scenario.vnetName, "cx_vnet_")
			vnetShort = strings.ReplaceAll(vnetShort, "_", "-")
			subnetNameSafe := strings.ReplaceAll(scenario.subnet, "_", "-")
			pnName := fmt.Sprintf("pn-%s-%s-%s", testScenarios.BuildID, vnetShort, subnetNameSafe)         // Reuse connectivity test PN
			pniName := fmt.Sprintf("pni-scale-%s-%s-%s", testScenarios.BuildID, vnetShort, subnetNameSafe) // New PNI for scale test

			resources := TestResources{
				Kubeconfig:         kubeconfig,
				PNName:             pnName,  // References the shared PodNetwork (also the namespace)
				PNIName:            pniName, // New PNI for scale test
				Namespace:          pnName,  // Same as PN namespace
				VnetGUID:           netInfo.VnetGUID,
				SubnetGUID:         netInfo.SubnetGUID,
				SubnetARMID:        netInfo.SubnetARMID,
				SubnetToken:        netInfo.SubnetToken,
				PodNetworkTemplate: "../../manifests/swiftv2/long-running-cluster/podnetwork.yaml",
				PNITemplate:        "../../manifests/swiftv2/long-running-cluster/podnetworkinstance.yaml",
				PodTemplate:        "../../manifests/swiftv2/long-running-cluster/pod-with-device-plugin.yaml",
				PodImage:           testScenarios.PodImage,
				Reservations:       20, // Reserve 20 IPs for scale test pods
			}

			ginkgo.By(fmt.Sprintf("Reusing existing PodNetwork: %s in cluster %s", pnName, scenario.cluster))
			ginkgo.By(fmt.Sprintf("Creating PodNetworkInstance: %s (references PN: %s) in namespace %s in cluster %s", pniName, pnName, pnName, scenario.cluster))
			err = CreatePodNetworkInstanceResource(resources)
			gomega.Expect(err).To(gomega.BeNil(), "Failed to create PodNetworkInstance")

			allResources = append(allResources, resources)
		}

		//Create pods in burst across both clusters - let scheduler place them automatically
		totalPods := 0
		for _, s := range scenarios {
			totalPods += s.podCount
		}
		ginkgo.By(fmt.Sprintf("Creating %d pods in burst (auto-scheduled by device plugin)", totalPods))

		var wg sync.WaitGroup
		errors := make(chan error, totalPods)
		podIndex := 0

		for i, scenario := range scenarios {
			for j := 0; j < scenario.podCount; j++ {
				wg.Add(1)
				go func(resources TestResources, cluster string, idx int) {
					defer wg.Done()
					defer ginkgo.GinkgoRecover()

					podName := fmt.Sprintf("scale-pod-%d", idx)
					ginkgo.By(fmt.Sprintf("Creating pod %s in namespace %s in cluster %s (auto-scheduled)", podName, resources.PNName, cluster))

					// Create pod without specifying node - let device plugin and scheduler decide
					err := CreatePod(resources.Kubeconfig, PodData{
						PodName:   podName,
						NodeName:  "",
						OS:        "linux",
						PNName:    resources.PNName,
						PNIName:   resources.PNIName,
						Namespace: resources.PNName,
						Image:     resources.PodImage,
					}, resources.PodTemplate)
					if err != nil {
						errors <- fmt.Errorf("failed to create pod %s in cluster %s: %w", podName, cluster, err)
						return
					}

					err = helpers.WaitForPodScheduled(resources.Kubeconfig, resources.PNName, podName, 10, 6)
					if err != nil {
						errors <- fmt.Errorf("pod %s in cluster %s was not scheduled: %w", podName, cluster, err)
					}
				}(allResources[i], scenario.cluster, podIndex)
				podIndex++
			}
		}

		wg.Wait()
		close(errors)

		elapsedTime := time.Since(startTime)
		var errList []error
		for err := range errors {
			errList = append(errList, err)
		}
		gomega.Expect(errList).To(gomega.BeEmpty(), "Some pods failed to create")
		ginkgo.By(fmt.Sprintf("Successfully created %d pods in %s", totalPods, elapsedTime))
		ginkgo.By("Waiting 30 seconds for pods to stabilize")
		time.Sleep(30 * time.Second)

		ginkgo.By("Verifying all pods are in Running state")
		podIndex = 0
		for i, scenario := range scenarios {
			for j := 0; j < scenario.podCount; j++ {
				podName := fmt.Sprintf("scale-pod-%d", podIndex)
				err := helpers.WaitForPodRunning(allResources[i].Kubeconfig, allResources[i].PNName, podName, 5, 10)
				gomega.Expect(err).To(gomega.BeNil(), fmt.Sprintf("Pod %s did not reach running state in cluster %s", podName, scenario.cluster))
				podIndex++
			}
		}

		ginkgo.By(fmt.Sprintf("All %d pods are running successfully across both clusters", totalPods))
		ginkgo.By("Cleaning up scale test resources")
		podIndex = 0
		for i, scenario := range scenarios {
			resources := allResources[i]
			kubeconfig := resources.Kubeconfig

			for j := 0; j < scenario.podCount; j++ {
				podName := fmt.Sprintf("scale-pod-%d", podIndex)
				ginkgo.By(fmt.Sprintf("Deleting pod: %s from namespace %s in cluster %s", podName, resources.PNName, scenario.cluster))
				err := helpers.DeletePod(kubeconfig, resources.PNName, podName)
				if err != nil {
					fmt.Printf("Warning: Failed to delete pod %s: %v\n", podName, err)
				}
				podIndex++
			}

			ginkgo.By(fmt.Sprintf("Deleting PodNetworkInstance: %s from namespace %s in cluster %s", resources.PNIName, resources.PNName, scenario.cluster))
			err := helpers.DeletePodNetworkInstance(kubeconfig, resources.PNName, resources.PNIName)
			if err != nil {
				fmt.Printf("Warning: Failed to delete PNI %s: %v\n", resources.PNIName, err)
			}
			ginkgo.By(fmt.Sprintf("Keeping PodNetwork and namespace: %s (shared with connectivity tests) in cluster %s", resources.PNName, scenario.cluster))
		}

		ginkgo.By("Scale test cleanup completed")
	})
})
