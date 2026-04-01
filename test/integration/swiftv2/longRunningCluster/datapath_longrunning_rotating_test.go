//go:build longrunning_rotating_test
// +build longrunning_rotating_test

package longrunningcluster

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/test/integration/swiftv2/helpers"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestLongRunningRotating(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Long-Running Rotating Pod Suite")
}

const rotatingPodMaxAge = 6 * time.Hour

// getDeploymentCreationTime gets the created-at annotation from a deployment's pod template.
func getDeploymentCreationTime(kubeconfig, namespace, deploymentName string) (time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := helpers.MustGetK8sClient(kubeconfig)

	dep := &appsv1.Deployment{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: deploymentName}, dep); err != nil {
		return time.Time{}, fmt.Errorf("failed to get deployment %s: %w", deploymentName, err)
	}

	timeStr := dep.Spec.Template.Annotations[LongRunningCreatedAtAnnotation]
	if timeStr == "" {
		// Fall back to deployment creation timestamp
		return dep.CreationTimestamp.Time, nil
	}

	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse time '%s': %w", timeStr, err)
	}
	return t, nil
}

// deleteRotatingDeployment deletes a single deployment with MTPNC cleanup wait.
func deleteRotatingDeployment(kubeconfig, namespace, deploymentName string) error {
	fmt.Printf("Deleting rotating deployment %s in namespace %s\n", deploymentName, namespace)
	ctx := context.Background()
	c := helpers.MustGetK8sClient(kubeconfig)

	if err := helpers.DeleteDeploymentAndWait(ctx, c, namespace, deploymentName, 90*time.Second); err != nil {
		return fmt.Errorf("failed to delete deployment %s: %w", deploymentName, err)
	}

	if err := helpers.WaitForMTPNCCleanupK8s(ctx, c, namespace, 120*time.Second); err != nil {
		fmt.Printf("Warning: MTPNC cleanup didn't complete for deployment %s: %v\n", deploymentName, err)
	}
	return nil
}

// createRotatingDeployment creates a single rotating deployment with a created-at annotation.
func createRotatingDeployment(kubeconfig, namespace, pniName, pnName, nodeName, deploymentName, podImage string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	ctx := context.Background()
	c := helpers.MustGetK8sClient(kubeconfig)

	dep := helpers.CreateDeploymentObject(helpers.LongrunningDeploymentData{
		DeploymentName: deploymentName,
		NodeName:       nodeName,
		PNName:         pnName,
		PNIName:        pniName,
		Namespace:      namespace,
		Image:          podImage,
		CreatedAt:      now,
	})

	if err := helpers.WithRetry(ctx, 3, 5*time.Second, func() error {
		return c.Create(ctx, dep)
	}); err != nil {
		return fmt.Errorf("failed to create deployment %s: %w", deploymentName, err)
	}

	if err := helpers.WaitForDeploymentReady(ctx, c, namespace, deploymentName, 10, 30); err != nil {
		return fmt.Errorf("deployment %s did not become ready: %w", deploymentName, err)
	}

	fmt.Printf("Created rotating deployment %s at %s on node %s\n", deploymentName, now, nodeName)
	return nil
}

// ensureRotatingPNAndPNI ensures the PodNetwork and PodNetworkInstance exist for rotating pods.
func ensureRotatingPNAndPNI(kubeconfig, rg, pnName, pniName, namespace string) {
	ctx := context.Background()
	c := helpers.MustGetK8sClient(kubeconfig)

	exists, err := helpers.PodNetworkExists(ctx, c, pnName)
	gomega.Expect(err).To(gomega.BeNil(), "Failed to check PodNetwork existence")
	if exists {
		fmt.Printf("PodNetwork %s already exists, reusing\n", pnName)
	} else {
		fmt.Printf("Creating PodNetwork %s\n", pnName)
		info, infoErr := GetOrFetchVnetSubnetInfo(rg, "cx_vnet_v1", "lr", make(map[string]VnetSubnetInfo))
		gomega.Expect(infoErr).To(gomega.BeNil(), "Failed to get VNet/Subnet info for rotating PN")
		err = helpers.CreatePodNetworkCR(ctx, c, pnName, info.VnetGUID, info.SubnetGUID, info.SubnetARMID)
		gomega.Expect(err).To(gomega.BeNil(), "Failed to create PodNetwork")
	}

	exists, err = helpers.PodNetworkInstanceExists(ctx, c, namespace, pniName)
	gomega.Expect(err).To(gomega.BeNil(), "Failed to check PodNetworkInstance existence")
	if exists {
		fmt.Printf("PodNetworkInstance %s already exists, reusing\n", pniName)
	} else {
		fmt.Printf("Creating PodNetworkInstance %s\n", pniName)
		err = helpers.CreatePodNetworkInstanceCR(ctx, c, pniName, namespace, pnName, 0)
		gomega.Expect(err).To(gomega.BeNil(), "Failed to create PodNetworkInstance")
	}
}

