package longrunningcluster

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/test/integration/swiftv2/helpers"
)

var (
	ErrWindowsPodNoDelegatedIP = errors.New("windows pod has no delegated subnet IP (no non-primary IPv4 interface found)")
	ErrUnexpectedWindowsTCPRsp = errors.New("unexpected TCP response from windows pod")
)

// ExecInPodPowerShell executes a PowerShell command inside a Windows pod and returns the
// combined stdout/stderr output. It mirrors helpers.ExecInPod but invokes pwsh
// (PowerShell 7) instead of /bin/sh, since the default Windows image is the PowerShell-
// on-Nanoserver image which does not ship powershell.exe.
func ExecInPodPowerShell(kubeconfig, namespace, podName, script string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig, "exec", podName,
		"-n", namespace, "--", "pwsh", "-NoProfile", "-Command", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("failed to exec in windows pod %s in namespace %s: %w", podName, namespace, err)
	}

	return string(out), nil
}

// GetPodDelegatedIPWindows returns the SwiftV2 delegated subnet IP for a Windows pod.
//
// The primary pod IP (status.podIP) is the CNI overlay IP. The delegated NIC is a separate
// interface injected by SwiftV2 with its own IPv4 address. We discover it by enumerating
// IPv4 addresses inside the pod via PowerShell and returning any address that is not the
// primary pod IP, not loopback, and not link-local (APIPA 169.254.0.0/16).
func GetPodDelegatedIPWindows(kubeconfig, namespace, podName string) (string, error) {
	primaryIP, err := helpers.GetPodIP(kubeconfig, namespace, podName)
	if err != nil {
		return "", fmt.Errorf("failed to get primary pod IP for %s: %w", podName, err)
	}

	// PowerShell: emit one IPv4 address per line.
	script := "Get-NetIPAddress -AddressFamily IPv4 | ForEach-Object { $_.IPAddress }"

	maxRetries := 12
	for attempt := 1; attempt <= maxRetries; attempt++ {
		out, execErr := ExecInPodPowerShell(kubeconfig, namespace, podName, script)
		if execErr == nil {
			for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
				ip := strings.TrimSpace(line)
				if ip == "" || ip == primaryIP {
					continue
				}
				parsed := net.ParseIP(ip)
				if parsed == nil || parsed.To4() == nil {
					continue
				}
				if parsed.IsLoopback() || parsed.IsLinkLocalUnicast() {
					continue
				}
				return ip, nil
			}
			if attempt < maxRetries {
				fmt.Printf("Delegated NIC not yet visible inside windows pod %s (attempt %d/%d). Waiting 10 seconds...\n", podName, attempt, maxRetries)
				time.Sleep(10 * time.Second)
				continue
			}
			return "", fmt.Errorf("%w: pod %s in namespace %s\nIPs:\n%s", ErrWindowsPodNoDelegatedIP, podName, namespace, out)
		}

		// Retry on common transient errors during pod startup.
		errStr := strings.ToLower(execErr.Error())
		outStr := strings.ToLower(out)
		isRetryable := strings.Contains(outStr, "container not found") ||
			strings.Contains(errStr, "signal: killed") ||
			strings.Contains(errStr, "context deadline exceeded")
		if isRetryable && attempt < maxRetries {
			fmt.Printf("Retryable error getting delegated IP for windows pod %s (attempt %d/%d): %v. Waiting 5 seconds...\n", podName, attempt, maxRetries, execErr)
			time.Sleep(5 * time.Second)
			continue
		}

		return "", fmt.Errorf("failed to get delegated IP for windows pod %s in namespace %s: %w\nOutput: %s", podName, namespace, execErr, out)
	}

	return "", fmt.Errorf("%w: pod %s after %d attempts", ErrWindowsPodNoDelegatedIP, podName, maxRetries)
}

// RunWindowsConnectivityTest performs a TCP connectivity check from a Windows source pod
// to a destination pod's delegated subnet IP on port 8080, mirroring RunConnectivityTest
// but using PowerShell's System.Net.Sockets.TcpClient instead of netcat.
//
// The TCP client is bound to the source pod's delegated NIC IP so the traffic egresses on
// the SwiftV2 delegated interface (matching the Linux test's `nc -s <eth1>` behaviour).
func RunWindowsConnectivityTest(test ConnectivityTest) error {
	sourceKubeconfig := getKubeconfigPath(test.Cluster)

	destKubeconfig := sourceKubeconfig
	if test.DestCluster != "" {
		destKubeconfig = getKubeconfigPath(test.DestCluster)
	}

	destIP, err := GetPodDelegatedIPWindows(destKubeconfig, test.DestNamespace, test.DestinationPod)
	if err != nil {
		return fmt.Errorf("failed to get destination windows pod delegated IP: %w", err)
	}

	srcIP, err := GetPodDelegatedIPWindows(sourceKubeconfig, test.SourceNamespace, test.SourcePod)
	if err != nil {
		return fmt.Errorf("failed to get source windows pod delegated IP: %w", err)
	}

	fmt.Printf("Testing TCP connectivity from %s/%s (cluster: %s, src delegated: %s) to %s/%s (cluster: %s, dst delegated: %s) on port 8080\n",
		test.SourceNamespace, test.SourcePod, test.Cluster, srcIP,
		test.DestNamespace, test.DestinationPod, test.DestCluster, destIP)

	// PowerShell client: bind to source delegated IP, connect to destination, read one line.
	// 3-second connect timeout via WaitOne on the async BeginConnect handle.
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
try {
  $localEP  = New-Object System.Net.IPEndPoint([System.Net.IPAddress]::Parse('%s'), 0)
  $client   = New-Object System.Net.Sockets.TcpClient($localEP)
  $client.SendTimeout = 3000
  $client.ReceiveTimeout = 3000
  $iar = $client.BeginConnect('%s', 8080, $null, $null)
  if (-not $iar.AsyncWaitHandle.WaitOne(3000)) {
    $client.Close()
    Write-Output 'TIMEOUT'
    exit 1
  }
  $client.EndConnect($iar)
  $stream = $client.GetStream()
  $reader = New-Object System.IO.StreamReader($stream)
  $line   = $reader.ReadLine()
  $client.Close()
  Write-Output $line
} catch {
  Write-Output ('ERROR: ' + $_.Exception.Message)
  exit 1
}`, srcIP, destIP)

	output, err := ExecInPodPowerShell(sourceKubeconfig, test.SourceNamespace, test.SourcePod, script)
	if err != nil {
		return fmt.Errorf("TCP connectivity test failed: %w\nOutput: %s", err, output)
	}

	if strings.Contains(output, "TCP Connection Success") {
		fmt.Printf("TCP connectivity successful! Response: %s\n", truncateString(output, 100))
		return nil
	}

	return fmt.Errorf("%w (expected 'TCP Connection Success')\nOutput: %s", ErrUnexpectedWindowsTCPRsp, truncateString(output, 200))
}
