package helpers

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

var (
	ErrPodNotRunning = errors.New("pod did not reach Running state")
	ErrPodNoIP = errors.New("pod has no IP address assigned")
	ErrPodNoEth1IP = errors.New("pod has no eth1 IP address (delegated subnet not configured?)")
	ErrPodContainerNotReady = errors.New("pod container not ready")
	ErrMTPNCStuckDeletion = errors.New("MTPNC resources should have been deleted but were found")
	ErrPodDeletionFailed = errors.New("pod still exists after deletion attempts")
	ErrPNIDeletionFailed = errors.New("PodNetworkInstance still exists after deletion attempts")
	ErrPNDeletionFailed = errors.New("PodNetwork still exists after deletion attempts")
	ErrNamespaceDeletionFailed = errors.New("namespace still exists after deletion attempts")
)

func runAzCommand(cmd string, args ...string) (string, error) {
	out, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run %s %v: %w\nOutput: %s", cmd, args, err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

func GetVnetGUID(rg, vnet string) (string, error) {
	return runAzCommand("az", "network", "vnet", "show", "--resource-group", rg, "--name", vnet, "--query", "resourceGuid", "-o", "tsv")
}

func GetSubnetARMID(rg, vnet, subnet string) (string, error) {
	return runAzCommand("az", "network", "vnet", "subnet", "show", "--resource-group", rg, "--vnet-name", vnet, "--name", subnet, "--query", "id", "-o", "tsv")
}

func GetSubnetGUID(rg, vnet, subnet string) (string, error) {
	subnetID, err := GetSubnetARMID(rg, vnet, subnet)
	if err != nil {
		return "", err
	}
	return runAzCommand("az", "resource", "show", "--ids", subnetID, "--api-version", "2023-09-01", "--query", "properties.serviceAssociationLinks[0].properties.subnetId", "-o", "tsv")
}

// GetClusterNodes returns a slice of node names from a cluster using the given kubeconfig
func GetClusterNodes(kubeconfig string) ([]string, error) {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "nodes", "-o", "name")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes using kubeconfig %s: %w\nOutput: %s", kubeconfig, err, string(out))
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	nodes := make([]string, 0, len(lines))

	for _, line := range lines {
		// kubectl returns "node/<node-name>", we strip the prefix
		if strings.HasPrefix(line, "node/") {
			nodes = append(nodes, strings.TrimPrefix(line, "node/"))
		}
	}
	return nodes, nil
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
		return fmt.Errorf("failed to create namespace %s: %w\nOutput: %s", namespace, err, string(out))
	}

	return nil
}

// DeletePod deletes a pod in the specified namespace and waits for it to be fully removed
func DeletePod(kubeconfig, namespace, podName string) error {
	fmt.Printf("Deleting pod %s in namespace %s...\n", podName, namespace)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig, "delete", "pod", podName, "-n", namespace, "--ignore-not-found=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			fmt.Printf("Warning: kubectl delete pod command timed out after 90s\n")
		} else {
			return fmt.Errorf("failed to delete pod %s in namespace %s: %w\nOutput: %s", podName, namespace, err, string(out))
		}
	}

	// Wait for pod to be completely gone (critical for IP release)
	fmt.Printf("Waiting for pod %s to be fully removed...\n", podName)
	for attempt := 1; attempt <= 30; attempt++ {
		checkCtx, checkCancel := context.WithTimeout(context.Background(), 20*time.Second)
		checkCmd := exec.CommandContext(checkCtx, "kubectl", "--kubeconfig", kubeconfig, "get", "pod", podName, "-n", namespace, "--ignore-not-found=true", "-o", "name")
		checkOut, _ := checkCmd.CombinedOutput()
		checkCancel()

		if strings.TrimSpace(string(checkOut)) == "" {
			fmt.Printf("Pod %s fully removed after %d seconds\n", podName, attempt*2)
			time.Sleep(5 * time.Second)
			return nil
		}

		if attempt%5 == 0 {
			fmt.Printf("Pod %s still terminating (attempt %d/30)...\n", podName, attempt)
		}
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("%w: pod %s still exists", ErrPodDeletionFailed, podName)
}