var _ = ginkgo.Describe("Long-Running Rotating Pod Tests", func() {
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
		c := helpers.MustGetK8sClient(kubeconfig)

		// Get the rotating node in this zone
		rotatingNode := GetNodeByLabel(kubeconfig, GetRotatingNodeSelector(location))
		gomega.Expect(rotatingNode).NotTo(gomega.BeEmpty(),
			"No node found with selector: "+GetRotatingNodeSelector(location))

		// Confirm the node's zone
		nodeZone := GetNodeZone(kubeconfig, rotatingNode)
		fmt.Printf("Rotating node: %s (zone: %s)\n", rotatingNode, nodeZone)

		// Zone-scoped resource names
		namespace := GetZonedRotatingNS(buildID)
		pnName := GetZonedPNName(LongRunningRotatingPNPrefix, buildID)
		pniName := GetZonedPNIName(LongRunningRotatingPNIPrefix, buildID)

		// Ensure namespace exists
		ctx := context.Background()
		err := helpers.EnsureNamespaceK8s(ctx, c, namespace)
		gomega.Expect(err).To(gomega.BeNil(), "Failed to ensure namespace exists")

		// Ensure PodNetwork and PodNetworkInstance exist (reuse across runs)
		ensureRotatingPNAndPNI(kubeconfig, rg, pnName, pniName, namespace)

		// Scan existing deployments: find which slots are occupied and their ages
		now := time.Now().UTC()
		deletedCount := 0
		createdCount := 0
		existingSlots := make(map[int]bool)

		for slot := 0; slot < LongRunningRotatingPodCount; slot++ {
			deploymentName := GetRotatingPodName(slot)
			if !IsDeploymentExists(kubeconfig, namespace, deploymentName) {
				continue
			}
			existingSlots[slot] = true

			// Self-heal: if the deployment is not Ready (e.g., node was replaced and the old
			// hostname selector no longer matches any node), delete and recreate it immediately
			// rather than waiting up to 6 hours for the age-based rotation to fire.
			if !IsDeploymentReady(kubeconfig, namespace, deploymentName) {
				fmt.Printf("Deployment %s is not Ready (node may have been replaced), deleting for self-heal\n", deploymentName)
				delErr := deleteRotatingDeployment(kubeconfig, namespace, deploymentName)
				gomega.Expect(delErr).To(gomega.BeNil(), "Failed to delete unready deployment "+deploymentName)
				existingSlots[slot] = false
				deletedCount++
				continue
			}

			// Check age - delete if older than 6 hours
			createdAt, err := getDeploymentCreationTime(kubeconfig, namespace, deploymentName)
			if err != nil {
				fmt.Printf("Cannot determine age of deployment %s, deleting: %v\n", deploymentName, err)
				delErr := deleteRotatingDeployment(kubeconfig, namespace, deploymentName)
				gomega.Expect(delErr).To(gomega.BeNil(), "Failed to delete aged-out deployment "+deploymentName)
				existingSlots[slot] = false
				deletedCount++
				continue
			}

			age := now.Sub(createdAt)
			fmt.Printf("Deployment %s age: %v (created at %s)\n", deploymentName, age.Round(time.Minute), createdAt.Format(time.RFC3339))

			if age > rotatingPodMaxAge {
				fmt.Printf("Deployment %s exceeded max age (%v > %v), deleting\n", deploymentName, age.Round(time.Minute), rotatingPodMaxAge)
				delErr := deleteRotatingDeployment(kubeconfig, namespace, deploymentName)
				gomega.Expect(delErr).To(gomega.BeNil(), "Failed to delete aged-out deployment "+deploymentName)
				existingSlots[slot] = false
				deletedCount++
			}
		}

		// Ensure at least 1 deployment is rotated per hour even if none expired
		if deletedCount == 0 {
			oldestSlot := -1
			var oldestTime time.Time

			for slot := 0; slot < LongRunningRotatingPodCount; slot++ {
				if !existingSlots[slot] {
					continue
				}
				deploymentName := GetRotatingPodName(slot)
				createdAt, err := getDeploymentCreationTime(kubeconfig, namespace, deploymentName)
				if err != nil {
					continue
				}
				if oldestSlot == -1 || createdAt.Before(oldestTime) {
					oldestSlot = slot
					oldestTime = createdAt
				}
			}

			if oldestSlot >= 0 {
				deploymentName := GetRotatingPodName(oldestSlot)
				fmt.Printf("Rotating oldest deployment %s (age: %v) to ensure at least 1 rotation per hour\n",
					deploymentName, now.Sub(oldestTime).Round(time.Minute))
				delErr := deleteRotatingDeployment(kubeconfig, namespace, deploymentName)
				gomega.Expect(delErr).To(gomega.BeNil(), "Failed to delete oldest deployment "+deploymentName)
				existingSlots[oldestSlot] = false
				deletedCount++
			}
		}

		// Create deployments for all empty slots
		for slot := 0; slot < LongRunningRotatingPodCount; slot++ {
			if existingSlots[slot] {
				continue
			}
			deploymentName := GetRotatingPodName(slot)
			fmt.Printf("Creating deployment %s in slot %d\n", deploymentName, slot)
			err := createRotatingDeployment(kubeconfig, namespace, pniName, pnName, rotatingNode, deploymentName, podImage)
			gomega.Expect(err).To(gomega.BeNil(), "Failed to create rotating deployment "+deploymentName)
			createdCount++
		}

		fmt.Printf("\nRotating deployment summary (zone %s): deleted=%d, created=%d\n", zone, deletedCount, createdCount)

		// Verify all 6 deployments are ready
		for slot := 0; slot < LongRunningRotatingPodCount; slot++ {
			deploymentName := GetRotatingPodName(slot)
			gomega.Expect(IsDeploymentReady(kubeconfig, namespace, deploymentName)).To(gomega.BeTrue(),
				"Deployment "+deploymentName+" is not ready after rotation")
		}

		ginkgo.By("All 6 rotating deployments are running in zone " + zone)
	})
})
