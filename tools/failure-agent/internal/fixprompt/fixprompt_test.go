package fixprompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

// gatedIncident is a high-confidence pr_regression incident that passes the gate.
func gatedIncident() model.Incident {
	return model.Incident{
		PipelineName:     "Azure CNI Cilium",
		BuildID:          "123456",
		Commit:           "abc1234",
		Stage:            "Cilium",
		Job:              "e2e",
		Fingerprint:      "deadbeefcafef00dbaadf00d",
		Category:         model.CategoryPRRegression,
		Confidence:       0.87,
		ConfidenceBand:   model.BandHigh,
		RootCauseSummary: "The new CNI conflist drops egress traffic.",
		RecommendedOwner: "johnpayne@microsoft.com",
		ProposedFix:      "Restore the egress accept rule in the conflist template.",
		AnalysisStatus:   model.StatusAnalyzed,
		TopEvidence:      []string{"iptables DROP on egress chain"},
		ErrorSnippets: []model.ErrorSnippet{
			{File: "cilium.log", Line: 42, Snippet: "level=error msg=egress blocked"},
		},
	}
}

func TestShouldEmit(t *testing.T) {
	nonPR := model.RunContext{IsPR: false}
	tests := []struct {
		name string
		rc   model.RunContext
		inc  model.Incident
		want bool
	}{
		{"gated", nonPR, gatedIncident(), true},
		{"pr build", model.RunContext{IsPR: true}, gatedIncident(), false},
		{
			name: "not analyzed",
			rc:   nonPR,
			inc:  func() model.Incident { i := gatedIncident(); i.AnalysisStatus = model.StatusAnalysisFailed; return i }(),
			want: false,
		},
		{
			name: "wrong category",
			rc:   nonPR,
			inc:  func() model.Incident { i := gatedIncident(); i.Category = model.CategoryKnownFlake; return i }(),
			want: false,
		},
		{
			name: "not high confidence",
			rc:   nonPR,
			inc:  func() model.Incident { i := gatedIncident(); i.ConfidenceBand = model.BandMedium; return i }(),
			want: false,
		},
		{
			name: "no proposed fix",
			rc:   nonPR,
			inc:  func() model.Incident { i := gatedIncident(); i.ProposedFix = "  "; return i }(),
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldEmit(tc.rc, tc.inc); got != tc.want {
				t.Errorf("ShouldEmit = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRenderContainsSections(t *testing.T) {
	md := Render(gatedIncident())

	for _, want := range []string{
		"<!-- faa-fix:v1",
		"# [FAA] Draft fix: Azure CNI Cilium regression (deadbeefcafe)",
		"## Failure context",
		"## Root cause",
		"The new CNI conflist drops egress traffic.",
		"## Proposed fix direction",
		"Restore the egress accept rule",
		"## Top evidence",
		"## Evidence snippets",
		"cilium.log:42",
		"## Instructions for the coding agent",
		"agents.md",
		"draft",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("rendered fix.md missing %q", want)
		}
	}
}

// TestRenderMetadataHeader verifies the header is a line-oriented block the
// workflow can parse, and that free-text values stay on a single line.
func TestRenderMetadataHeader(t *testing.T) {
	inc := gatedIncident()
	inc.RecommendedOwner = "team\nowner@microsoft.com"

	meta := parseMetadata(t, Render(inc))

	checks := map[string]string{
		"fingerprint":    "deadbeefcafef00dbaadf00d",
		"category":       "pr_regression",
		"confidence":     "0.87",
		"confidenceBand": "high",
		"pipeline":       "Azure CNI Cilium",
		"buildId":        "123456",
		"commit":         "abc1234",
		"owner":          "team owner@microsoft.com",
	}
	for k, want := range checks {
		if got := meta[k]; got != want {
			t.Errorf("metadata[%q] = %q, want %q", k, got, want)
		}
	}
	if !strings.HasPrefix(meta["title"], "[FAA] Draft fix:") {
		t.Errorf("metadata title = %q", meta["title"])
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	inc := gatedIncident()

	path, err := WriteFile(dir, inc)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if path != filepath.Join(dir, FileName) {
		t.Errorf("path = %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != Render(inc) {
		t.Error("file content does not match Render output")
	}
}

// parseMetadata extracts the key: value pairs from the faa-fix header block.
func parseMetadata(t *testing.T, md string) map[string]string {
	t.Helper()
	start := strings.Index(md, "<!-- faa-fix:v1")
	if start < 0 {
		t.Fatal("no faa-fix metadata header")
	}
	end := strings.Index(md[start:], "-->")
	if end < 0 {
		t.Fatal("unterminated metadata header")
	}
	block := md[start : start+end]
	out := map[string]string{}
	for _, line := range strings.Split(block, "\n") {
		if strings.HasPrefix(line, "<!--") {
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		out[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}
	return out
}
