//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azure/aksmigrate/pkg/types"
)

var (
	binaryPath string
	repoRoot   string
)

// buildBinary compiles the aksmigrate binary and returns its path.
func buildBinary(t *testing.T) string {
	t.Helper()
	if binaryPath != "" {
		return binaryPath
	}

	repoRoot = findRepoRoot(t)
	binaryPath = filepath.Join(repoRoot, "aksmigrate-e2e-test")

	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/aksmigrate")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build binary: %v\n%s", err, out)
	}
	return binaryPath
}

// findRepoRoot walks up from the test directory to find the go.mod.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("Could not find go.mod starting from %s", dir)
		}
		dir = parent
	}
}

// skipIfNoCluster skips the test if kubectl can't connect to a cluster.
func skipIfNoCluster(t *testing.T) {
	t.Helper()
	cmd := exec.Command("kubectl", "cluster-info")
	if err := cmd.Run(); err != nil {
		t.Skip("Skipping: no cluster connection available (kubectl cluster-info failed)")
	}
}

// runBinary executes the aksmigrate binary with the given args and returns stdout, stderr, and the exit code.
func runBinary(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	bin := buildBinary(t)

	var outBuf, errBuf bytes.Buffer
	cmd := exec.Command(bin, args...)
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("Failed to run binary: %v", err)
		}
	}

	return outBuf.String(), errBuf.String(), exitCode
}

// runAuditJSON runs "aksmigrate audit --output json" and returns the parsed report.
func runAuditJSON(t *testing.T, k8sVersion string) *types.AuditReport {
	t.Helper()
	args := []string{"audit", "--output", "json", "--k8s-version", k8sVersion}
	stdout, stderr, _ := runBinary(t, args...)

	// The binary may write the JSON to stdout or mix with stderr
	combined := stdout + stderr
	report := &types.AuditReport{}

	// Find the JSON object in the output
	jsonStart := strings.Index(combined, "{")
	if jsonStart < 0 {
		t.Fatalf("No JSON found in audit output:\n%s", combined)
	}
	jsonData := combined[jsonStart:]

	if err := json.Unmarshal([]byte(jsonData), report); err != nil {
		t.Fatalf("Failed to parse audit JSON: %v\nRaw output:\n%s", err, jsonData)
	}
	return report
}

// findFindings returns all findings matching the given ruleID and namespace.
func findFindings(report *types.AuditReport, ruleID types.RuleID, namespace string) []types.Finding {
	var matches []types.Finding
	for _, f := range report.Findings {
		if f.RuleID == ruleID {
			if namespace == "" || f.Namespace == namespace {
				matches = append(matches, f)
			}
		}
	}
	return matches
}

// findFindingsByPolicy returns all findings matching the given ruleID, namespace, and policy name.
func findFindingsByPolicy(report *types.AuditReport, ruleID types.RuleID, namespace, policyName string) []types.Finding {
	var matches []types.Finding
	for _, f := range report.Findings {
		if f.RuleID == ruleID && f.Namespace == namespace && f.PolicyName == policyName {
			matches = append(matches, f)
		}
	}
	return matches
}

// requireFinding asserts that at least one finding exists for the given rule+namespace.
func requireFinding(t *testing.T, report *types.AuditReport, ruleID types.RuleID, namespace string) []types.Finding {
	t.Helper()
	findings := findFindings(report, ruleID, namespace)
	if len(findings) == 0 {
		t.Errorf("Expected finding for %s in namespace %q, but found none", ruleID, namespace)
	}
	return findings
}

// requireSeverity asserts all matching findings have the expected severity.
func requireSeverity(t *testing.T, findings []types.Finding, expected types.Severity) {
	t.Helper()
	for _, f := range findings {
		if f.Severity != expected {
			t.Errorf("Finding %s/%s: severity=%s, want %s", f.RuleID, f.PolicyName, f.Severity, expected)
		}
	}
}

// fileExists returns true if the file at the given path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// fileContains returns true if the file at the given path contains the substring.
func fileContains(t *testing.T, path, substr string) bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", path, err)
	}
	return strings.Contains(string(data), substr)
}

// tempDir creates a temporary directory for test output and registers cleanup.
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "aksmigrate-e2e-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// parseSnapshotJSON parses a connectivity snapshot JSON file.
func parseSnapshotJSON(t *testing.T, path string) *types.ConnectivitySnapshot {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read snapshot file %s: %v", path, err)
	}
	snap := &types.ConnectivitySnapshot{}
	if err := json.Unmarshal(data, snap); err != nil {
		t.Fatalf("Failed to parse snapshot JSON: %v", err)
	}
	return snap
}

// k8sVersion returns the k8s-version flag to use (defaults to "1.30").
func k8sVersion() string {
	if v := os.Getenv("K8S_VERSION"); v != "" {
		return v
	}
	return "1.31"
}

// scenarioNamespaces returns the list of e2e scenario namespaces.
func scenarioNamespaces() []string {
	return []string{
		"e2e-ipblock",
		"e2e-namedports",
		"e2e-endport",
		"e2e-lb-ingress",
		"e2e-host-egress",
		"e2e-combined",
	}
}

// assertFileExists is a test helper that fails if the file doesn't exist.
func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if !fileExists(path) {
		t.Errorf("Expected file to exist: %s", path)
	}
}

// dumpFindings logs all findings for debugging.
func dumpFindings(t *testing.T, report *types.AuditReport) {
	t.Helper()
	for i, f := range report.Findings {
		t.Logf("Finding[%d]: %s %s %s/%s: %s", i, f.Severity, f.RuleID, f.Namespace, f.PolicyName, truncate(f.Description, 80))
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// clusterName returns the cluster name for tests.
func clusterName() string {
	if v := os.Getenv("CLUSTER_NAME"); v != "" {
		return v
	}
	return "aksmigrate-e2e"
}

// resourceGroup returns the resource group for tests.
func resourceGroup() string {
	if v := os.Getenv("RESOURCE_GROUP"); v != "" {
		return v
	}
	return "aksmigrate-e2e-test"
}

// countFiles counts YAML files in a directory.
func countFiles(t *testing.T, dir, pattern string) int {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		t.Fatalf("Failed to glob %s/%s: %v", dir, pattern, err)
	}
	return len(matches)
}

// formatFindingSummary creates a concise summary of findings for debug logging.
func formatFindingSummary(findings []types.Finding) string {
	var parts []string
	for _, f := range findings {
		parts = append(parts, fmt.Sprintf("%s(%s/%s)", f.RuleID, f.Namespace, f.PolicyName))
	}
	return strings.Join(parts, ", ")
}