func DeletePodNetworkInstance(kubeconfig, namespace, pniName string) error {
	fmt.Printf("Deleting PodNetworkInstance %s in namespace %s...\n", pniName, namespace)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig, "delete", "podnetworkinstance", pniName, "-n", namespace, "--ignore-not-found=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			fmt.Printf("Warning: kubectl delete PNI command timed out after 90s\n")
		} else {
			return fmt.Errorf("failed to delete PodNetworkInstance %s: %w\nOutput: %s", pniName, err, string(out))
		}
	}

	fmt.Printf("Waiting for PodNetworkInstance %s to be fully removed...\n", pniName)
	for attempt := 1; attempt <= 30; attempt++ {
		checkCtx, checkCancel := context.WithTimeout(context.Background(), 20*time.Second)
		checkCmd := exec.CommandContext(checkCtx, "kubectl", "--kubeconfig", kubeconfig, "get", "podnetworkinstance", pniName, "-n", namespace, "--ignore-not-found=true", "-o", "name")
		checkOut, _ := checkCmd.CombinedOutput()
		checkCancel()

		if strings.TrimSpace(string(checkOut)) == "" {
			fmt.Printf("PodNetworkInstance %s fully removed after %d seconds\n", pniName, attempt*2)
			return nil
		}

		if attempt%10 == 0 {
			descCmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "describe", "podnetworkinstance", pniName, "-n", namespace)
			descOut, _ := descCmd.CombinedOutput()
			descStr := string(descOut)
			if strings.Contains(descStr, "ReservationInUse") {
				fmt.Printf("PNI %s still has active reservations (attempt %d/30). Waiting for DNC to release...\n", pniName, attempt)
			} else {
				fmt.Printf("PNI %s still terminating (attempt %d/30)...\n", pniName, attempt)
			}
		}
		time.Sleep(2 * time.Second)
	}

	fmt.Printf("PNI %s still exists, attempting to remove finalizers...\n", pniName)
	patchCmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "patch", "podnetworkinstance", pniName, "-n", namespace, "-p", `{"metadata":{"finalizers":[]}}`, "--type=merge")
	patchOut, patchErr := patchCmd.CombinedOutput()
	if patchErr != nil {
		fmt.Printf("Warning: Failed to remove finalizers: %s\n%s\n", patchErr, string(patchOut))
	} else {
		fmt.Printf("Finalizers removed, waiting for deletion...\n")
		time.Sleep(5 * time.Second)
	}

	checkCtx, checkCancel := context.WithTimeout(context.Background(), 10*time.Second)
	checkCmd := exec.CommandContext(checkCtx, "kubectl", "--kubeconfig", kubeconfig, "get", "podnetworkinstance", pniName, "-n", namespace, "--ignore-not-found=true", "-o", "name")
	checkOut, _ := checkCmd.CombinedOutput()
	checkCancel()
	if strings.TrimSpace(string(checkOut)) != "" {
		return fmt.Errorf("%w: PodNetworkInstance %s in namespace %s", ErrPNIDeletionFailed, pniName, namespace)
	}

	fmt.Printf("PodNetworkInstance %s deletion completed\n", pniName)
	return nil
}

