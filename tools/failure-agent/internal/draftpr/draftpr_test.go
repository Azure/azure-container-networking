package draftpr

import (
	"context"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/publish"
)

type fakeClient struct {
	got publish.V2JobRequest
	err error
}

func (f *fakeClient) CreateV2Job(_ context.Context, req publish.V2JobRequest) (publish.JobResponse, error) {
	f.got = req
	if f.err != nil {
		return publish.JobResponse{}, f.err
	}
	return publish.JobResponse{JobID: "job-9", State: "Queued"}, nil
}

func analyzedRegression() model.Incident {
	return model.Incident{
		AnalysisStatus: model.StatusAnalyzed,
		Category:       model.CategoryPRRegression,
		ConfidenceBand: model.BandHigh,
		ProposedFix:    "adjust the retry timeout",
	}
}

func TestShouldOpen(t *testing.T) {
	nonPR := model.RunContext{IsPR: false}
	tests := []struct {
		name string
		rc   model.RunContext
		inc  model.Incident
		want bool
	}{
		{"happy path", nonPR, analyzedRegression(), true},
		{"pr build blocked", model.RunContext{IsPR: true}, analyzedRegression(), false},
		{"not analyzed", nonPR, func() model.Incident {
			i := analyzedRegression()
			i.AnalysisStatus = model.StatusAnalysisFailed
			return i
		}(), false},
		{"wrong category", nonPR, func() model.Incident { i := analyzedRegression(); i.Category = model.CategoryKnownFlake; return i }(), false},
		{"low confidence", nonPR, func() model.Incident { i := analyzedRegression(); i.ConfidenceBand = model.BandLow; return i }(), false},
		{"no proposed fix", nonPR, func() model.Incident { i := analyzedRegression(); i.ProposedFix = "  "; return i }(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldOpen(tt.rc, tt.inc); got != tt.want {
				t.Errorf("ShouldOpen = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOpenBuildsAgencyRequest(t *testing.T) {
	fc := &fakeClient{}
	res, err := Open(context.Background(), fc, Params{
		Organization: "msazure",
		Project:      "proj",
		Repository:   "acn",
		Branch:       "master",
		Prompt:       "fix it",
		Fingerprint:  "abc123",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if res.JobID != "job-9" {
		t.Errorf("JobID = %q, want job-9", res.JobID)
	}

	repo := fc.got.Context.Repository
	if repo.Type != publish.RepoTypeADO || repo.Organization != "msazure" || repo.Project != "proj" || repo.Repository != "acn" || repo.Branch != "master" {
		t.Errorf("repository = %+v", repo)
	}
	if fc.got.CustomAgent == nil || fc.got.CustomAgent.Name != CodingAgent || fc.got.CustomAgent.PluginName != PluginAgencyCoding {
		t.Errorf("customAgent = %+v", fc.got.CustomAgent)
	}
	if len(fc.got.Plugins) != 1 || fc.got.Plugins[0].Name != PluginAgencyCoding {
		t.Errorf("plugins = %+v", fc.got.Plugins)
	}
	if fc.got.Metadata["faaFingerprint"] != "abc123" {
		t.Errorf("metadata fingerprint = %q", fc.got.Metadata["faaFingerprint"])
	}
}

func TestOpenRequiresCoordinates(t *testing.T) {
	fc := &fakeClient{}
	if _, err := Open(context.Background(), fc, Params{Project: "p", Repository: "r", Prompt: "x"}); err == nil {
		t.Error("expected error when organization is empty")
	}
	if _, err := Open(context.Background(), fc, Params{Organization: "o", Project: "p", Repository: "r"}); err == nil {
		t.Error("expected error when prompt is empty")
	}
}

func TestBuildPromptIncludesFixAndDraftInstruction(t *testing.T) {
	inc := analyzedRegression()
	inc.RootCauseSummary = "nil pointer in reconciler"
	inc.PipelineName = "acn-ci"
	inc.TopEvidence = []string{"panic: nil map"}

	p := BuildPrompt(inc)
	for _, want := range []string{"DRAFT", "adjust the retry timeout", "nil pointer in reconciler", "acn-ci", "panic: nil map"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q\n%s", want, p)
		}
	}
}

func TestBaseBranchFromRef(t *testing.T) {
	if got := BaseBranchFromRef("refs/heads/master"); got != "master" {
		t.Errorf("got %q, want master", got)
	}
	if got := BaseBranchFromRef("feature/x"); got != "feature/x" {
		t.Errorf("got %q, want feature/x", got)
	}
}

func TestOrgFromCollectionURI(t *testing.T) {
	tests := map[string]string{
		"https://dev.azure.com/msazure/":    "msazure",
		"https://dev.azure.com/msazure":     "msazure",
		"https://msazure.visualstudio.com/": "msazure",
		"":                                  "",
	}
	for in, want := range tests {
		if got := OrgFromCollectionURI(in); got != want {
			t.Errorf("OrgFromCollectionURI(%q) = %q, want %q", in, got, want)
		}
	}
}
