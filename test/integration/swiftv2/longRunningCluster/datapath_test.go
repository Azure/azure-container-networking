package longRunningCluster

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/Azure/azure-container-networking/test/integration/swiftv2/helpers"
)

func TestDatapath(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Datapath Suite")
}

var _ = ginkgo.Describe("Datapath Tests", func() {
	rg := os.Getenv("RG")
	buildId := os.Getenv("BUILD_ID")

	if rg == "" || buildId == "" {
		ginkgo.Fail(fmt.Sprintf("Missing required environment variables: RG='%s', BUILD_ID='%s'", rg, buildId))
	}

	cluster2 := "aks-2"
	kubeconfig2 := fmt.Sprintf("/tmp/%s.kubeconfig", cluster2)
	pnName := fmt.Sprintf("pn-%s-c2", buildId)
	pniName := fmt.Sprintf("pni-%s-c2", buildId)

	ginkgo.It("creates and deletes PodNetwork, PodNetworkInstance, and Pods in a loop", func() {
		vnetName := "cx_vnet_b1"
		subnetName := "s1"

		// Get subnet information once (doesn't change between iterations)
		vnetGUID := helpers.GetVnetGUID(rg, vnetName)
		subnetGUID := helpers.GetSubnetGUID(rg, vnetName, subnetName)
		subnetARMID := helpers.GetSubnetARMID(rg, vnetName, subnetName)
		subnetToken := helpers.GetSubnetToken(rg, vnetName, subnetName)

		// Run indefinitely until test is cancelled
		iteration := 0
		for {
			iteration++
			ginkgo.By(fmt.Sprintf("Starting iteration %d at %s", iteration, time.Now().Format(time.RFC3339)))

			// Step 1: Create PodNetwork
			ginkgo.By(fmt.Sprintf("Creating PodNetwork %s", pnName))
			err := CreatePodNetwork(kubeconfig2, PodNetworkData{
				PNName:      pnName,
				VnetGUID:    vnetGUID,
				SubnetGUID:  subnetGUID,
				SubnetARMID: subnetARMID,
				SubnetToken: subnetToken,
			}, "../../manifests/swiftv2/long-running-cluster/podnetwork.yaml")
			gomega.Expect(err).To(gomega.BeNil())

			// Step 2: Create namespace and PodNetworkInstance
			ginkgo.By(fmt.Sprintf("Creating namespace %s", pnName))
			err = helpers.EnsureNamespaceExists(kubeconfig2, pnName)
			gomega.Expect(err).To(gomega.BeNil())

			ginkgo.By(fmt.Sprintf("Creating PodNetworkInstance %s", pniName))
			err = CreatePodNetworkInstance(kubeconfig2, PNIData{
				PNIName:      pniName,
				PNName:       pnName,
				Namespace:    pnName,
				Type:         "explicit",
				Reservations: 2,
			}, "../../manifests/swiftv2/long-running-cluster/podnetworkinstance.yaml")
			gomega.Expect(err).To(gomega.BeNil())

			// Step 3: Create pods on first two nodes
			ginkgo.By("Getting cluster nodes")
			nodes := helpers.GetClusterNodes(kubeconfig2)
			gomega.Expect(len(nodes)).To(gomega.BeNumerically(">=", 2), "Need at least 2 nodes")

			podNames := []string{}
			for i, node := range nodes[:2] {
				podName := fmt.Sprintf("pod-c2-%d", i)
				podNames = append(podNames, podName)

				ginkgo.By(fmt.Sprintf("Creating pod %s on node %s", podName, node))
				err := CreatePod(kubeconfig2, PodData{
					PodName:  podName,
					NodeName: node,
					OS:       "linux",
					PNName:   pnName,
					PNIName:  pniName,
					Image:    "weibeld/ubuntu-networking",
				}, "../../manifests/swiftv2/long-running-cluster/pod.yaml")
				gomega.Expect(err).To(gomega.BeNil())
			}

			// Step 4: Wait for 5 minutes
			ginkgo.By("Waiting for 5 minutes before deletion")
			time.Sleep(5 * time.Minute)

			// Step 5: Delete resources in reverse order
			ginkgo.By("Deleting pods")
			for _, podName := range podNames {
				ginkgo.By(fmt.Sprintf("Deleting pod %s", podName))
				err := helpers.DeletePod(kubeconfig2, pnName, podName)
				gomega.Expect(err).To(gomega.BeNil())
			}

			ginkgo.By(fmt.Sprintf("Deleting PodNetworkInstance %s", pniName))
			err = helpers.DeletePodNetworkInstance(kubeconfig2, pnName, pniName)
			gomega.Expect(err).To(gomega.BeNil())

			ginkgo.By(fmt.Sprintf("Deleting PodNetwork %s", pnName))
			err = helpers.DeletePodNetwork(kubeconfig2, pnName)
			gomega.Expect(err).To(gomega.BeNil())

			ginkgo.By(fmt.Sprintf("Deleting namespace %s", pnName))
			err = helpers.DeleteNamespace(kubeconfig2, pnName)
			gomega.Expect(err).To(gomega.BeNil())

			ginkgo.By(fmt.Sprintf("Completed iteration %d at %s", iteration, time.Now().Format(time.RFC3339)))

			// Step 6: Wait for 30 minutes before next iteration
			ginkgo.By("Waiting for 30 minutes before next iteration")
			time.Sleep(30 * time.Minute)
		}
	})
})
