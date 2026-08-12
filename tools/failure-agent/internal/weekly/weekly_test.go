package weekly

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/classify"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

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

func sampleIncidents() []model.Incident {
	return []model.Incident{
		{
			PipelineName: "cni-load", Category: model.CategoryPipelineInfraConfig,
			Fingerprint: "abc123def4560000", RecommendedOwner: "aks-node",
			RootCauseSummary: "AzSecPack install-packages.sh exit 1", AnalysisStatus: model.StatusAnalyzed,
		},
		{
			PipelineName: "cni-load", Category: model.CategoryPipelineInfraConfig,
			Fingerprint: "abc123def4560000", RecommendedOwner: "aks-node",
			RootCauseSummary: "AzSecPack install-packages.sh exit 1", AnalysisStatus: model.StatusAnalyzed,
		},
		{
			PipelineName: "cilium-e2e", Category: model.CategoryPRRegression,
			Fingerprint: "99ff00", RecommendedOwner: "acn-cni",
			RootCauseSummary: "endpoint plumbing regression", AnalysisStatus: model.StatusAnalyzed,
		},
		{
			PipelineName: "cilium-e2e", Category: model.CategoryUnknownNeedsHuman,
			Fingerprint: "", AnalysisStatus: model.StatusAnalysisFailed,
		},
	}
}

func TestAggregateCountsAndRanks(t *testing.T) {
	s := Aggregate(sampleIncidents())

	if s.TotalIncidents != 4 {
		t.Fatalf("TotalIncidents = %d, want 4", s.TotalIncidents)
	}
	if s.AnalyzedCount != 3 || s.FailedCount != 1 {
		t.Errorf("analyzed/failed = %d/%d, want 3/1", s.AnalyzedCount, s.FailedCount)
	}
	if len(s.TopRecurring) == 0 || s.TopRecurring[0].Fingerprint != "abc123def4560000" || s.TopRecurring[0].Count != 2 {
		t.Errorf("top recurring = %+v, want abc123def4560000 ×2 first", s.TopRecurring)
	}
	// pipeline_infra_config appears twice, the most frequent category.
	if len(s.CategoryCounts) == 0 || s.CategoryCounts[0].Label != string(model.CategoryPipelineInfraConfig) || s.CategoryCounts[0].Count != 2 {
		t.Errorf("top category = %+v, want pipeline_infra_config ×2 first", s.CategoryCounts)
	}
	if len(s.TopOwners) == 0 || s.TopOwners[0].Label != "aks-node" || s.TopOwners[0].Count != 2 {
		t.Errorf("top owner = %+v, want aks-node ×2 first", s.TopOwners)
	}
}

func TestLoadIncidentsRecursesAndSkipsJunk(t *testing.T) {
	root := t.TempDir()
	// One valid incident.json in a nested artifact subdir.
	sub := filepath.Join(root, "failureAnalysis_linux_cluster1")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	inc := model.Incident{PipelineName: "p", Category: model.CategoryKnownFlake, Fingerprint: "deadbeef"}
	data, _ := json.Marshal(inc)
	if err := os.WriteFile(filepath.Join(sub, "incident.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	// A malformed incident.json and an unrelated file must be ignored.
	if err := os.WriteFile(filepath.Join(root, "incident.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "report.md"), []byte("# ignore me"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadIncidents(root)
	if err != nil {
		t.Fatalf("LoadIncidents: %v", err)
	}
	if len(got) != 1 || got[0].Fingerprint != "deadbeef" {
		t.Fatalf("LoadIncidents = %+v, want the one valid incident", got)
	}
}

func TestSynthesizeParsesSummary(t *testing.T) {
	fc := &fakeCompleter{response: `{
		"headline": "20 incidents, infra-dominated",
		"narrative": "**pipeline_infra_config** was the majority this week.",
		"keyTrends": ["abc123 recurred 2x"],
		"recommendations": ["Fix AzSecPack install path"]
	}`}

	got, err := Synthesize(context.Background(), fc, Aggregate(sampleIncidents()), sampleIncidents())
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if got.Headline == "" || got.Narrative == "" || len(got.KeyTrends) != 1 || len(got.Recommendations) != 1 {
		t.Fatalf("summary not fully parsed: %+v", got)
	}
	if fc.gotSchema == nil || fc.gotSchema.Name != "weekly_trends" {
		t.Errorf("expected weekly_trends schema, got %+v", fc.gotSchema)
	}
	// The prompt must carry the authoritative counts and per-incident digests.
	if !strings.Contains(fc.gotUser, "totalIncidents") || !strings.Contains(fc.gotUser, "category=") {
		t.Errorf("user prompt missing stats/digests: %q", fc.gotUser)
	}
	if !strings.Contains(fc.gotSystem, "failure-trends analyst") {
		t.Errorf("system prompt missing trends-analyst role: %q", fc.gotSystem)
	}
}

func TestSynthesizeRejectsEmptyNarrative(t *testing.T) {
	fc := &fakeCompleter{response: `{"headline":"h","narrative":"   ","keyTrends":[],"recommendations":[]}`}
	if _, err := Synthesize(context.Background(), fc, Stats{}, nil); err == nil {
		t.Fatal("expected error for empty narrative")
	}
}

func TestSynthesizeErrorPropagates(t *testing.T) {
	fc := &fakeCompleter{err: errors.New("boom")}
	if _, err := Synthesize(context.Background(), fc, Stats{}, nil); err == nil {
		t.Fatal("expected completer error to propagate")
	}
}

func TestFallbackSummaryUsesStats(t *testing.T) {
	s := Aggregate(sampleIncidents())
	sum := FallbackSummary(s)
	if !strings.Contains(sum.Narrative, "deterministic") {
		t.Errorf("fallback narrative should note it is deterministic: %q", sum.Narrative)
	}
	if len(sum.KeyTrends) == 0 || !strings.Contains(sum.KeyTrends[0], "abc123def456") {
		t.Errorf("fallback trends should include the top recurring fingerprint: %+v", sum.KeyTrends)
	}
}

func TestRenderMarkdownAndWriteFiles(t *testing.T) {
	s := Aggregate(sampleIncidents())
	sum := Summary{
		Headline:        "infra week",
		Narrative:       "**pipeline_infra_config** dominated.",
		KeyTrends:       []string{"abc123 recurred 2x"},
		Recommendations: []string{"fix install path"},
	}
	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	wi := Build(now, now.AddDate(0, 0, -7), 7, s, sum)

	md := RenderMarkdown(wi)
	for _, want := range []string{
		"Weekly Trends", "infra week", "**pipeline_infra_config** dominated",
		"Key trends", "Recommendations", "Top recurring signatures", "abc123def456",
		"**Incidents:** 4 (analyzed: 3, analysis failed: 1)",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q\n---\n%s", want, md)
		}
	}

	dir := t.TempDir()
	if err := WriteFiles(dir, wi); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, JSONFile))
	if err != nil {
		t.Fatalf("reading weekly-incident.json: %v", err)
	}
	var round WeeklyIncident
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatalf("weekly-incident.json not valid: %v", err)
	}
	if round.TotalIncidents != 4 || round.Headline != "infra week" {
		t.Errorf("round-tripped weekly incident wrong: %+v", round)
	}
	if _, err := os.Stat(filepath.Join(dir, MarkdownFile)); err != nil {
		t.Errorf("weekly-report.md not written: %v", err)
	}
}
