//go:build longrunning_rotating_test || longrunning_alwayson_test || longrunning_connectivity_test

package longrunningcluster

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Azure/azure-container-networking/test/integration/swiftv2/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Shared constants for long-running pod tests (rotating + always-on DaemonSet).
// These are defined in this shared file and are available to all long-running
// tests built with the longrunning_* build tags.
const (
	LongRunningRotatingPodCount    = 6
	LongRunningRotatingPodPrefix   = "pod-rotating-"
	LongRunningRotatingNSPrefix    = "ns-rotating"
	LongRunningAlwaysOnNSPrefix    = "ns-alwayson"
	LongRunningRotatingPNPrefix    = "pn-rotating"
	LongRunningAlwaysOnPNPrefix    = "pn-alwayson"
	LongRunningRotatingPNIPrefix   = "pni-rotating"
	LongRunningAlwaysOnPNIPrefix   = "pni-alwayson"
	LongRunningCreatedAtAnnotation = "acn-test/created-at"
	LongRunningDaemonSetPrefix     = "ds-alwayson"
)

// GetZone returns the ZONE environment variable (e.g., "1", "2", "3", "4").
// Tests use this to select zone-specific nodes and create zone-scoped resources.
func GetZone() string {
	return os.Getenv("ZONE")
}

// GetZoneSuffix returns "-z<ZONE>" if ZONE is set, empty string otherwise.
// Used to create zone-scoped resource names.
func GetZoneSuffix() string {
	zone := GetZone()
	if zone == "" {
		return ""
	}
	return "-z" + zone
}

// GetRotatingPodName returns the pod name for a given rotating slot index (0-5).
func GetRotatingPodName(slot int) string {
	return fmt.Sprintf("%s%d", LongRunningRotatingPodPrefix, slot)
}

// GetZonedRotatingNS returns the zone-scoped namespace for rotating pods.
func GetZonedRotatingNS(buildID string) string {
	return fmt.Sprintf("%s%s-%s", LongRunningRotatingNSPrefix, GetZoneSuffix(), buildID)
}

// GetZonedAlwaysOnNS returns the zone-scoped namespace for always-on pods.
func GetZonedAlwaysOnNS(buildID string) string {
	return fmt.Sprintf("%s%s-%s", LongRunningAlwaysOnNSPrefix, GetZoneSuffix(), buildID)
}

// GetZonedPNName returns a zone-scoped PodNetwork name.
func GetZonedPNName(prefix, buildID string) string {
	return fmt.Sprintf("%s%s-%s", prefix, GetZoneSuffix(), buildID)
}

// GetZonedPNIName returns a zone-scoped PodNetworkInstance name.
func GetZonedPNIName(prefix, buildID string) string {
	return fmt.Sprintf("%s%s-%s", prefix, GetZoneSuffix(), buildID)
}

// GetRotatingNodeSelector returns the label selector for the zone's node.
// Each zone has 1 node labeled longrunning-zone-pool=true with the AKS zone label.
func GetRotatingNodeSelector(location string) string {
	zone := GetZone()
	if zone == "" {
		return "longrunning-zone-pool=true"
	}
	return fmt.Sprintf("longrunning-zone-pool=true,topology.kubernetes.io/zone=%s-%s", location, zone)
}

// GetAlwaysOnNodeSelector returns the same selector as GetRotatingNodeSelector
// since both rotating pods and the DaemonSet always-on pod share the same node.
func GetAlwaysOnNodeSelector(location string) string {
	return GetRotatingNodeSelector(location)
}

// GetDaemonSetName returns the zone-scoped DaemonSet name.
func GetDaemonSetName() string {
	return fmt.Sprintf("%s%s", LongRunningDaemonSetPrefix, GetZoneSuffix())
}

// GetDaemonSetPodName finds the DaemonSet pod name in the given namespace.
func GetDaemonSetPodName(kubeconfig, namespace, dsName string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := helpers.MustGetK8sClient(kubeconfig)
	name, err := helpers.GetDaemonSetPodName(ctx, c, namespace, dsName)
	if err != nil {
		fmt.Printf("Warning: GetDaemonSetPodName(%s/%s): %v\n", namespace, dsName, err)
	}
	return name
}

// GetZoneLabel returns the full zone label value (e.g., "eastus2euap-1").
func GetZoneLabel(location string) string {
	zone := GetZone()
	if zone == "" {
		return ""
	}
	return fmt.Sprintf("%s-%s", location, zone)
}

// IsPodExists checks if a pod exists in the namespace.
func IsPodExists(kubeconfig, namespace, podName string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := helpers.MustGetK8sClient(kubeconfig)
	pod := &corev1.Pod{}
	err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: podName}, pod)
	return err == nil
}

// IsPodRunning checks if a pod is in Running phase.
func IsPodRunning(kubeconfig, namespace, podName string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := helpers.MustGetK8sClient(kubeconfig)
	pod := &corev1.Pod{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: podName}, pod); err != nil {
		return false
	}
	return pod.Status.Phase == corev1.PodRunning
}

// GetNodeByLabel returns the first node matching the given label selector.
func GetNodeByLabel(kubeconfig, labelSelector string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := helpers.MustGetK8sClient(kubeconfig)
	name, err := helpers.GetNodeByLabels(ctx, c, labelSelector)
	if err != nil {
		fmt.Printf("Warning: GetNodeByLabel(%s): %v\n", labelSelector, err)
	}
	return name
}

// IsDeploymentExists checks if a deployment exists in the namespace.
func IsDeploymentExists(kubeconfig, namespace, deploymentName string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := helpers.MustGetK8sClient(kubeconfig)
	exists, err := helpers.DeploymentExists(ctx, c, namespace, deploymentName)
	if err != nil {
		fmt.Printf("Warning: IsDeploymentExists(%s/%s): %v\n", namespace, deploymentName, err)
	}
	return exists
}

// IsDeploymentReady checks if a deployment has its desired replicas ready.
func IsDeploymentReady(kubeconfig, namespace, deploymentName string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := helpers.MustGetK8sClient(kubeconfig)
	ready, err := helpers.IsDeploymentReady(ctx, c, namespace, deploymentName)
	if err != nil {
		fmt.Printf("Warning: IsDeploymentReady(%s/%s): %v\n", namespace, deploymentName, err)
	}
	return ready
}

// GetDeploymentPodName returns the name of the pod managed by a deployment.
func GetDeploymentPodName(kubeconfig, namespace, deploymentName string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := helpers.MustGetK8sClient(kubeconfig)
	name, err := helpers.GetDeploymentPodName(ctx, c, namespace, deploymentName)
	if err != nil {
		fmt.Printf("Warning: GetDeploymentPodName(%s/%s): %v\n", namespace, deploymentName, err)
	}
	return name
}

// GetNodeZone returns the zone label value for a given node.
func GetNodeZone(kubeconfig, nodeName string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := helpers.MustGetK8sClient(kubeconfig)
	zone, err := helpers.GetNodeZoneLabel(ctx, c, nodeName)
	if err != nil {
		fmt.Printf("Warning: GetNodeZone(%s): %v\n", nodeName, err)
	}
	return zone
}