func DeletePodNetwork(kubeconfig, pnName string) error {
	fmt.Printf("Deleting PodNetwork %s...\n", pnName)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig, "delete", "podnetwork", pnName, "--ignore-not-found=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			fmt.Printf("Warning: kubectl delete PN command timed out after 90s\n")
		} else {
			return fmt.Errorf("failed to delete PodNetwork %s: %w\nOutput: %s", pnName, err, string(out))
		}
	}

	// Wait for PN to be completely gone
	fmt.Printf("Waiting for PodNetwork %s to be fully removed...\n", pnName)
	for attempt := 1; attempt <= 30; attempt++ {
		checkCtx, checkCancel := context.WithTimeout(context.Background(), 20*time.Second)
		checkCmd := exec.CommandContext(checkCtx, "kubectl", "--kubeconfig", kubeconfig, "get", "podnetwork", pnName, "--ignore-not-found=true", "-o", "name")
		checkOut, _ := checkCmd.CombinedOutput()
		checkCancel()

		if strings.TrimSpace(string(checkOut)) == "" {
			fmt.Printf("PodNetwork %s fully removed after %d seconds\n", pnName, attempt*2)
			return nil
		}

		if attempt%10 == 0 {
			fmt.Printf("PodNetwork %s still terminating (attempt %d/30)...\n", pnName, attempt)
		}
		time.Sleep(2 * time.Second)
	}

	// Try to remove finalizers if still stuck
	fmt.Printf("PodNetwork %s still exists, attempting to remove finalizers...\n", pnName)
	patchCmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "patch", "podnetwork", pnName, "-p", `{"metadata":{"finalizers":[]}}`, "--type=merge")
	patchOut, patchErr := patchCmd.CombinedOutput()
	if patchErr != nil {
		fmt.Printf("Warning: Failed to remove finalizers: %s\n%s\n", patchErr, string(patchOut))
	}

	time.Sleep(5 * time.Second)
	checkCtx, checkCancel := context.WithTimeout(context.Background(), 10*time.Second)
	checkCmd := exec.CommandContext(checkCtx, "kubectl", "--kubeconfig", kubeconfig, "get", "podnetwork", pnName, "--ignore-not-found=true", "-o", "name")
	checkOut, _ := checkCmd.CombinedOutput()
	checkCancel()

	if strings.TrimSpace(string(checkOut)) != "" {
		return fmt.Errorf("%w: PodNetwork %s", ErrPNDeletionFailed, pnName)
	}

	fmt.Printf("PodNetwork %s deletion completed\n", pnName)
	return nil
}

// DeleteNamespace deletes a namespace and waits for it to be removed
func DeleteNamespace(kubeconfig, namespace string) error {
	fmt.Printf("Deleting namespace %s...\n", namespace)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig, "delete", "namespace", namespace, "--ignore-not-found=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			fmt.Printf("Warning: kubectl delete namespace command timed out after 90s\n")
		} else {
			return fmt.Errorf("failed to delete namespace %s: %w\nOutput: %s", namespace, err, string(out))
		}
	}

	// Wait for namespace to be completely gone
	fmt.Printf("Waiting for namespace %s to be fully removed...\n", namespace)
	for attempt := 1; attempt <= 30; attempt++ {
		checkCtx, checkCancel := context.WithTimeout(context.Background(), 20*time.Second)
		checkCmd := exec.CommandContext(checkCtx, "kubectl", "--kubeconfig", kubeconfig, "get", "namespace", namespace, "--ignore-not-found=true", "-o", "name")
		checkOut, _ := checkCmd.CombinedOutput()
		checkCancel()

		if strings.TrimSpace(string(checkOut)) == "" {
			fmt.Printf("Namespace %s fully removed after %d seconds\n", namespace, attempt*2)
			return nil
		}

		if attempt%10 == 0 {
			fmt.Printf("Namespace %s still terminating (attempt %d/30)...\n", namespace, attempt)
		}
		time.Sleep(2 * time.Second)
	}

	// Try to remove finalizers if still stuck
	fmt.Printf("Namespace %s still exists, attempting to remove finalizers...\n", namespace)
	patchCmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "patch", "namespace", namespace, "-p", `{"metadata":{"finalizers":[]}}`, "--type=merge")
	patchOut, patchErr := patchCmd.CombinedOutput()
	if patchErr != nil {
		fmt.Printf("Warning: Failed to remove finalizers: %s\n%s\n", patchErr, string(patchOut))
	}

	time.Sleep(5 * time.Second)

	// Verify namespace is actually gone
	checkCtx, checkCancel := context.WithTimeout(context.Background(), 10*time.Second)
	checkCmd := exec.CommandContext(checkCtx, "kubectl", "--kubeconfig", kubeconfig, "get", "namespace", namespace, "--ignore-not-found=true", "-o", "name")
	checkOut, _ := checkCmd.CombinedOutput()
	checkCancel()

	if strings.TrimSpace(string(checkOut)) != "" {
		return fmt.Errorf("%w: namespace %s", ErrNamespaceDeletionFailed, namespace)
	}

	fmt.Printf("Namespace %s deletion completed\n", namespace)
	return nil
}

