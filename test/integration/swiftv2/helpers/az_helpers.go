package helpers

import (
	"fmt"
	"os/exec"
	"strings"
)

func runAzCommand(cmd string, args ...string) string {
	out, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("Failed to run %s %v: %s", cmd, args, string(out)))
	}
	return strings.TrimSpace(string(out))
}

func GetVnetGUID(rg, vnet string) string {
	return runAzCommand("az", "network", "vnet", "show", "--resource-group", rg, "--name", vnet, "--query", "resourceGuid", "-o", "tsv")
}

func GetSubnetARMID(rg, vnet, subnet string) string {
	return runAzCommand("az", "network", "vnet", "subnet", "show", "--resource-group", rg, "--vnet-name", vnet, "--name", subnet, "--query", "id", "-o", "tsv")
}

func GetSubnetGUID(rg, vnet, subnet string) string {
	subnetID := GetSubnetARMID(rg, vnet, subnet)
	return runAzCommand("az", "resource", "show", "--ids", subnetID, "--api-version", "2023-09-01", "--query", "properties.serviceAssociationLinks[0].properties.subnetId", "-o", "tsv")
}

func GetSubnetToken(rg, vnet, subnet string) string {
	// Optionally implement if you use subnet token override
	return ""
}

// GetClusterNodes returns a slice of node names from a cluster using the given kubeconfig
func GetClusterNodes(kubeconfig string) []string {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "nodes", "-o", "name")
	out, err := cmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("Failed to get nodes using kubeconfig %s: %s\n%s", kubeconfig, err, string(out)))
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	nodes := make([]string, 0, len(lines))

	for _, line := range lines {
		// kubectl returns "node/<node-name>", we strip the prefix
		if strings.HasPrefix(line, "node/") {
			nodes = append(nodes, strings.TrimPrefix(line, "node/"))
		}
	}
	return nodes
}

// EnsureNamespaceExists checks if a namespace exists and creates it if it doesn't
func EnsureNamespaceExists(kubeconfig, namespace string) error {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "namespace", namespace)
	err := cmd.Run()

	if err == nil {
		return nil // Namespace exists
	}

	// Namespace doesn't exist, create it
	cmd = exec.Command("kubectl", "--kubeconfig", kubeconfig, "create", "namespace", namespace)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create namespace %s: %s\n%s", namespace, err, string(out))
	}

	return nil
}

// DeletePod deletes a pod in the specified namespace
func DeletePod(kubeconfig, namespace, podName string) error {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "delete", "pod", podName, "-n", namespace, "--ignore-not-found=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete pod %s in namespace %s: %s\n%s", podName, namespace, err, string(out))
	}
	return nil
}

// DeletePodNetworkInstance deletes a PodNetworkInstance
func DeletePodNetworkInstance(kubeconfig, namespace, pniName string) error {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "delete", "podnetworkinstance", pniName, "-n", namespace, "--ignore-not-found=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete PodNetworkInstance %s: %s\n%s", pniName, err, string(out))
	}
	return nil
}

// DeletePodNetwork deletes a PodNetwork
func DeletePodNetwork(kubeconfig, pnName string) error {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "delete", "podnetwork", pnName, "--ignore-not-found=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete PodNetwork %s: %s\n%s", pnName, err, string(out))
	}
	return nil
}

// DeleteNamespace deletes a namespace
func DeleteNamespace(kubeconfig, namespace string) error {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "delete", "namespace", namespace, "--ignore-not-found=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete namespace %s: %s\n%s", namespace, err, string(out))
	}
	return nil
}
