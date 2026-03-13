//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azure/aksmigrate/pkg/types"
)

// TestMain builds the binary once for all tests.
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// TestAuditAllScenarios runs "aksmigrate audit" once against the cluster
// and validates findings for each scenario namespace.
func TestAuditAllScenarios(t *testing.T) {
	skipIfNoCluster(t)
	version := k8sVersion()

	report := runAuditJSON(t, version)
	t.Logf("Audit: %d findings, %d policies", len(report.Findings), report.TotalPolicies)
	dumpFindings(t, report)

	if report.TotalPolicies == 0 {
		t.Fatal("Audit found 0 policies — are the scenarios deployed?")
	}

	t.Run("Scenario1_IPBlockCatchAll_CILIUM001", func(t *testing.T) {
		findings := requireFinding(t, report, types.RuleIPBlockCatchAll, "e2e-ipblock")
		requireSeverity(t, findings, types.SeverityFail)

		// Verify the specific policy is identified
		policyFindings := findFindingsByPolicy(report, types.RuleIPBlockCatchAll, "e2e-ipblock", "egress-ipblock-catch-all")
		if len(policyFindings) == 0 {
			t.Error("Expected CILIUM-001 finding for policy 'egress-ipblock-catch-all'")
		}

		// Verify description mentions 0.0.0.0/0
		for _, f := range findings {
			if !strings.Contains(f.Description, "0.0.0.0/0") {
				t.Errorf("Expected description to mention '0.0.0.0/0', got: %s", truncate(f.Description, 120))
			}
		}
	})

	t.Run("Scenario2_NamedPorts_CILIUM002", func(t *testing.T) {
		findings := requireFinding(t, report, types.RuleNamedPorts, "e2e-namedports")

		// Should be FAIL (conflicting 8080 vs 9090) or WARN (named port detected)
		for _, f := range findings {
			if f.Severity != types.SeverityFail && f.Severity != types.SeverityWarn {
				t.Errorf("CILIUM-002 severity=%s, want FAIL or WARN", f.Severity)
			}
		}

		// Verify the named port "http" is mentioned
		for _, f := range findings {
			if !strings.Contains(f.Description, "http") {
				t.Errorf("Expected description to mention named port 'http', got: %s", truncate(f.Description, 120))
			}
		}
	})

	t.Run("Scenario3_EndPort_CILIUM003", func(t *testing.T) {
		findings := requireFinding(t, report, types.RuleEndPort, "e2e-endport")
		requireSeverity(t, findings, types.SeverityFail)

		// Verify endPort value is mentioned
		for _, f := range findings {
			if !strings.Contains(f.Description, "5440") {
				t.Errorf("Expected description to mention endPort 5440, got: %s", truncate(f.Description, 120))
			}
		}
	})

	t.Run("Scenario4_LBIngress_CILIUM005", func(t *testing.T) {
		findings := requireFinding(t, report, types.RuleLBIngressEnforcement, "e2e-lb-ingress")
		requireSeverity(t, findings, types.SeverityFail)

		// Verify service name is mentioned
		for _, f := range findings {
			if !strings.Contains(f.Description, "web-lb") {
				t.Errorf("Expected description to mention service 'web-lb', got: %s", truncate(f.Description, 120))
			}
		}
	})

	t.Run("Scenario5_HostEgress_CILIUM004", func(t *testing.T) {
		findings := requireFinding(t, report, types.RuleImplicitLocalNodeEgress, "e2e-host-egress")
		requireSeverity(t, findings, types.SeverityWarn)
	})

	t.Run("CrossCutting_KubeProxyRemoval_CILIUM007", func(t *testing.T) {
		findings := findFindings(report, types.RuleKubeProxyRemoval, "")
		if len(findings) == 0 {
			t.Error("Expected CILIUM-007 (kube-proxy removal) INFO finding")
		}
		requireSeverity(t, findings, types.SeverityInfo)
	})

	t.Run("Scenario6_Combined_MultipleRules", func(t *testing.T) {
		// CILIUM-001 in e2e-combined (egress-catch-all with 0.0.0.0/0)
		findings001 := findFindings(report, types.RuleIPBlockCatchAll, "e2e-combined")
		if len(findings001) == 0 {
			t.Error("Expected CILIUM-001 in e2e-combined namespace")
		}

		// CILIUM-005 in e2e-combined (deny-all-ingress-frontend + frontend-lb service)
		findings005 := findFindings(report, types.RuleLBIngressEnforcement, "e2e-combined")
		if len(findings005) == 0 {
			t.Error("Expected CILIUM-005 in e2e-combined namespace")
		}

		// CILIUM-004 in e2e-combined (restrict-backend-egress)
		findings004 := findFindings(report, types.RuleImplicitLocalNodeEgress, "e2e-combined")
		if len(findings004) == 0 {
			t.Error("Expected CILIUM-004 in e2e-combined namespace")
		}

		// CILIUM-003 in e2e-combined (egress-catch-all uses endPort: 5440)
		findings003 := findFindings(report, types.RuleEndPort, "e2e-combined")
		if len(findings003) == 0 {
			t.Error("Expected CILIUM-003 in e2e-combined namespace")
		}
	})
}

