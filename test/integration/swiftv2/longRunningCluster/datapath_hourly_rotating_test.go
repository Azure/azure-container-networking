//go:build hourly_rotating_test
// +build hourly_rotating_test

package longrunningcluster

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/test/integration/swiftv2/helpers"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

func TestHourlyRotating(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Hourly Rotating Pod Suite")
}

const rotatingPodMaxAge = 6 * time.Hour

// getPodCreationTime gets the created-at annotation from a pod to determine its age.
func getPodCreationTime(kubeconfig, namespace, podName string) (time.Time, error) {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "pod", podName,
		"-n", namespace, "-o", fmt.Sprintf("jsonpath={.metadata.annotations['%s']}", HourlyCreatedAtAnnotation))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get pod annotation: %w\nOutput: %s", err, string(out))
	}

	timeStr := strings.TrimSpace(string(out))
	if timeStr == "" {
		// Fall back to pod creation timestamp
		cmd = exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "pod", podName,
			"-n", namespace, "-o", "jsonpath={.metadata.creationTimestamp}")
		out, err = cmd.CombinedOutput()
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to get pod creation timestamp: %w", err)
		}
		timeStr = strings.TrimSpace(string(out))
	}

	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse time '%s': %w", timeStr, err)
	}
	return t, nil
}

// deleteRotatingPod deletes a single pod with MTPNC cleanup wait.
func deleteRotatingPod(kubeconfig, namespace, podName string) error {
	fmt.Printf("Deleting rotating pod %s in namespace %s\n", podName, namespace)
	err := helpers.DeletePod(kubeconfig, namespace, podName)
	if err != nil {
		return fmt.Errorf("failed to delete pod %s: %w", podName, err)
	}
	if err := helpers.WaitForMTPNCCleanup(kubeconfig, namespace, 120); err != nil {
		fmt.Printf("Warning: MTPNC cleanup didn't complete for pod %s: %v\n", podName, err)
	}
	return nil
}

// createRotatingPod creates a single rotating pod with a created-at annotation.
func createRotatingPod(kubeconfig, namespace, pniName, pnName, nodeName, podName, podImage string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	err := CreatePod(kubeconfig, PodData{
		PodName:   podName,
		NodeName:  nodeName,
		OS:        "linux",
		PNName:    pnName,
		PNIName:   pniName,
		Namespace: namespace,
		Image:     podImage,
	}, "../../manifests/swiftv2/long-running-cluster/pod.yaml")
	if err != nil {
		return fmt.Errorf("failed to create pod %s: %w", podName, err)
	}

	// Annotate with creation time
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "annotate", "pod", podName,
		"-n", namespace, fmt.Sprintf("%s=%s", HourlyCreatedAtAnnotation, now), "--overwrite")
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Warning: failed to annotate pod %s: %s\n", podName, string(out))
	}

	err = helpers.WaitForPodRunning(kubeconfig, namespace, podName, 10, 30)
	if err != nil {
		return fmt.Errorf("pod %s did not reach running state: %w", podName, err)
	}

	fmt.Printf("Created rotating pod %s at %s on node %s\n", podName, now, nodeName)
	return nil
}

// ensureRotatingPNAndPNI ensures the PodNetwork and PodNetworkInstance exist for rotating pods.
func ensureRotatingPNAndPNI(kubeconfig, rg, pnName, pniName, namespace string) {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "podnetwork", pnName, "--no-headers", "--ignore-not-found")
	out, _ := cmd.CombinedOutput()
	if strings.TrimSpace(string(out)) != "" {
		fmt.Printf("PodNetwork %s already exists, reusing\n", pnName)
	} else {
		fmt.Printf("Creating PodNetwork %s\n", pnName)
		info, err := GetOrFetchVnetSubnetInfo(rg, "cx_vnet_v1", "s1", make(map[string]VnetSubnetInfo))
		gomega.Expect(err).To(gomega.BeNil(), "Failed to get VNet/Subnet info for rotating PN")
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
		fmt.Printf("Creating PodNetworkInstance %s\n", pniName)
		err := CreatePodNetworkInstance(kubeconfig, PNIData{
			PNIName:      pniName,
			PNName:       pnName,
			Namespace:    namespace,
			Reservations: HourlyRotatingPodCount + 1,
		}, "../../manifests/swiftv2/long-running-cluster/podnetworkinstance.yaml")
		gomega.Expect(err).To(gomega.BeNil(), "Failed to create PodNetworkInstance")
	}
}

