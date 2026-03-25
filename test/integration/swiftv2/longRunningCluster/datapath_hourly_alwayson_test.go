//go:build hourly_alwayson_test
// +build hourly_alwayson_test

package longrunningcluster

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/test/integration/swiftv2/helpers"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

func TestHourlyAlwaysOn(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Hourly Always-On DaemonSet Suite")
}

// ensureAlwaysOnPNAndPNI ensures the PodNetwork and PodNetworkInstance exist for always-on pods.
func ensureAlwaysOnPNAndPNI(kubeconfig, rg, pnName, pniName, namespace string) {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "podnetwork", pnName, "--no-headers", "--ignore-not-found")
	out, _ := cmd.CombinedOutput()
	if strings.TrimSpace(string(out)) != "" {
		fmt.Printf("PodNetwork %s already exists, reusing\n", pnName)
	} else {
		fmt.Printf("Creating PodNetwork %s\n", pnName)
		info, err := GetOrFetchVnetSubnetInfo(rg, "cx_vnet_v1", "s1", make(map[string]VnetSubnetInfo))
		gomega.Expect(err).To(gomega.BeNil(), "Failed to get VNet/Subnet info for always-on PN")
		err = CreatePodNetwork(kubeconfig, PodNetworkData{
			PNName:      pnName,
			VnetGUID:    info.VnetGUID,
			SubnetGUID:  info.SubnetGUID,
			SubnetARMID: info.SubnetARMID,
		}, "../../manifests/swiftv2/long-running-cluster/podnetwork.yaml")
		gomega.Expect(err).To(gomega.BeNil(), "Failed to create PodNetwork")
	}

	cmd = exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "podnetworkinstance", pniName,
		"-n", namespace, "--no-headers", "--ignore-not-found")
	out, _ = cmd.CombinedOutput()
	if strings.TrimSpace(string(out)) != "" {
		fmt.Printf("PodNetworkInstance %s already exists, reusing\n", pniName)
	} else {
		fmt.Printf("Creating PodNetworkInstance %s in namespace %s\n", pniName, namespace)
		err := CreatePodNetworkInstance(kubeconfig, PNIData{
			PNIName:      pniName,
			PNName:       pnName,
			Namespace:    namespace,
			Reservations: 2, // 1 DaemonSet pod + 1 buffer
		}, "../../manifests/swiftv2/long-running-cluster/podnetworkinstance.yaml")
		gomega.Expect(err).To(gomega.BeNil(), "Failed to create PodNetworkInstance")
	}
}

// isDaemonSetExists checks if the DaemonSet already exists.
func isDaemonSetExists(kubeconfig, namespace, dsName string) bool {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "daemonset", dsName,
		"-n", namespace, "--no-headers", "--ignore-not-found")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

var _ = ginkgo.Describe("Hourly Always-On DaemonSet Tests", func() {
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

		// Zone-scoped resource names
		namespace := GetZonedAlwaysOnNS(buildID)
		pnName := GetZonedPNName(HourlyAlwaysOnPNPrefix, buildID)
		pniName := GetZonedPNIName(HourlyAlwaysOnPNIPrefix, buildID)
		dsName := GetDaemonSetName()
		zoneLabel := GetZoneLabel(location)
		if zoneLabel == "" {
			ginkgo.Fail(fmt.Sprintf("Missing zone label for always-on DaemonSet. Ensure ZONE and LOCATION are set correctly (LOCATION='%s')", location))
		}

		// Ensure namespace exists
		err := helpers.EnsureNamespaceExists(kubeconfig, namespace)
		gomega.Expect(err).To(gomega.BeNil(), "Failed to ensure namespace exists")

		// Ensure PodNetwork and PodNetworkInstance exist
		ensureAlwaysOnPNAndPNI(kubeconfig, rg, pnName, pniName, namespace)

		// Ensure DaemonSet exists
		if isDaemonSetExists(kubeconfig, namespace, dsName) {
			fmt.Printf("DaemonSet %s already exists, verifying pod\n", dsName)
		} else {
			fmt.Printf("Creating DaemonSet %s in namespace %s (zone label: %s)\n", dsName, namespace, zoneLabel)
			err := CreateDaemonSet(kubeconfig, DaemonSetData{
				DaemonSetName: dsName,
				Namespace:     namespace,
				PNIName:       pniName,
				PNName:        pnName,
				ZoneLabel:     zoneLabel,
				Image:         podImage,
			}, "../../manifests/swiftv2/long-running-cluster/daemonset.yaml")
			gomega.Expect(err).To(gomega.BeNil(), "Failed to create DaemonSet")
		}

		// Wait for DaemonSet pod to be running
		fmt.Printf("Waiting for DaemonSet %s pod to be ready\n", dsName)
		err = helpers.WaitForDaemonSetReady(kubeconfig, namespace, dsName, 10, 30)
		gomega.Expect(err).To(gomega.BeNil(), fmt.Sprintf("DaemonSet %s pod is not running", dsName))

		// Verify the DaemonSet pod exists and is running
		podName := GetDaemonSetPodName(kubeconfig, namespace, dsName)
		gomega.Expect(podName).NotTo(gomega.BeEmpty(), "DaemonSet pod not found")
		gomega.Expect(IsPodRunning(kubeconfig, namespace, podName)).To(gomega.BeTrue(),
			fmt.Sprintf("DaemonSet pod %s is not running", podName))

		fmt.Printf("Always-on DaemonSet pod %s is running in zone %s\n", podName, zone)
		ginkgo.By(fmt.Sprintf("DaemonSet always-on pod verified in zone %s", zone))
	})
})
