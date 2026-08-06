package ask

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/classify"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

// fakeCompleter records the prompts and schema it was called with so tests can
// assert grounding and that ask mode passes no JSON schema.
type fakeCompleter struct {
	response string
	err      error

	gotSystem string
	gotUser   string
	gotSchema *classify.Schema
}

func (f *fakeCompleter) Complete(_ context.Context, system, user string, schema *classify.Schema) (string, error) {
	f.gotSystem = system
	f.gotUser = user
	f.gotSchema = schema
	return f.response, f.err
}

func TestAnswerBuildsGroundedFreeFormPrompt(t *testing.T) {
	fc := &fakeCompleter{response: "CNS restarted because the node rebooted."}
	prior := PriorAnalysis{
		Report: "# Failure Analysis\nNode aks-nodepool1 went NotReady.",
		Incident: &model.Incident{
			Category:         model.CategoryPipelineInfraConfig,
			Confidence:       0.62,
			ConfidenceBand:   model.BandMedium,
			RootCauseSummary: "node reboot caused CNS restart",
			ProposedFix:      "rerun once nodepool is healthy",
		},
	}
	ev := model.Evidence{
		TopErrorLines: []string{"caught exit signal terminated"},
		Excerpts:      map[string]string{"live/nodes": "aks-nodepool1 NotReady"},
	}

	got, err := NewAnswerer(fc).Answer(context.Background(), "why did CNS restart?", prior, ev)
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if got != fc.response {
		t.Errorf("answer: got %q, want raw completer response", got)
	}
	if fc.gotSchema != nil {
		t.Error("expected no JSON schema in ask mode (free-form answer)")
	}

	for _, want := range []string{
		"why did CNS restart?",
		"node reboot caused CNS restart",
		"rerun once nodepool is healthy",
		"aks-nodepool1 NotReady",
		"caught exit signal terminated",
	} {
		if !strings.Contains(fc.gotUser, want) {
			t.Errorf("user prompt missing %q", want)
		}
	}

	sys := strings.ToLower(fc.gotSystem)
	if !strings.Contains(sys, "insufficient") {
		t.Error("expected system prompt to instruct explicit insufficiency")
	}
	if !strings.Contains(sys, "re-triage") {
		t.Error("expected system prompt to forbid re-triage")
	}
}

func TestAnswerDegradesWithoutPrior(t *testing.T) {
	fc := &fakeCompleter{response: "answer"}
	if _, err := NewAnswerer(fc).Answer(context.Background(), "q", PriorAnalysis{}, model.Evidence{}); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if !strings.Contains(fc.gotUser, "none available") {
		t.Error("expected prompt to note absent prior analysis")
	}
	if !strings.Contains(fc.gotUser, "no evidence excerpts available") {
		t.Error("expected prompt to note absent evidence")
	}
}

func TestAnswerRejectsEmptyQuestion(t *testing.T) {
	fc := &fakeCompleter{response: "x"}
	if _, err := NewAnswerer(fc).Answer(context.Background(), "   ", PriorAnalysis{}, model.Evidence{}); err == nil {
		t.Fatal("expected error for empty question")
	}
}

func TestAnswerRejectsEmptyResponse(t *testing.T) {
	fc := &fakeCompleter{response: "   "}
	if _, err := NewAnswerer(fc).Answer(context.Background(), "q", PriorAnalysis{}, model.Evidence{}); err == nil {
		t.Fatal("expected error for empty answer")
	}
}

func TestAnswerPropagatesCompleterError(t *testing.T) {
	fc := &fakeCompleter{err: errors.New("boom")}
	if _, err := NewAnswerer(fc).Answer(context.Background(), "q", PriorAnalysis{}, model.Evidence{}); err == nil {
		t.Fatal("expected error when completer fails")
	}
}

func TestLoadPriorAnalysis(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte("# report"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "incident.json"), []byte(`{"category":"known_flake","confidence":0.7}`), 0o644); err != nil {
		t.Fatal(err)
	}

	pa, err := LoadPriorAnalysis(dir)
	if err != nil {
		t.Fatalf("LoadPriorAnalysis: %v", err)
	}
	if pa.Report != "# report" {
		t.Errorf("report: got %q", pa.Report)
	}
	if pa.Incident == nil || pa.Incident.Category != model.CategoryKnownFlake {
		t.Errorf("incident not parsed: %+v", pa.Incident)
	}
}

func TestLoadPriorAnalysisEmptyDir(t *testing.T) {
	pa, err := LoadPriorAnalysis("")
	if err != nil {
		t.Fatalf("LoadPriorAnalysis: %v", err)
	}
	if pa.Incident != nil || pa.Report != "" {
		t.Error("expected empty prior analysis for empty dir")
	}
}

func TestLoadPriorAnalysisMissingFilesDegrade(t *testing.T) {
	// A dir that exists but holds neither file is not an error.
	pa, err := LoadPriorAnalysis(t.TempDir())
	if err != nil {
		t.Fatalf("LoadPriorAnalysis: %v", err)
	}
	if pa.Incident != nil || pa.Report != "" {
		t.Error("expected empty prior analysis when files are absent")
	}
}

func TestLoadPriorAnalysisMissingDir(t *testing.T) {
	if _, err := LoadPriorAnalysis(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("expected error for missing dir")
	}
}
