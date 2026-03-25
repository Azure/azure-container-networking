package longrunningcluster

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Shared constants for hourly pod tests (rotating + always-on DaemonSet).
// These are in a non-build-tagged file so they are available to all test files.
const (
	HourlyRotatingPodCount    = 6
	HourlyRotatingPodPrefix   = "pod-rotating-"
	HourlyRotatingNSPrefix    = "ns-rotating"
	HourlyAlwaysOnNSPrefix    = "ns-alwayson"
	HourlyRotatingPNPrefix    = "pn-rotating"
	HourlyAlwaysOnPNPrefix    = "pn-alwayson"
	HourlyRotatingPNIPrefix   = "pni-rotating"
	HourlyAlwaysOnPNIPrefix   = "pni-alwayson"
	HourlyCreatedAtAnnotation = "acn-test/created-at"
	HourlyDaemonSetPrefix     = "ds-alwayson"
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
	return fmt.Sprintf("%s%d", HourlyRotatingPodPrefix, slot)
}

// GetZonedRotatingNS returns the zone-scoped namespace for rotating pods.
func GetZonedRotatingNS(buildID string) string {
	return fmt.Sprintf("%s%s-%s", HourlyRotatingNSPrefix, GetZoneSuffix(), buildID)
}

// GetZonedAlwaysOnNS returns the zone-scoped namespace for always-on pods.
func GetZonedAlwaysOnNS(buildID string) string {
	return fmt.Sprintf("%s%s-%s", HourlyAlwaysOnNSPrefix, GetZoneSuffix(), buildID)
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
// Each zone has 1 node labeled hourly-zone-pool=true with the AKS zone label.
func GetRotatingNodeSelector(location string) string {
	zone := GetZone()
	if zone == "" {
		return "hourly-zone-pool=true"
	}
	return fmt.Sprintf("hourly-zone-pool=true,topology.kubernetes.io/zone=%s-%s", location, zone)
}

// GetAlwaysOnNodeSelector returns the same selector as GetRotatingNodeSelector
// since both rotating pods and the DaemonSet always-on pod share the same node.
func GetAlwaysOnNodeSelector(location string) string {
	return GetRotatingNodeSelector(location)
}

// GetDaemonSetName returns the zone-scoped DaemonSet name.
func GetDaemonSetName() string {
	return fmt.Sprintf("%s%s", HourlyDaemonSetPrefix, GetZoneSuffix())
}

// GetDaemonSetPodName finds the DaemonSet pod name in the given namespace.
func GetDaemonSetPodName(kubeconfig, namespace, dsName string) string {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "pods",
		"-n", namespace, "-l", fmt.Sprintf("app=%s", dsName),
		"-o", "jsonpath={.items[0].metadata.name}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "pod", podName,
		"-n", namespace, "--no-headers", "--ignore-not-found")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// IsPodRunning checks if a pod is in Running phase.
func IsPodRunning(kubeconfig, namespace, podName string) bool {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "pod", podName,
		"-n", namespace, "-o", "jsonpath={.status.phase}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "Running"
}

// GetNodeByLabel returns the first node matching the given label selector.
func GetNodeByLabel(kubeconfig, labelSelector string) string {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "nodes",
		"-l", labelSelector, "-o", "jsonpath={.items[0].metadata.name}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// GetNodeZone returns the zone label value for a given node.
func GetNodeZone(kubeconfig, nodeName string) string {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "node", nodeName,
		"-o", "jsonpath={.metadata.labels.topology\\.kubernetes\\.io/zone}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
