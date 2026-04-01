//go:build longrunning_alwayson_test
// +build longrunning_alwayson_test

package longrunningcluster

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/test/integration/swiftv2/helpers"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

func TestLongRunningAlwaysOn(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Long-Running Always-On DaemonSet Suite")
}

// ensureAlwaysOnPNAndPNI ensures the PodNetwork and PodNetworkInstance exist for always-on pods.
func ensureAlwaysOnPNAndPNI(kubeconfig, rg, pnName, pniName, namespace string) {
	ctx := context.Background()
	c := helpers.MustGetK8sClient(kubeconfig)

	exists, err := helpers.PodNetworkExists(ctx, c, pnName)
	gomega.Expect(err).To(gomega.BeNil(), "Failed to check PodNetwork existence")
	if exists {
		fmt.Printf("PodNetwork %s already exists, reusing\n", pnName)
	} else {
		fmt.Printf("Creating PodNetwork %s\n", pnName)
		info, infoErr := GetOrFetchVnetSubnetInfo(rg, "cx_vnet_v1", "lr", make(map[string]VnetSubnetInfo))
		gomega.Expect(infoErr).To(gomega.BeNil(), "Failed to get VNet/Subnet info for always-on PN")
		createErr := helpers.CreatePodNetworkCR(ctx, c, pnName, info.VnetGUID, info.SubnetGUID, info.SubnetARMID)
		gomega.Expect(createErr).To(gomega.BeNil(), "Failed to create PodNetwork")
	}

	exists, err = helpers.PodNetworkInstanceExists(ctx, c, namespace, pniName)
	gomega.Expect(err).To(gomega.BeNil(), "Failed to check PodNetworkInstance existence")
	if exists {
		fmt.Printf("PodNetworkInstance %s already exists, reusing\n", pniName)
	} else {
		fmt.Printf("Creating PodNetworkInstance %s in namespace %s\n", pniName, namespace)
		createErr := helpers.CreatePodNetworkInstanceCR(ctx, c, pniName, namespace, pnName, 0)
		gomega.Expect(createErr).To(gomega.BeNil(), "Failed to create PodNetworkInstance")
	}
}

var _ = ginkgo.Describe("Long-Running Always-On DaemonSet Tests", func() {
	ginkgo.It("ensures the always-on DaemonSet is running on the zone node", func() {
		rg := os.Getenv("RG")
		buildID := os.Getenv("BUILD_ID")
		location := os.Getenv("LOCATION")
		if rg == "" || buildID == "" || location == "" {
			ginkgo.Fail(fmt.Sprintf("Missing required environment variables: RG='%s', BUILD_ID='%s', LOCATION='%s'", rg, buildID, location))
		}

		zone := GetZone()
		if zone != "" {
			fmt.Printf("Running always-on DaemonSet test for zone %s\n", zone)
		}

		kubeconfig := getKubeconfigPath("aks-1")
		podImage := "nicolaka/netshoot:latest"
		ctx := context.Background()
		c := helpers.MustGetK8sClient(kubeconfig)

		// Zone-scoped resource names
		namespace := GetZonedAlwaysOnNS(buildID)
		pnName := GetZonedPNName(LongRunningAlwaysOnPNPrefix, buildID)
		pniName := GetZonedPNIName(LongRunningAlwaysOnPNIPrefix, buildID)
		dsName := GetDaemonSetName()
		zoneLabel := GetZoneLabel(location)
		if zoneLabel == "" {
			ginkgo.Fail(fmt.Sprintf("Missing zone label for always-on DaemonSet. Ensure ZONE and LOCATION are set correctly (LOCATION='%s')", location))
		}

		// Ensure namespace exists
		err := helpers.EnsureNamespaceK8s(ctx, c, namespace)
		gomega.Expect(err).To(gomega.BeNil(), "Failed to ensure namespace exists")

		// Ensure PodNetwork and PodNetworkInstance exist
		ensureAlwaysOnPNAndPNI(kubeconfig, rg, pnName, pniName, namespace)

		// Ensure DaemonSet exists
		exists, dsErr := helpers.DaemonSetExists(ctx, c, namespace, dsName)
		gomega.Expect(dsErr).To(gomega.BeNil(), "Failed to check DaemonSet existence")
		if exists {
			fmt.Printf("DaemonSet %s already exists, verifying pod\n", dsName)
		} else {
			fmt.Printf("Creating DaemonSet %s in namespace %s (zone label: %s)\n", dsName, namespace, zoneLabel)
			ds := helpers.CreateDaemonSetObject(helpers.LongrunningDaemonSetData{
				DaemonSetName: dsName,
				Namespace:     namespace,
				PNIName:       pniName,
				PNName:        pnName,
				ZoneLabel:     zoneLabel,
				Image:         podImage,
			})
			createErr := c.Create(ctx, ds)
			gomega.Expect(createErr).To(gomega.BeNil(), "Failed to create DaemonSet")
		}

		// Wait for DaemonSet pod to be running
		fmt.Printf("Waiting for DaemonSet %s pod to be ready\n", dsName)
		err = helpers.WaitForDaemonSetReady(ctx, c, namespace, dsName, 10, 30)
		gomega.Expect(err).To(gomega.BeNil(), fmt.Sprintf("DaemonSet %s pod is not running", dsName))

		// Verify the DaemonSet pod exists and is running
		podName := GetDaemonSetPodName(kubeconfig, namespace, dsName)
		gomega.Expect(podName).NotTo(gomega.BeEmpty(), "DaemonSet pod not found")
		gomega.Expect(IsPodRunning(kubeconfig, namespace, podName)).To(gomega.BeTrue(),
			fmt.Sprintf("DaemonSet pod %s is not running", podName))

		fmt.Printf("Always-on DaemonSet pod %s is running in zone %s\n", podName, zone)
		ginkgo.By("DaemonSet always-on pod verified in zone " + zone)
	})
})