var _ = ginkgo.Describe("Hourly Rotating Pod Tests", func() {
	ginkgo.It("rotates pods on the zone high-NIC node (6h lifetime, 1 per hour)", func() {
		rg := os.Getenv("RG")
		buildID := os.Getenv("BUILD_ID")
		location := os.Getenv("LOCATION")
		if rg == "" || buildID == "" || location == "" {
			ginkgo.Fail(fmt.Sprintf("Missing required environment variables: RG='%s', BUILD_ID='%s', LOCATION='%s'", rg, buildID, location))
		}

		zone := GetZone()
		if zone != "" {
			fmt.Printf("Running rotating pod test for zone %s\n", zone)
		}

		kubeconfig := getKubeconfigPath("aks-1")
		podImage := "nicolaka/netshoot:latest"

		// Get the rotating node in this zone
		rotatingNode := GetNodeByLabel(kubeconfig, GetRotatingNodeSelector(location))
		gomega.Expect(rotatingNode).NotTo(gomega.BeEmpty(),
			fmt.Sprintf("No node found with selector: %s", GetRotatingNodeSelector(location)))

		// Confirm the node's zone
		nodeZone := GetNodeZone(kubeconfig, rotatingNode)
		fmt.Printf("Rotating node: %s (zone: %s)\n", rotatingNode, nodeZone)

		// Zone-scoped resource names
		namespace := GetZonedRotatingNS(buildID)
		pnName := GetZonedPNName(HourlyRotatingPNPrefix, buildID)
		pniName := GetZonedPNIName(HourlyRotatingPNIPrefix, buildID)

		// Ensure namespace exists
		err := helpers.EnsureNamespaceExists(kubeconfig, namespace)
		gomega.Expect(err).To(gomega.BeNil(), "Failed to ensure namespace exists")

		// Ensure PodNetwork and PodNetworkInstance exist (reuse across runs)
		ensureRotatingPNAndPNI(kubeconfig, rg, pnName, pniName, namespace)

		// Scan existing pods: find which slots are occupied and their ages
		now := time.Now().UTC()
		deletedCount := 0
		createdCount := 0
		existingSlots := make(map[int]bool)

		for slot := 0; slot < HourlyRotatingPodCount; slot++ {
			podName := GetRotatingPodName(slot)
			if !IsPodExists(kubeconfig, namespace, podName) {
				continue
			}
			existingSlots[slot] = true

			// Check age - delete if older than 6 hours
			createdAt, err := getPodCreationTime(kubeconfig, namespace, podName)
			if err != nil {
				fmt.Printf("Cannot determine age of pod %s, deleting: %v\n", podName, err)
				delErr := deleteRotatingPod(kubeconfig, namespace, podName)
				gomega.Expect(delErr).To(gomega.BeNil(), fmt.Sprintf("Failed to delete aged-out pod %s", podName))
				existingSlots[slot] = false
				deletedCount++
				continue
			}

			age := now.Sub(createdAt)
			fmt.Printf("Pod %s age: %v (created at %s)\n", podName, age.Round(time.Minute), createdAt.Format(time.RFC3339))

			if age > rotatingPodMaxAge {
				fmt.Printf("Pod %s exceeded max age (%v > %v), deleting\n", podName, age.Round(time.Minute), rotatingPodMaxAge)
				delErr := deleteRotatingPod(kubeconfig, namespace, podName)
				gomega.Expect(delErr).To(gomega.BeNil(), fmt.Sprintf("Failed to delete aged-out pod %s", podName))
				existingSlots[slot] = false
				deletedCount++
			}
		}

		// Ensure at least 1 pod is rotated per hour even if none expired
		if deletedCount == 0 {
			oldestSlot := -1
			var oldestTime time.Time

			for slot := 0; slot < HourlyRotatingPodCount; slot++ {
				if !existingSlots[slot] {
					continue
				}
				podName := GetRotatingPodName(slot)
				createdAt, err := getPodCreationTime(kubeconfig, namespace, podName)
				if err != nil {
					continue
				}
				if oldestSlot == -1 || createdAt.Before(oldestTime) {
					oldestSlot = slot
					oldestTime = createdAt
				}
			}

			if oldestSlot >= 0 {
				podName := GetRotatingPodName(oldestSlot)
				fmt.Printf("Rotating oldest pod %s (age: %v) to ensure at least 1 rotation per hour\n",
					podName, now.Sub(oldestTime).Round(time.Minute))
				delErr := deleteRotatingPod(kubeconfig, namespace, podName)
				gomega.Expect(delErr).To(gomega.BeNil(), fmt.Sprintf("Failed to delete oldest pod %s", podName))
				existingSlots[oldestSlot] = false
				deletedCount++
			}
		}

		// Create pods for all empty slots
		for slot := 0; slot < HourlyRotatingPodCount; slot++ {
			if existingSlots[slot] {
				continue
			}
			podName := GetRotatingPodName(slot)
			fmt.Printf("Creating pod %s in slot %d\n", podName, slot)
			err := createRotatingPod(kubeconfig, namespace, pniName, pnName, rotatingNode, podName, podImage)
			gomega.Expect(err).To(gomega.BeNil(), fmt.Sprintf("Failed to create rotating pod %s", podName))
			createdCount++
		}

		fmt.Printf("\nRotating pod summary (zone %s): deleted=%d, created=%d\n", zone, deletedCount, createdCount)

		// Verify all 6 pods are running
		for slot := 0; slot < HourlyRotatingPodCount; slot++ {
			podName := GetRotatingPodName(slot)
			gomega.Expect(IsPodRunning(kubeconfig, namespace, podName)).To(gomega.BeTrue(),
				fmt.Sprintf("Pod %s is not running after rotation", podName))
		}

		ginkgo.By(fmt.Sprintf("All 6 rotating pods are running in zone %s", zone))
	})
})
