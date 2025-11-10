package long_running_cluster

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/Azure/azure-container-networking/test/integration/swiftv2/helpers"
)

func TestDatapath(t *testing.T) {
	RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Datapath Suite")
}

var _ = ginkgo.Describe("Datapath Tests", func() {
	RG := os.Getenv("RG")
	BUILD_ID := os.Getenv("BUILD_ID")

	if RG == "" || BUILD_ID == "" {
		ginkgo.Fail(fmt.Sprintf("Missing required environment variables: RG='%s', BUILD_ID='%s'", RG, BUILD_ID))
	}

	CLUSTER2 := "aks-2"
	KUBECONFIG2 := fmt.Sprintf("/tmp/%s.kubeconfig", CLUSTER2)
	PN_NAME := fmt.Sprintf("pn-%s-c2", BUILD_ID)
	PNI_NAME := fmt.Sprintf("pni-%s-c2", BUILD_ID)

	ginkgo.It("creates and deletes PodNetwork, PodNetworkInstance, and Pods in a loop", func() {
		vnetName := "cx_vnet_b1"
		subnetName := "s1"

		// Get subnet information once (doesn't change between iterations)
		vnetGUID := helpers.GetVnetGUID(RG, vnetName)
		subnetGUID := helpers.GetSubnetGUID(RG, vnetName, subnetName)
		subnetARMID := helpers.GetSubnetARMID(RG, vnetName, subnetName)
		subnetToken := helpers.GetSubnetToken(RG, vnetName, subnetName)

		// Run indefinitely until test is cancelled
		iteration := 0
		for {
			iteration++
			ginkgo.By(fmt.Sprintf("Starting iteration %d at %s", iteration, time.Now().Format(time.RFC3339)))

			// Step 1: Create PodNetwork
			ginkgo.By(fmt.Sprintf("Creating PodNetwork %s", PN_NAME))
			err := CreatePodNetwork(KUBECONFIG2, PodNetworkData{
				PNName:      PN_NAME,
				VnetGUID:    vnetGUID,
				SubnetGUID:  subnetGUID,
				SubnetARMID: subnetARMID,
				SubnetToken: subnetToken,
			}, "../../manifests/swiftv2/long-running-cluster/podnetwork.yaml")
			Expect(err).To(BeNil())

			// Step 2: Create namespace and PodNetworkInstance
			ginkgo.By(fmt.Sprintf("Creating namespace %s", PN_NAME))
			err = helpers.EnsureNamespaceExists(KUBECONFIG2, PN_NAME)
			Expect(err).To(BeNil())

			ginkgo.By(fmt.Sprintf("Creating PodNetworkInstance %s", PNI_NAME))
			err = CreatePodNetworkInstance(KUBECONFIG2, PNIData{
				PNIName:      PNI_NAME,
				PNName:       PN_NAME,
				Namespace:    PN_NAME,
				Type:         "explicit",
				Reservations: 2,
			}, "../../manifests/swiftv2/long-running-cluster/podnetworkinstance.yaml")
			Expect(err).To(BeNil())

			// Step 3: Create pods on first two nodes
			ginkgo.By("Getting cluster nodes")
			nodes := helpers.GetClusterNodes(KUBECONFIG2)
			Expect(len(nodes)).To(BeNumerically(">=", 2), "Need at least 2 nodes")

			podNames := []string{}
			for i, node := range nodes[:2] {
				podName := fmt.Sprintf("pod-c2-%d", i)
				podNames = append(podNames, podName)

				ginkgo.By(fmt.Sprintf("Creating pod %s on node %s", podName, node))
				err := CreatePod(KUBECONFIG2, PodData{
					PodName:  podName,
					NodeName: node,
					OS:       "linux",
					PNName:   PN_NAME,
					PNIName:  PNI_NAME,
					Image:    "weibeld/ubuntu-networking",
				}, "../../manifests/swiftv2/long-running-cluster/pod.yaml")
				Expect(err).To(BeNil())
			}

			// Step 4: Wait for 5 minutes
			ginkgo.By("Waiting for 5 minutes before deletion")
			time.Sleep(5 * time.Minute)

			// Step 5: Delete resources in reverse order
			ginkgo.By("Deleting pods")
			for _, podName := range podNames {
				ginkgo.By(fmt.Sprintf("Deleting pod %s", podName))
				err := helpers.DeletePod(KUBECONFIG2, PN_NAME, podName)
				Expect(err).To(BeNil())
			}

			ginkgo.By(fmt.Sprintf("Deleting PodNetworkInstance %s", PNI_NAME))
			err = helpers.DeletePodNetworkInstance(KUBECONFIG2, PN_NAME, PNI_NAME)
			Expect(err).To(BeNil())

			ginkgo.By(fmt.Sprintf("Deleting PodNetwork %s", PN_NAME))
			err = helpers.DeletePodNetwork(KUBECONFIG2, PN_NAME)
			Expect(err).To(BeNil())

			ginkgo.By(fmt.Sprintf("Deleting namespace %s", PN_NAME))
			err = helpers.DeleteNamespace(KUBECONFIG2, PN_NAME)
			Expect(err).To(BeNil())

			ginkgo.By(fmt.Sprintf("Completed iteration %d at %s", iteration, time.Now().Format(time.RFC3339)))

			// Step 6: Wait for 30 minutes before next iteration
			ginkgo.By("Waiting for 30 minutes before next iteration")
			time.Sleep(30 * time.Minute)
		}
	})
})
