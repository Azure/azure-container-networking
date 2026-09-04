package escalate

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

// fakeCompleter returns a canned response and records the prompts it was given.
type fakeCompleter struct {
	response  string
	err       error
	gotSystem string
	gotUser   string
}

func (f *fakeCompleter) Complete(_ context.Context, system, user string, _ *classify.Schema) (string, error) {
	f.gotSystem = system
	f.gotUser = user
	return f.response, f.err
}

func analyzedIncident() model.Incident {
	return model.Incident{
		Fingerprint:      "abc123def4567890",
		PipelineName:     "ACN Nightly",
		Stage:            "e2e",
		Job:              "cilium-overlay",
		Category:         model.CategoryPipelineInfraConfig,
		Confidence:       0.35,
		ConfidenceBand:   model.BandLow,
		RootCauseSummary: "The CNS image tag does not exist.",
		AnalysisStatus:   model.StatusAnalyzed,
	}
}

func TestShouldConsider(t *testing.T) {
	tests := []struct {
		name       string
		rc         model.RunContext
		inc        model.Incident
		wantOK     bool
		reasonHint string
	}{
		{
			name:   "non-PR analyzed build is considered",
			rc:     model.RunContext{},
			inc:    analyzedIncident(),
			wantOK: true,
		},
		{
			name:       "PR build is skipped",
			rc:         model.RunContext{IsPR: true},
			inc:        analyzedIncident(),
			reasonHint: "Pull-request build",
		},
		{
			name: "failed analysis is skipped",
			rc:   model.RunContext{},
			inc: func() model.Incident {
				inc := analyzedIncident()
				inc.AnalysisStatus = model.StatusAnalysisFailed
				return inc
			}(),
			reasonHint: "no conclusion to escalate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, reason := ShouldConsider(tt.rc, tt.inc)
			if ok != tt.wantOK {
				t.Fatalf("ShouldConsider: got %v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK {
				if reason != "" {
					t.Errorf("expected no reason when considered, got %q", reason)
				}
				return
			}
			if !strings.Contains(reason, tt.reasonHint) {
				t.Errorf("reason %q does not mention %q", reason, tt.reasonHint)
			}
		})
	}
}

// TestDecideIsNotGatedByCategoryOrConfidence is the point of this package: a
// low-confidence, non-regression incident still escalates when the model says the
// mechanism lives in this repository.
func TestDecideIsNotGatedByCategoryOrConfidence(t *testing.T) {
	c := &fakeCompleter{response: `{
      "needed": true,
      "reason": "The bad image tag is committed in .pipelines.",
      "title": "CNS image tag in the E2E template does not exist",
      "labels": ["bug", "cns"],
      "fixDirection": "Point the template at a published tag.",
      "suggestedFiles": [".pipelines/templates/e2e.yaml"],
      "blockers": ["Confirm which tag is current."]
    }`}

	got, err := NewGate(c).Decide(context.Background(), model.RunContext{}, analyzedIncident(), "report body")
	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}
	if !got.Needed {
		t.Fatal("expected escalation despite low confidence and a non-regression category")
	}
	if got.Source != model.EscalationLLM {
		t.Errorf("source: got %s, want llm", got.Source)
	}
	if got.Title != "CNS image tag in the E2E template does not exist" {
		t.Errorf("title: got %q", got.Title)
	}
	if strings.Join(got.Labels, ",") != "bug,cns" {
		t.Errorf("labels: got %v", got.Labels)
	}
	if len(got.SuggestedFiles) != 1 || got.SuggestedFiles[0] != ".pipelines/templates/e2e.yaml" {
		t.Errorf("suggestedFiles: got %v", got.SuggestedFiles)
	}
	if len(got.Blockers) != 1 {
		t.Errorf("blockers: got %v", got.Blockers)
	}
}

func TestDecideDeclineKeepsReasonAndDropsDraft(t *testing.T) {
	c := &fakeCompleter{response: `{
      "needed": false,
      "reason": "The subscription ran out of public IP quota; nothing here is fixable in code.",
      "title": "leftover title",
      "labels": ["bug"],
      "fixDirection": "leftover direction",
      "suggestedFiles": ["x.go"],
      "blockers": []
    }`}

	got, err := NewGate(c).Decide(context.Background(), model.RunContext{}, analyzedIncident(), "report body")
	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}
	if got.Needed {
		t.Fatal("expected no escalation")
	}
	if got.Reason == "" {
		t.Error("a decline must carry a reason")
	}
	// Draft fields are meaningless on a decline and would otherwise show up in
	// report.md as if an issue had been drafted.
	if got.Title != "" || got.FixDirection != "" || got.Labels != nil || got.SuggestedFiles != nil {
		t.Errorf("expected draft fields dropped on decline, got %+v", got)
	}
}

