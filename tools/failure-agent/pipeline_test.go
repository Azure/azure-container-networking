package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
	"go.uber.org/zap"
)

// TestRunDryRunEndToEnd exercises collect -> fingerprint -> signatures ->
// classify -> report over a committed evidence bundle using the real catalog.
func TestRunDryRunEndToEnd(t *testing.T) {
	out := t.TempDir()
	opts := options{
		input:          filepath.Join("testdata", "bundle"),
		output:         out,
		signaturesPath: filepath.Join("signatures", "signatures.yaml"),
		dryRun:         true,
		pipeline:       "ACN PR",
		clusterType:    "overlay-byocni-up",
		osName:         "linux",
		cni:            "cilium",
	}

	if err := run(zap.NewNop(), opts); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(out, "incident.json"))
	if err != nil {
		t.Fatalf("reading incident.json: %v", err)
	}
	var inc model.Incident
	if err := json.Unmarshal(data, &inc); err != nil {
		t.Fatalf("unmarshaling incident: %v", err)
	}

	if inc.Category != model.CategoryClusterBringupFailure {
		t.Errorf("category: got %s, want %s", inc.Category, model.CategoryClusterBringupFailure)
	}
	if inc.Fingerprint == "" {
		t.Error("expected non-empty fingerprint")
	}
	if len(inc.SignatureMatches) == 0 {
		t.Error("expected at least one signature match")
	}

	md, err := os.ReadFile(filepath.Join(out, "report.md"))
	if err != nil {
		t.Fatalf("reading report.md: %v", err)
	}
	if !strings.Contains(string(md), "acn-failure-agent:"+inc.Fingerprint) {
		t.Error("expected fingerprint marker in report.md")
	}
}

func TestRunRequiresInput(t *testing.T) {
	if err := run(zap.NewNop(), options{dryRun: true}); err == nil {
		t.Fatal("expected error when --input missing")
	}
}

func TestRunLiveModeRequiresAOAI(t *testing.T) {
	opts := options{
		input:          filepath.Join("testdata", "bundle"),
		output:         t.TempDir(),
		signaturesPath: filepath.Join("signatures", "signatures.yaml"),
		dryRun:         false,
	}
	if err := run(zap.NewNop(), opts); err == nil {
		t.Fatal("expected error in live mode without --aoai-endpoint/--aoai-deployment")
	}
}
