package notify

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

func analyzedIncident() model.Incident {
	return model.Incident{
		AnalysisStatus:   model.StatusAnalyzed,
		Confidence:       0.9,
		ConfidenceBand:   model.BandHigh,
		Category:         model.CategoryPRRegression,
		RootCauseSummary: "the change under test broke pod networking",
		ProposedFix:      "revert the CNI config change and re-run",
		PipelineName:     "ACN-e2e",
		Repository:       "Azure/azure-container-networking",
	}
}

func TestShouldNotify(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*model.Incident)
		want bool
	}{
		{"analyzed high confidence with fix", func(*model.Incident) {}, true},
		{"exactly at threshold", func(i *model.Incident) { i.Confidence = 0.75 }, true},
		{"below threshold", func(i *model.Incident) { i.Confidence = 0.74 }, false},
		{"analysis failed", func(i *model.Incident) { i.AnalysisStatus = model.StatusAnalysisFailed }, false},
		{"no proposed fix", func(i *model.Incident) { i.ProposedFix = "" }, false},
		{"whitespace proposed fix", func(i *model.Incident) { i.ProposedFix = "   " }, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inc := analyzedIncident()
			c.mut(&inc)
			if got := ShouldNotify(inc, 0.75); got != c.want {
				t.Errorf("ShouldNotify = %v, want %v", got, c.want)
			}
		})
	}
}

func TestRenderCardWithoutMention(t *testing.T) {
	inc := analyzedIncident()
	payload := RenderCard(inc, Mention{})

	// Round-trip through JSON so we validate the shape the webhook receives.
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(raw)

	if !strings.Contains(s, "AdaptiveCard") {
		t.Errorf("card type missing: %s", s)
	}
	if !strings.Contains(s, "application/vnd.microsoft.card.adaptive") {
		t.Errorf("adaptive card contentType missing")
	}
	if !strings.Contains(s, inc.RootCauseSummary) {
		t.Errorf("root cause missing from card")
	}
	if !strings.Contains(s, inc.ProposedFix) {
		t.Errorf("proposed fix missing from card")
	}
	if strings.Contains(s, "\"msteams\"") {
		t.Errorf("mention block should be absent without a mention")
	}
}

func TestRenderCardWithMentionAndPR(t *testing.T) {
	inc := analyzedIncident()
	inc.PullRequestNumber = "1234"
	payload := RenderCard(inc, Mention{UPN: "alice@contoso.com", Name: "Alice"})

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(raw)

	if !strings.Contains(s, "\"msteams\"") {
		t.Errorf("expected msteams mention entity, got: %s", s)
	}
	if !strings.Contains(s, "alice@contoso.com") {
		t.Errorf("expected mentioned UPN in card")
	}
	if !strings.Contains(s, "<at>Alice</at>") && !strings.Contains(s, `\u003cat\u003eAlice\u003c/at\u003e`) {
		t.Errorf("expected mention token in card text, got: %s", s)
	}
	if !strings.Contains(s, "https://github.com/Azure/azure-container-networking/pull/1234") {
		t.Errorf("expected PR open-url action, got: %s", s)
	}
}

func TestRenderCardOmitsMentionWhenIncomplete(t *testing.T) {
	inc := analyzedIncident()
	payload := RenderCard(inc, Mention{UPN: "alice@contoso.com"}) // missing Name

	raw, _ := json.Marshal(payload)
	if strings.Contains(string(raw), "\"msteams\"") {
		t.Errorf("mention should be omitted when Name is empty")
	}
}
