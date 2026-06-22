package classify

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

type fakeCompleter struct {
	response string
	err      error

	gotSystem string
	gotUser   string
	gotSchema *Schema
}

func (f *fakeCompleter) Complete(_ context.Context, system, user string, schema *Schema) (string, error) {
	f.gotSystem = system
	f.gotUser = user
	f.gotSchema = schema
	return f.response, f.err
}

func TestLLMClassifierValidResponse(t *testing.T) {
	fc := &fakeCompleter{response: `{
		"category": "pr_regression",
		"confidence": 0.91,
		"rootCauseSummary": "the change under test removed a required field",
		"topEvidence": ["panic: nil pointer", "added in this PR"],
		"recommendedOwner": "acn-cni",
		"proposedFix": "Restore the required field in the struct and add a nil check before accessing it."
	}`}

	got, err := NewLLMClassifier(fc).Classify(context.Background(), model.RunContext{}, model.Evidence{}, model.Fingerprint{}, nil, PriorContext{})
	if err != nil {
		t.Fatalf("Classify returned error: %v", err)
	}

	if got.Category != model.CategoryPRRegression {
		t.Errorf("category: got %s, want %s", got.Category, model.CategoryPRRegression)
	}
	if got.Confidence != 0.91 {
		t.Errorf("confidence: got %v, want 0.91", got.Confidence)
	}
	if got.Source != "llm" {
		t.Errorf("source: got %q, want llm", got.Source)
	}
	if got.RootCauseSummary == "" {
		t.Error("expected non-empty root cause summary")
	}
	if fc.gotSchema == nil || fc.gotSchema.Name == "" {
		t.Error("expected a schema to be passed to the completer")
	}
}

func TestLLMClassifierRejectsBadResponses(t *testing.T) {
	tests := map[string]string{
		"invalid category": `{"category":"definitely_not_real","confidence":0.5,"rootCauseSummary":"x"}`,
		"confidence high":  `{"category":"known_flake","confidence":1.5,"rootCauseSummary":"x"}`,
		"confidence low":   `{"category":"known_flake","confidence":-0.1,"rootCauseSummary":"x"}`,
		"empty summary":    `{"category":"known_flake","confidence":0.5,"rootCauseSummary":"   "}`,
		"not json":         `not json at all`,
	}

	for name, resp := range tests {
		t.Run(name, func(t *testing.T) {
			fc := &fakeCompleter{response: resp}
			if _, err := NewLLMClassifier(fc).Classify(context.Background(), model.RunContext{}, model.Evidence{}, model.Fingerprint{}, nil, PriorContext{}); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestLLMClassifierPropagatesCompleterError(t *testing.T) {
	fc := &fakeCompleter{err: errors.New("boom")}
	if _, err := NewLLMClassifier(fc).Classify(context.Background(), model.RunContext{}, model.Evidence{}, model.Fingerprint{}, nil, PriorContext{}); err == nil {
		t.Fatal("expected error when completer fails")
	}
}

func TestLLMClassifierInjectsPriorKnowledge(t *testing.T) {
	fc := &fakeCompleter{response: `{
		"category": "known_flake",
		"confidence": 0.7,
		"rootCauseSummary": "recurring image pull flake",
		"topEvidence": ["ImagePullBackOff"],
		"recommendedOwner": "acn-cni",
		"proposedFix": "retry"
	}`}
	prior := PriorContext{
		Resolved:   []PriorIncident{{Category: "known_flake", Summary: "same image pull flake", Fix: "bump retry budget", Status: "validated_resolved"}},
		Unresolved: []PriorIncident{{Category: "known_flake", Summary: "open flake report", Status: "pr_open"}},
	}

	if _, err := NewLLMClassifier(fc).Classify(context.Background(), model.RunContext{}, model.Evidence{}, model.Fingerprint{}, nil, prior); err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if !strings.Contains(fc.gotUser, "Prior validated resolutions") {
		t.Error("expected validated resolutions section in prompt")
	}
	if !strings.Contains(fc.gotUser, "bump retry budget") {
		t.Error("expected validated fix text in prompt")
	}
	if !strings.Contains(fc.gotUser, "NOT yet validated") {
		t.Error("expected in-flight incidents to be clearly labeled")
	}
}