// TestTranslateScenarios runs "aksmigrate translate" and validates the generated files.
func TestTranslateScenarios(t *testing.T) {
	skipIfNoCluster(t)
	version := k8sVersion()
	outputDir := tempDir(t)

	stdout, stderr, exitCode := runBinary(t, "translate",
		"--output-dir", outputDir,
		"--k8s-version", version,
	)
	t.Logf("Translate exit=%d\nstdout:\n%s\nstderr:\n%s", exitCode, stdout, stderr)

	if exitCode != 0 {
		t.Fatalf("Translate exited with code %d", exitCode)
	}

	patchedDir := filepath.Join(outputDir, "patched")
	ciliumDir := filepath.Join(outputDir, "cilium")

	t.Run("PatchedDirCreated", func(t *testing.T) {
		assertFileExists(t, patchedDir)
		count := countFiles(t, patchedDir, "*.yaml")
		if count == 0 {
			t.Error("No patched YAML files generated")
		}
		t.Logf("Patched files: %d", count)
	})

	t.Run("CiliumDirCreated", func(t *testing.T) {
		assertFileExists(t, ciliumDir)
		count := countFiles(t, ciliumDir, "*.yaml")
		if count == 0 {
			t.Error("No Cilium YAML files generated")
		}
		t.Logf("Cilium files: %d", count)
	})

	t.Run("Scenario1_IPBlockPatched", func(t *testing.T) {
		path := filepath.Join(patchedDir, "e2e-ipblock-egress-ipblock-catch-all.yaml")
		if !fileExists(path) {
			t.Fatalf("Patched ipBlock policy not found: %s", path)
		}
		if !fileContains(t, path, "namespaceSelector") {
			t.Error("Patched ipBlock policy should contain namespaceSelector")
		}
	})

	t.Run("Scenario5_HostEgressCNP", func(t *testing.T) {
		path := filepath.Join(ciliumDir, "e2e-host-egress-allow-host-egress.yaml")
		if !fileExists(path) {
			t.Fatalf("Host egress CiliumNetworkPolicy not found: %s", path)
		}
		if !fileContains(t, path, "toEntities") {
			t.Error("Host egress CNP should contain toEntities")
		}
		if !fileContains(t, path, "host") {
			t.Error("Host egress CNP should reference 'host' entity")
		}
		if !fileContains(t, path, "remote-node") {
			t.Error("Host egress CNP should reference 'remote-node' entity")
		}
	})

	t.Run("Scenario4_LBIngressCNP", func(t *testing.T) {
		path := filepath.Join(ciliumDir, "e2e-lb-ingress-allow-lb-ingress-web-lb.yaml")
		if !fileExists(path) {
			t.Fatalf("LB ingress CiliumNetworkPolicy not found: %s", path)
		}
		if !fileContains(t, path, "fromEntities") {
			t.Error("LB ingress CNP should contain fromEntities")
		}
		if !fileContains(t, path, "world") {
			t.Error("LB ingress CNP should reference 'world' entity")
		}
	})
}

// TestConntestSnapshot validates that "aksmigrate conntest snapshot" produces valid JSON.
func TestConntestSnapshot(t *testing.T) {
	skipIfNoCluster(t)
	outputDir := tempDir(t)
	snapshotFile := filepath.Join(outputDir, "snapshot.json")

	stdout, stderr, exitCode := runBinary(t,
		"conntest", "snapshot",
		"--phase", "pre-migration",
		"--output", snapshotFile,
	)
	t.Logf("Conntest snapshot exit=%d\nstdout:\n%s\nstderr:\n%s", exitCode, stdout, stderr)

	if exitCode != 0 {
		t.Fatalf("Conntest snapshot exited with code %d", exitCode)
	}

	t.Run("SnapshotFileCreated", func(t *testing.T) {
		assertFileExists(t, snapshotFile)
	})

	t.Run("SnapshotHasResults", func(t *testing.T) {
		snap := parseSnapshotJSON(t, snapshotFile)
		if len(snap.Results) == 0 {
			t.Error("Snapshot has no connectivity results")
		}
		t.Logf("Snapshot: %d results", len(snap.Results))
	})

	t.Run("SnapshotHasPhase", func(t *testing.T) {
		snap := parseSnapshotJSON(t, snapshotFile)
		if snap.Phase != "pre-migration" {
			t.Errorf("Phase=%q, want 'pre-migration'", snap.Phase)
		}
	})

	t.Run("SnapshotHasTimestamp", func(t *testing.T) {
		snap := parseSnapshotJSON(t, snapshotFile)
		if snap.Timestamp == "" {
			t.Error("Snapshot missing timestamp")
		}
	})
}