func TestDecideRejectsDisallowedLabels(t *testing.T) {
	c := &fakeCompleter{response: `{
      "needed": true,
      "reason": "actionable",
      "title": "a title",
      "labels": ["bug", "P0", "security", "CNS", "bug"],
      "fixDirection": "",
      "suggestedFiles": [],
      "blockers": []
    }`}

	got, err := NewGate(c).Decide(context.Background(), model.RunContext{}, analyzedIncident(), "report")
	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}
	// "P0" and "security" are not in the allowlist; "CNS" normalizes to the
	// allowed "cns"; the duplicate "bug" collapses.
	if strings.Join(got.Labels, ",") != "bug,cns" {
		t.Errorf("labels: got %v, want [bug cns]", got.Labels)
	}
}

func TestDecideRejectsIncompleteResponses(t *testing.T) {
	tests := []struct {
		name     string
		response string
	}{
		{"no reason", `{"needed": false, "reason": "  ", "title": "", "labels": [], "fixDirection": "", "suggestedFiles": [], "blockers": []}`},
		{"needed without title", `{"needed": true, "reason": "actionable", "title": "", "labels": [], "fixDirection": "", "suggestedFiles": [], "blockers": []}`},
		{"malformed json", `not json`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &fakeCompleter{response: tt.response}
			if _, err := NewGate(c).Decide(context.Background(), model.RunContext{}, analyzedIncident(), "report"); err == nil {
				t.Fatal("expected an error so the caller records a failed gate")
			}
		})
	}
}

func TestDecidePropagatesCompleterError(t *testing.T) {
	c := &fakeCompleter{err: errors.New("unauthorized")}
	if _, err := NewGate(c).Decide(context.Background(), model.RunContext{}, analyzedIncident(), "report"); err == nil {
		t.Fatal("expected the completer error to propagate")
	}
}

// TestDecidePromptGrounding checks the gate is given the rendered report plus the
// change under test, which the report itself does not carry.
func TestDecidePromptGrounding(t *testing.T) {
	c := &fakeCompleter{response: `{"needed": false, "reason": "environmental", "title": "", "labels": [], "fixDirection": "", "suggestedFiles": [], "blockers": []}`}
	rc := model.RunContext{ChangedFiles: []string{"cns/restserver/api.go"}}
	inc := analyzedIncident()
	inc.RootCauseSources = []model.RootCauseRef{{File: "logs/cns.log", Line: 42, Explanation: "nil map write"}}

	if _, err := NewGate(c).Decide(context.Background(), rc, inc, "RENDERED REPORT BODY"); err != nil {
		t.Fatalf("Decide failed: %v", err)
	}

	for _, want := range []string{"cns/restserver/api.go", "logs/cns.log:42", "RENDERED REPORT BODY", inc.Fingerprint} {
		if !strings.Contains(c.gotUser, want) {
			t.Errorf("user prompt is missing %q", want)
		}
	}
	if !strings.Contains(c.gotSystem, "Escalation gate") {
		t.Error("expected the escalation playbook as the system prompt")
	}
}

// TestDecidePromptNotesMissingChangeContext guards against the gate reading an
// absent diff as evidence that the failure is environmental.
func TestDecidePromptNotesMissingChangeContext(t *testing.T) {
	c := &fakeCompleter{response: `{"needed": false, "reason": "environmental", "title": "", "labels": [], "fixDirection": "", "suggestedFiles": [], "blockers": []}`}
	if _, err := NewGate(c).Decide(context.Background(), model.RunContext{}, analyzedIncident(), "report"); err != nil {
		t.Fatalf("Decide failed: %v", err)
	}
	if !strings.Contains(c.gotUser, "Do not infer from its absence") {
		t.Error("expected the prompt to warn about absent change context")
	}
}

func TestDecideTruncatesOversizedReport(t *testing.T) {
	c := &fakeCompleter{response: `{"needed": false, "reason": "environmental", "title": "", "labels": [], "fixDirection": "", "suggestedFiles": [], "blockers": []}`}
	huge := strings.Repeat("x", maxReportChars*2)

	if _, err := NewGate(c).Decide(context.Background(), model.RunContext{}, analyzedIncident(), huge); err != nil {
		t.Fatalf("Decide failed: %v", err)
	}
	if len(c.gotUser) > maxReportChars*2 {
		t.Errorf("prompt not bounded: %d chars", len(c.gotUser))
	}
	if !strings.Contains(c.gotUser, "[report truncated]") {
		t.Error("expected a truncation notice")
	}
}

