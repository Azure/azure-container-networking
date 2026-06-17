package report

import (
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

func sampleClassification() model.Classification {
	return model.Classification{
		Category:         model.CategoryUnknownNeedsHuman,
		Confidence:       0,
		RootCauseSummary: "Unclassified failure.",
		TopEvidence:      []string{"some error line"},
		Source:           "deterministic",
	}
}

func TestBuildAppliesPolicy(t *testing.T) {
	rc := model.RunContext{PipelineName: "ACN", StageName: "Cilium", SourceCommitID: "abc123"}
	inc := Build(time.Unix(0, 0), rc, model.Fingerprint{Hash: "deadbeef"}, sampleClassification(), nil, model.Evidence{})

	if inc.ConfidenceBand != model.BandLow {
		t.Errorf("band: got %s", inc.ConfidenceBand)
	}
	if inc.RetentionDecision != model.RetentionRetainTTL {
		t.Errorf("retention: got %s", inc.RetentionDecision)
	}
	if inc.Commit != "abc123" {
		t.Errorf("commit: got %q", inc.Commit)
	}
	if inc.Fingerprint != "deadbeef" {
		t.Errorf("fingerprint: got %q", inc.Fingerprint)
	}
}

func TestRenderMarkdownContainsMarkerAndFields(t *testing.T) {
	inc := Build(time.Unix(0, 0), model.RunContext{PipelineName: "ACN"}, model.Fingerprint{Hash: "deadbeef"}, sampleClassification(), nil, model.Evidence{})
	md := RenderMarkdown(inc)

	if !strings.HasPrefix(md, CommentMarker("deadbeef")) {
		t.Errorf("expected marker as first line, got:\n%s", md)
	}
	for _, want := range []string{"ACN Pipeline Failure Analysis", "unknown_needs_human", "Recommended next action"} {
		if !strings.Contains(md, want) {
			t.Errorf("expected %q in markdown", want)
		}
	}
}
