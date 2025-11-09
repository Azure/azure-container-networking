package long_running_cluster

import (
	"fmt"
	"testing"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/Azure/azure-container-networking/test/integration/swiftv2/helpers"
)

func TestDatapath(t *testing.T) {
	RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Datapath Suite")
}

var _ = ginkgo.Describe("Datapath Tests", func() {
	RG := "siglin-143139088-westus2"
	BUILD_ID := "001"
	CLUSTER2 := "aks-2"
	KUBECONFIG2 := fmt.Sprintf("/tmp/%s.kubeconfig", CLUSTER2)

	PN_NAME := fmt.Sprintf("pn-%s-c2", BUILD_ID)
	PNI_NAME := fmt.Sprintf("pni-%s-c2", BUILD_ID)

	ginkgo.It("creates PodNetwork", func() {
		vnetName := "cx_vnet_b1"
		subnetName := "s1"

		vnetGUID := helpers.GetVnetGUID(RG, vnetName)
		subnetGUID := helpers.GetSubnetGUID(RG, vnetName, subnetName)
		subnetARMID := helpers.GetSubnetARMID(RG, vnetName, subnetName)
		subnetToken := helpers.GetSubnetToken(RG, vnetName, subnetName)

		err := CreatePodNetwork(KUBECONFIG2, PodNetworkData{
			PNName:      PN_NAME,
			VnetGUID:    vnetGUID,
			SubnetGUID:  subnetGUID,
			SubnetARMID: subnetARMID,
			SubnetToken: subnetToken,
		}, "./templates/podnetwork.yaml.tmpl")
		Expect(err).To(BeNil())
	})

	ginkgo.It("creates PodNetworkInstance", func() {
		err := CreatePodNetworkInstance(KUBECONFIG2, PNIData{
			PNIName:      PNI_NAME,
			PNName:       PN_NAME,
			Namespace:    PN_NAME, // namespace same as PN
			Type:         "explicit",
			Reservations: 2,
		}, "./templates/podnetworkinstance.yaml.tmpl")
		Expect(err).To(BeNil())
	})

	ginkgo.It("creates pods on each node", func() {
		nodes := helpers.GetClusterNodes(KUBECONFIG2)
		Expect(len(nodes)).To(BeNumerically(">", 0))

		for i, node := range nodes[:2] {
			podName := fmt.Sprintf("pod-c2-%d", i)
			err := CreatePod(KUBECONFIG2, PodData{
				PodName:  podName,
				NodeName: node,
				OS:       "linux",
				PNName:   PN_NAME,
				PNIName:  PNI_NAME,
				Image:    "weibeld/ubuntu-networking",
			}, "./templates/pod.yaml.tmpl")
			Expect(err).To(BeNil())
		}
	})
})
