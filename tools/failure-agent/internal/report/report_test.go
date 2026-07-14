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
		Source:           "llm",
	}
}

func TestRenderMarkdownNodeAssessment(t *testing.T) {
	inc := Build(time.Unix(0, 0), model.RunContext{}, model.Fingerprint{Hash: "x"},
		model.Classification{
			Category:         model.CategoryPipelineInfraConfig,
			Confidence:       0.6,
			RootCauseSummary: "CNS restarted after the node rebooted.",
			NodeAssessment:   "Node aks-nodepool1-vmss000000 went NotReady after a RebootScheduled event; CNS restart is a side effect.",
			Source:           "llm",
		}, nil, model.Evidence{})

	md := RenderMarkdown(inc)
	if !strings.Contains(md, "### Node / nodepool health") {
		t.Error("expected node/nodepool health section in report")
	}
	if !strings.Contains(md, "RebootScheduled") {
		t.Error("expected node assessment text in report")
	}
}

func TestRenderMarkdownOmitsEmptyNodeAssessment(t *testing.T) {
	md := RenderMarkdown(Build(time.Unix(0, 0), model.RunContext{}, model.Fingerprint{Hash: "x"}, sampleClassification(), nil, model.Evidence{}))
	if strings.Contains(md, "Node / nodepool health") {
		t.Error("did not expect node section when assessment is empty")
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

func TestBuildDefaultsToAnalyzed(t *testing.T) {
	inc := Build(time.Unix(0, 0), model.RunContext{}, model.Fingerprint{Hash: "x"}, sampleClassification(), nil, model.Evidence{})
	if inc.AnalysisStatus != model.StatusAnalyzed {
		t.Errorf("status: got %s, want analyzed", inc.AnalysisStatus)
	}
}

func TestRenderMarkdownShowsAnalysisFailedBanner(t *testing.T) {
	inc := Build(time.Unix(0, 0), model.RunContext{}, model.Fingerprint{Hash: "x"}, sampleClassification(), nil, model.Evidence{})
	inc.AnalysisStatus = model.StatusAnalysisFailed
	inc.AnalysisError = "azure openai unauthorized"
	md := RenderMarkdown(inc)

	if !strings.Contains(md, "Automated analysis failed") {
		t.Errorf("expected analysis-failed banner, got:\n%s", md)
	}
	if !strings.Contains(md, "azure openai unauthorized") {
		t.Error("expected analysis error reason in markdown")
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