func TestTitle(t *testing.T) {
	inc := analyzedIncident()

	got := Title(inc, model.Escalation{Title: "CNS drops the endpoint route"})
	if got != "[FAA] CNS drops the endpoint route (abc123def456)" {
		t.Errorf("title: got %q", got)
	}

	// A multi-line title would corrupt the line-oriented metadata header.
	got = Title(inc, model.Escalation{Title: "line one\nline two"})
	if strings.Contains(got, "\n") {
		t.Errorf("expected a single-line title, got %q", got)
	}

	if got := Title(inc, model.Escalation{}); !strings.HasPrefix(got, "[FAA] ") {
		t.Errorf("expected a fallback title, got %q", got)
	}
}

func TestRender(t *testing.T) {
	inc := analyzedIncident()
	inc.BuildID = "998877"
	inc.RecommendedOwner = "acn-cni"
	inc.TopEvidence = []string{"ErrImagePull azure-cns"}
	inc.RootCauseSources = []model.RootCauseRef{{
		File: ".pipelines/templates/e2e.yaml", Line: 12, Snippet: "image: azure-cns:bad-tag", Explanation: "tag does not exist",
	}}
	e := model.Escalation{
		Needed:         true,
		Reason:         "The bad tag is committed here.",
		Title:          "CNS image tag does not exist",
		Labels:         []string{"bug", "cns"},
		FixDirection:   "Point the template at a published tag.",
		SuggestedFiles: []string{".pipelines/templates/e2e.yaml"},
		Blockers:       []string{"Confirm the current tag."},
		Source:         model.EscalationLLM,
	}

	body := Render(inc, e)

	for _, want := range []string{
		"<!-- faa-issue:v1",
		"fingerprint: abc123def4567890",
		"labels: bug,cns",
		"owner: acn-cni",
		FingerprintMarker(inc.Fingerprint),
		"## Why this was raised",
		"The bad tag is committed here.",
		"## Cited evidence",
		"`.pipelines/templates/e2e.yaml:12`",
		"image: azure-cns:bad-tag",
		"## Fix direction",
		"## Files to look at first",
		"## Open questions before fixing",
		"acn-faa-fix-from-issue",
		"draft",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("issue body is missing %q", want)
		}
	}

	// The metadata header must stay parseable line-by-line by the workflow.
	header, _, ok := strings.Cut(body, "-->")
	if !ok {
		t.Fatal("metadata header is not terminated")
	}
	for _, line := range strings.Split(strings.TrimPrefix(header, "<!-- faa-issue:v1\n"), "\n") {
		if line == "" {
			continue
		}
		if !strings.Contains(line, ": ") {
			t.Errorf("metadata line is not key/value: %q", line)
		}
	}
}

// TestRenderMultilineReasonStaysOutOfHeader guards the shell-parsed header
// against a model response that spans lines.
func TestRenderMultilineReasonStaysOutOfHeader(t *testing.T) {
	inc := analyzedIncident()
	body := Render(inc, model.Escalation{
		Needed: true,
		Reason: "first line\nsecond line",
		Title:  "a title\nwith a newline",
	})

	header, _, _ := strings.Cut(body, "-->")
	if strings.Contains(header, "with a newline\n") {
		t.Error("a multi-line title leaked into the metadata header")
	}
	if !strings.Contains(body, "second line") {
		t.Error("the full reason should still appear in the body")
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	inc := analyzedIncident()

	path, err := WriteFile(dir, inc, model.Escalation{Needed: true, Reason: "actionable", Title: "a title"})
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if path != filepath.Join(dir, FileName) {
		t.Errorf("path: got %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading issue.md: %v", err)
	}
	if !strings.Contains(string(data), inc.Fingerprint) {
		t.Error("expected the fingerprint in issue.md")
	}
}

// TestAllowedLabelsIsACopy ensures a caller cannot mutate the allowlist that the
// schema and the workflow both depend on.
func TestAllowedLabelsIsACopy(t *testing.T) {
	got := AllowedLabels()
	got[0] = "mutated"
	if AllowedLabels()[0] == "mutated" {
		t.Error("AllowedLabels leaked the backing array")
	}
}

// TestSchemaLabelEnumMatchesAllowlist keeps the model-facing enum and the
// server-side filter from drifting apart.
func TestSchemaLabelEnumMatchesAllowlist(t *testing.T) {
	def := string(escalationSchema().Definition)
	for _, l := range AllowedLabels() {
		if !strings.Contains(def, `"`+l+`"`) {
			t.Errorf("schema enum is missing allowed label %q", l)
		}
	}
}
