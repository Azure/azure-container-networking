package weekly

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/classify"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

// maxDigests caps how many per-incident digests are sent to the model, and
// maxSummaryChars caps each incident's summary excerpt, to bound the prompt.
const (
	maxDigests      = 80
	maxSummaryChars = 240
)

// Completer is the minimal LLM capability the weekly synthesis needs. It matches
// the per-run classifier's ChatCompleter so the same Azure OpenAI client backs
// both paths; declaring it here (consumer-side) keeps weekly decoupled from any
// specific SDK.
type Completer interface {
	Complete(ctx context.Context, system, user string, schema *classify.Schema) (string, error)
}

// Summary is the model-synthesized trends digest rendered onto the weekly card.
type Summary struct {
	// Headline is a one-line takeaway for the card title/lead.
	Headline string `json:"headline"`
	// Narrative is the Markdown trends writeup rendered as the card body.
	Narrative string `json:"narrative"`
	// KeyTrends are the notable recurring patterns worth calling out.
	KeyTrends []string `json:"keyTrends"`
	// Recommendations are the concrete follow-ups for the coming week.
	Recommendations []string `json:"recommendations"`
}

// Synthesize asks the model to identify the week's trends from the deterministic
// stats and the per-incident digests. A malformed or empty response is an error
// so the caller can fall back to a deterministic summary.
func Synthesize(ctx context.Context, c Completer, stats Stats, incidents []model.Incident) (Summary, error) {
	raw, err := c.Complete(ctx, weeklySystemPrompt, weeklyUserPrompt(stats, incidents), weeklySchema())
	if err != nil {
		return Summary{}, fmt.Errorf("weekly synthesis completion: %w", err)
	}
	var s Summary
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return Summary{}, fmt.Errorf("parsing weekly synthesis response: %w", err)
	}
	if strings.TrimSpace(s.Narrative) == "" {
		return Summary{}, errors.New("weekly synthesis returned empty narrative")
	}
	return s, nil
}

// FallbackSummary builds a deterministic trends summary from the stats alone. It
// is used when the LLM is unavailable or its response cannot be used, so the
// weekly card still posts hard numbers rather than nothing.
func FallbackSummary(stats Stats) Summary {
	var b strings.Builder
	fmt.Fprintf(&b, "Automated trend synthesis was unavailable; showing the deterministic aggregation of %d incident(s).\n\n",
		stats.TotalIncidents)
	if len(stats.CategoryCounts) > 0 {
		b.WriteString("Categories:\n")
		for _, c := range stats.CategoryCounts {
			fmt.Fprintf(&b, "- %s — %d\n", c.Label, c.Count)
		}
	}

	trends := make([]string, 0, len(stats.TopRecurring))
	for _, r := range stats.TopRecurring {
		fp := r.Fingerprint
		if len(fp) > 12 {
			fp = fp[:12]
		}
		trends = append(trends, fmt.Sprintf("`%s` recurred %d× (%s)", fp, r.Count, emptyDash(r.Category)))
	}

	headline := fmt.Sprintf("%d failure incidents analyzed this week", stats.TotalIncidents)
	return Summary{Headline: headline, Narrative: strings.TrimSpace(b.String()), KeyTrends: trends}
}

// weeklyUserPrompt renders the stats block and the capped per-incident digests
// the model reasons over.
func weeklyUserPrompt(stats Stats, incidents []model.Incident) string {
	var b strings.Builder
	b.WriteString("Aggregated statistics for the window (authoritative counts — cite these, do not recompute):\n")
	if statsJSON, err := json.MarshalIndent(stats, "", "  "); err == nil {
		b.Write(statsJSON)
		b.WriteString("\n\n")
	}

	b.WriteString("Per-incident digests (sample; may be truncated):\n")
	n := len(incidents)
	if n > maxDigests {
		n = maxDigests
	}
	for i := 0; i < n; i++ {
		inc := incidents[i]
		fp := inc.Fingerprint
		if len(fp) > 12 {
			fp = fp[:12]
		}
		summary := oneLine(firstNonEmpty(inc.RootCauseSummary, inc.FinalVerdict))
		if len(summary) > maxSummaryChars {
			summary = summary[:maxSummaryChars] + "…"
		}
		fmt.Fprintf(&b, "- [%s] category=%s fp=%s pipeline=%s failingUnit=%q owner=%q: %s\n",
			inc.GeneratedAt.Format("2006-01-02"),
			emptyDash(string(inc.Category)),
			emptyDash(fp),
			emptyDash(inc.PipelineName),
			oneLine(inc.FailingUnit),
			oneLine(inc.RecommendedOwner),
			summary,
		)
	}
	return b.String()
}

// weeklySystemPrompt is the trends-analyst contract for the weekly digest.
//
//go:embed weekly-trends-playbook.md
var weeklySystemPrompt string

// weeklySchema constrains the synthesis output to the Summary shape.
func weeklySchema() *classify.Schema {
	def := `{
  "type": "object",
  "additionalProperties": false,
  "required": ["headline", "narrative", "keyTrends", "recommendations"],
  "properties": {
    "headline": {"type": "string"},
    "narrative": {"type": "string"},
    "keyTrends": {"type": "array", "items": {"type": "string"}},
    "recommendations": {"type": "array", "items": {"type": "string"}}
  }
}`
	return &classify.Schema{Name: "weekly_trends", Definition: json.RawMessage(def)}
}