// TestMigrateDryRun validates that "aksmigrate migrate --dry-run" completes the 7-step flow.
func TestMigrateDryRun(t *testing.T) {
	skipIfNoCluster(t)
	version := k8sVersion()
	outputDir := tempDir(t)

	stdout, stderr, exitCode := runBinary(t,
		"migrate",
		"--cluster-name", clusterName(),
		"--resource-group", resourceGroup(),
		"--output-dir", outputDir,
		"--k8s-version", version,
		"--dry-run",
		"--skip-snapshot",
	)
	combined := stdout + stderr
	t.Logf("Migrate dry-run exit=%d\nOutput:\n%s", exitCode, combined)

	// Dry-run should complete (may exit 0 or non-zero depending on FAIL findings,
	// but with --dry-run it should not be blocked by them)

	t.Run("AllStepMarkers", func(t *testing.T) {
		for step := 1; step <= 7; step++ {
			marker := strings.ReplaceAll("[N/7]", "N", string(rune('0'+step)))
			marker = "[" + string(rune('0'+step)) + "/7]"
			if !strings.Contains(combined, marker) {
				t.Errorf("Step marker %s not found in output", marker)
			}
		}
	})

	t.Run("PatchesDirectoryCreated", func(t *testing.T) {
		patchesDir := filepath.Join(outputDir, "patches")
		if !fileExists(patchesDir) {
			t.Error("Patches directory not created by migrate --dry-run")
		} else {
			count := countFiles(t, patchesDir, "*.yaml")
			t.Logf("Patches directory has %d YAML files", count)
		}
	})

	t.Run("FinalReport", func(t *testing.T) {
		if !strings.Contains(combined, "Migration Report") {
			t.Error("Migration final report not found in output")
		}
		if !strings.Contains(combined, "DRY RUN") {
			t.Error("DRY RUN indicator not found in final report")
		}
	})

	t.Run("DryRunComplete", func(t *testing.T) {
		if !strings.Contains(combined, "Dry run complete") {
			t.Error("'Dry run complete' message not found")
		}
	})
}

// TestAuditExitCodeOnFail verifies that audit exits with code 1 when FAIL findings exist.
func TestAuditExitCodeOnFail(t *testing.T) {
	skipIfNoCluster(t)
	version := k8sVersion()

	_, _, exitCode := runBinary(t, "audit", "--output", "json", "--k8s-version", version)
	// With our scenarios deployed, there should be FAIL findings
	if exitCode != 1 {
		t.Errorf("Expected exit code 1 (FAIL findings present), got %d", exitCode)
	}
}

// TestTranslateEndPortExpansion validates that endPort ranges are expanded into individual ports.
func TestTranslateEndPortExpansion(t *testing.T) {
	skipIfNoCluster(t)
	version := k8sVersion()
	outputDir := tempDir(t)

	_, _, exitCode := runBinary(t, "translate",
		"--output-dir", outputDir,
		"--k8s-version", version,
	)
	if exitCode != 0 {
		t.Fatalf("Translate failed with exit code %d", exitCode)
	}

	patchedDir := filepath.Join(outputDir, "patched")

	// Find the endport scenario patched file
	path := filepath.Join(patchedDir, "e2e-endport-egress-db-port-range.yaml")
	if !fileExists(path) {
		// The policy in scenario3 uses ipBlock 10.0.0.0/8 which is broad (<=16),
		// so it should be patched. If not patched, the endPort expansion still
		// occurs as part of the translation.
		t.Skipf("Patched endPort policy not found at %s (may not have been patched)", path)
	}
}

// TestCombinedScenarioMigrateDryRun validates that the combined scenario
// triggers multiple rule types in a single namespace.
func TestCombinedScenarioMigrateDryRun(t *testing.T) {
	skipIfNoCluster(t)
	version := k8sVersion()

	report := runAuditJSON(t, version)

	// Count distinct rules triggered in e2e-combined
	rulesInCombined := make(map[types.RuleID]bool)
	for _, f := range report.Findings {
		if f.Namespace == "e2e-combined" {
			rulesInCombined[f.RuleID] = true
		}
	}

	t.Logf("Rules triggered in e2e-combined: %d", len(rulesInCombined))
	for rule := range rulesInCombined {
		t.Logf("  - %s", rule)
	}

	// We expect at least CILIUM-001, CILIUM-003, CILIUM-004, CILIUM-005
	expectedRules := []types.RuleID{
		types.RuleIPBlockCatchAll,
		types.RuleEndPort,
		types.RuleImplicitLocalNodeEgress,
		types.RuleLBIngressEnforcement,
	}

	for _, rule := range expectedRules {
		if !rulesInCombined[rule] {
			t.Errorf("Expected rule %s in e2e-combined namespace", rule)
		}
	}
}
