//go:build scale_test
// +build scale_test

package longRunningCluster

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/test/integration/swiftv2/helpers"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func TestDatapathScale(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	suiteConfig, reporterConfig := ginkgo.GinkgoConfiguration()
	suiteConfig.Timeout = 0
	ginkgo.RunSpecs(t, "Datapath Scale Suite", suiteConfig, reporterConfig)
}

var _ = ginkgo.Describe("Datapath Scale Tests", func() {
	rg := os.Getenv("RG")
	buildId := os.Getenv("BUILD_ID")

	if rg == "" || buildId == "" {
		ginkgo.Fail(fmt.Sprintf("Missing required environment variables: RG='%s', BUILD_ID='%s'", rg, buildId))
	}

	ginkgo.It("creates and deletes 15 pods in a burst using device plugin", ginkgo.NodeTimeout(0), func() {
		// NOTE: Maximum pods per PodNetwork/PodNetworkInstance is limited by:
		// 1. Subnet IP address capacity
		// 2. Node capacity (typically 250 pods per node)
		// 3. Available NICs on nodes (device plugin resources)
		// For this test: Creating 15 pods across aks-1 and aks-2
		// Device plugin and Kubernetes scheduler automatically place pods on nodes with available NICs

		// Define scenarios for both clusters - 3 pods on aks-1, 2 pods on aks-2 (5 total for testing)
		scenarios := []struct {
			cluster  string
			vnetName string
			subnet   string
			podCount int
		}{
			{cluster: "aks-1", vnetName: "cx_vnet_v1", subnet: "s1", podCount: 3},
			{cluster: "aks-2", vnetName: "cx_vnet_v2", subnet: "s1", podCount: 2},
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

		// Create PodNetwork and PodNetworkInstance for each scenario
		for _, scenario := range scenarios {
			kubeconfig := fmt.Sprintf("/tmp/%s.kubeconfig", scenario.cluster)

			// Get network info
			ginkgo.By(fmt.Sprintf("Getting network info for %s/%s in cluster %s", scenario.vnetName, scenario.subnet, scenario.cluster))
			netInfo, err := GetOrFetchVnetSubnetInfo(testScenarios.ResourceGroup, scenario.vnetName, scenario.subnet, testScenarios.VnetSubnetCache)
			gomega.Expect(err).To(gomega.BeNil(), fmt.Sprintf("Failed to get network info for %s/%s", scenario.vnetName, scenario.subnet))

			// Create unique names
			vnetShort := strings.TrimPrefix(scenario.vnetName, "cx_vnet_")
			vnetShort = strings.ReplaceAll(vnetShort, "_", "-")
			subnetNameSafe := strings.ReplaceAll(scenario.subnet, "_", "-")
			pnName := fmt.Sprintf("pn-scale-%s-%s-%s", testScenarios.BuildID, vnetShort, subnetNameSafe)
			pniName := fmt.Sprintf("pni-scale-%s-%s-%s", testScenarios.BuildID, vnetShort, subnetNameSafe)

			resources := TestResources{
				Kubeconfig:         kubeconfig,
				PNName:             pnName,
				PNIName:            pniName,
				VnetGUID:           netInfo.VnetGUID,
				SubnetGUID:         netInfo.SubnetGUID,
				SubnetARMID:        netInfo.SubnetARMID,
				SubnetToken:        netInfo.SubnetToken,
				PodNetworkTemplate: "../../manifests/swiftv2/long-running-cluster/podnetwork.yaml",
				PNITemplate:        "../../manifests/swiftv2/long-running-cluster/podnetworkinstance.yaml",
				PodTemplate:        "../../manifests/swiftv2/long-running-cluster/pod-with-device-plugin.yaml",
				PodImage:           testScenarios.PodImage,
			}

			// Step 1: Create PodNetwork
			ginkgo.By(fmt.Sprintf("Creating PodNetwork: %s in cluster %s", pnName, scenario.cluster))
			err = CreatePodNetworkResource(resources)
			gomega.Expect(err).To(gomega.BeNil(), "Failed to create PodNetwork")

			// Step 2: Create namespace
			ginkgo.By(fmt.Sprintf("Creating namespace: %s in cluster %s", pnName, scenario.cluster))
			err = CreateNamespaceResource(resources.Kubeconfig, resources.PNName)
			gomega.Expect(err).To(gomega.BeNil(), "Failed to create namespace")

			// Step 3: Create PodNetworkInstance
			ginkgo.By(fmt.Sprintf("Creating PodNetworkInstance: %s in cluster %s", pniName, scenario.cluster))
			err = CreatePodNetworkInstanceResource(resources)
			gomega.Expect(err).To(gomega.BeNil(), "Failed to create PodNetworkInstance")

			allResources = append(allResources, resources)
		}

		// Step 4: Create pods in burst across both clusters - let scheduler place them automatically
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
					ginkgo.By(fmt.Sprintf("Creating pod %s in cluster %s (auto-scheduled)", podName, cluster))

					// Create pod without specifying node - let device plugin and scheduler decide
					err := CreatePod(resources.Kubeconfig, PodData{
						PodName:   podName,
						NodeName:  "", // No node specified - auto-schedule
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

					// Wait for pod to be scheduled (node assignment) before considering it created
					// This prevents CNS errors about missing node names
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

		// Check for any errors
		var errList []error
		for err := range errors {
			errList = append(errList, err)
		}
		gomega.Expect(errList).To(gomega.BeEmpty(), "Some pods failed to create")

		ginkgo.By(fmt.Sprintf("Successfully created %d pods in %s", totalPods, elapsedTime))

		// Wait for pods to stabilize
		ginkgo.By("Waiting 30 seconds for pods to stabilize")
		time.Sleep(30 * time.Second)

		// Verify all pods are running
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

		// Cleanup: Delete all scale test resources
		ginkgo.By("Cleaning up scale test resources")
		podIndex = 0
		for i, scenario := range scenarios {
			resources := allResources[i]
			kubeconfig := resources.Kubeconfig

			for j := 0; j < scenario.podCount; j++ {
				podName := fmt.Sprintf("scale-pod-%d", podIndex)
				ginkgo.By(fmt.Sprintf("Deleting pod: %s from cluster %s", podName, scenario.cluster))
				err := helpers.DeletePod(kubeconfig, resources.PNName, podName)
				if err != nil {
					fmt.Printf("Warning: Failed to delete pod %s: %v\n", podName, err)
				}
				podIndex++
			}

			// Delete namespace (this will also delete PNI)
			ginkgo.By(fmt.Sprintf("Deleting namespace: %s from cluster %s", resources.PNName, scenario.cluster))
			err := helpers.DeleteNamespace(kubeconfig, resources.PNName)
			gomega.Expect(err).To(gomega.BeNil(), "Failed to delete namespace")

			// Delete PodNetworkInstance
			ginkgo.By(fmt.Sprintf("Deleting PodNetworkInstance: %s from cluster %s", resources.PNIName, scenario.cluster))
			err = helpers.DeletePodNetworkInstance(kubeconfig, resources.PNName, resources.PNIName)
			if err != nil {
				fmt.Printf("Warning: Failed to delete PNI %s: %v\n", resources.PNIName, err)
			}

			// Delete PodNetwork
			ginkgo.By(fmt.Sprintf("Deleting PodNetwork: %s from cluster %s", resources.PNName, scenario.cluster))
			err = helpers.DeletePodNetwork(kubeconfig, resources.PNName)
			gomega.Expect(err).To(gomega.BeNil(), "Failed to delete PodNetwork")
		}

		ginkgo.By("Scale test cleanup completed")
	})
})