// WaitForPodRunning waits for a pod to reach Running state with retries
func WaitForPodRunning(kubeconfig, namespace, podName string, maxRetries, sleepSeconds int) error {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "pod", podName, "-n", namespace, "-o", "jsonpath={.status.phase}")
		out, err := cmd.CombinedOutput()

		if err == nil && strings.TrimSpace(string(out)) == "Running" {
			fmt.Printf("Pod %s is now Running\n", podName)
			return nil
		}

		if attempt < maxRetries {
			fmt.Printf("Pod %s not running yet (attempt %d/%d), status: %s. Waiting %d seconds...\n",
				podName, attempt, maxRetries, strings.TrimSpace(string(out)), sleepSeconds)
			time.Sleep(time.Duration(sleepSeconds) * time.Second)
		}
	}

	return fmt.Errorf("%w: pod %s after %d attempts", ErrPodNotRunning, podName, maxRetries)
}

// GetPodIP retrieves the IP address of a pod
func GetPodIP(kubeconfig, namespace, podName string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig, "get", "pod", podName,
		"-n", namespace, "-o", "jsonpath={.status.podIP}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get pod IP for %s in namespace %s: %w\nOutput: %s", podName, namespace, err, string(out))
	}

	ip := strings.TrimSpace(string(out))
	if ip == "" {
		return "", fmt.Errorf("%w: pod %s in namespace %s", ErrPodNoIP, podName, namespace)
	}

	return ip, nil
}

// GetPodDelegatedIP retrieves the eth1 IP address (delegated subnet IP) of a pod
// This is the IP used for cross-VNet communication and is subject to NSG rules
func GetPodDelegatedIP(kubeconfig, namespace, podName string) (string, error) {
	// Retry logic - pod might be Running but container not ready yet, or network interface still initializing
	maxRetries := 5
	for attempt := 1; attempt <= maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		// Get eth1 IP address by running 'ip addr show eth1' in the pod
		cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig, "exec", podName,
			"-n", namespace, "-c", "net-debugger", "--", "sh", "-c", "ip -4 addr show eth1 | grep 'inet ' | awk '{print $2}' | cut -d'/' -f1")
		out, err := cmd.CombinedOutput()
		cancel()

		if err == nil {
			ip := strings.TrimSpace(string(out))
			if ip != "" {
				return ip, nil
			}
			return "", fmt.Errorf("%w: pod %s in namespace %s", ErrPodNoEth1IP, podName, namespace)
		}

		// Check for retryable errors: container not found, signal killed, context deadline exceeded
		errStr := strings.ToLower(err.Error())
		outStr := strings.ToLower(string(out))
		isRetryable := strings.Contains(outStr, "container not found") ||
			strings.Contains(errStr, "signal: killed") ||
			strings.Contains(errStr, "context deadline exceeded")

		if isRetryable && attempt < maxRetries {
			fmt.Printf("Retryable error getting IP for pod %s (attempt %d/%d): %v. Waiting 5 seconds...\n", podName, attempt, maxRetries, err)
			time.Sleep(5 * time.Second)
			continue
		}

		return "", fmt.Errorf("failed to get eth1 IP for %s in namespace %s: %w\nOutput: %s", podName, namespace, err, string(out))
	}

	return "", fmt.Errorf("%w: pod %s after %d attempts", ErrPodContainerNotReady, podName, maxRetries)
}

// ExecInPod executes a command in a pod and returns the output
func ExecInPod(kubeconfig, namespace, podName, command string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig, "exec", podName,
		"-n", namespace, "--", "sh", "-c", command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("failed to exec in pod %s in namespace %s: %w", podName, namespace, err)
	}

	return string(out), nil
}

// VerifyNoMTPNC checks if there are any pending MTPNC (MultiTenantPodNetworkConfig) resources
// associated with a specific build ID that should have been cleaned up
func VerifyNoMTPNC(kubeconfig, buildID string) error {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "mtpnc", "-A", "-o", "json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "the server doesn't have a resource type") {
			return nil
		}
		return fmt.Errorf("failed to get MTPNC resources: %w\nOutput: %s", err, string(out))
	}

	output := string(out)
	if strings.Contains(output, buildID) {
		lines := strings.Split(output, "\n")
		var mtpncNames []string
		for _, line := range lines {
			if strings.Contains(line, buildID) && strings.Contains(line, "\"name\":") {
				mtpncNames = append(mtpncNames, line)
			}
		}

		if len(mtpncNames) > 0 {
			return fmt.Errorf("%w: found %d MTPNC resources with build ID '%s'", ErrMTPNCStuckDeletion, len(mtpncNames), buildID)
		}
	}

	return nil
}
