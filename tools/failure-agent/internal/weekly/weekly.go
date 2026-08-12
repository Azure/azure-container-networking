// Package weekly aggregates a window of failure-agent incidents (the per-run
// incident.json artifacts collected across a week) and synthesizes a trends
// digest for the weekly Teams card. It reuses the same Azure OpenAI completer
// as the per-run classifier to write the narrative, and emits two artifacts:
// a human-readable weekly-report.md and a machine-readable weekly-incident.json
// that the notify-weekly.sh bridge renders into the Adaptive Card.
package weekly

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/report"
)

// MarkdownFile and JSONFile are the artifact names written by WriteFiles.
const (
	MarkdownFile = "weekly-report.md"
	JSONFile     = "weekly-incident.json"
)

// topRecurringLimit and topOwnerLimit cap how many rows each ranking surfaces.
const (
	topRecurringLimit = 8
	topOwnerLimit     = 8
)

// FingerprintCount is one recurring failure signature and how often it recurred
// across the window, with a representative summary for context.
type FingerprintCount struct {
	Fingerprint string `json:"fingerprint"`
	Count       int    `json:"count"`
	Category    string `json:"category"`
	Summary     string `json:"summary"`
}

// LabelCount is a simple label→count ranking row (owner, category, pipeline).
type LabelCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// Stats is the deterministic aggregation over the window's incidents. It grounds
// the LLM synthesis and is emitted verbatim so the card carries hard numbers.
type Stats struct {
	TotalIncidents int                `json:"totalIncidents"`
	AnalyzedCount  int                `json:"analyzedCount"`
	FailedCount    int                `json:"failedCount"`
	CategoryCounts []LabelCount       `json:"categoryCounts"`
	TopRecurring   []FingerprintCount `json:"topRecurring"`
	TopOwners      []LabelCount       `json:"topOwners"`
	Pipelines      []LabelCount       `json:"pipelines"`
}

// LoadIncidents reads every incident.json under root (recursively). The weekly
// aggregation artifact directory holds one subdirectory per downloaded build
// artifact, so a recursive walk is required. Unreadable or malformed files are
// skipped rather than failing the whole digest.
func LoadIncidents(root string) ([]model.Incident, error) {
	var incidents []model.Incident
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // skip unreadable entries, keep aggregating
		}
		if d.IsDir() || !strings.EqualFold(filepath.Base(path), report.JSONFile) {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil //nolint:nilerr // skip unreadable file, keep aggregating
		}
		var inc model.Incident
		if json.Unmarshal(data, &inc) != nil {
			return nil
		}
		incidents = append(incidents, inc)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking incident directory %q: %w", root, err)
	}
	return incidents, nil
}

// Aggregate computes deterministic Stats over the window's incidents.
func Aggregate(incidents []model.Incident) Stats {
	s := Stats{TotalIncidents: len(incidents)}

	categories := map[string]int{}
	owners := map[string]int{}
	pipelines := map[string]int{}
	type fpAgg struct {
		count    int
		category string
		summary  string
	}
	fingerprints := map[string]*fpAgg{}

	for i := range incidents {
		inc := incidents[i]
		if inc.AnalysisStatus == model.StatusAnalysisFailed {
			s.FailedCount++
		} else {
			s.AnalyzedCount++
		}
		if c := string(inc.Category); c != "" {
			categories[c]++
		}
		if o := strings.TrimSpace(inc.RecommendedOwner); o != "" {
			owners[o]++
		}
		if p := strings.TrimSpace(inc.PipelineName); p != "" {
			pipelines[p]++
		}
		if fp := strings.TrimSpace(inc.Fingerprint); fp != "" {
			agg := fingerprints[fp]
			if agg == nil {
				agg = &fpAgg{category: string(inc.Category), summary: firstNonEmpty(inc.RootCauseSummary, inc.FinalVerdict)}
				fingerprints[fp] = agg
			}
			agg.count++
		}
	}

	s.CategoryCounts = rankLabels(categories, 0)
	s.TopOwners = rankLabels(owners, topOwnerLimit)
	s.Pipelines = rankLabels(pipelines, 0)

	recurring := make([]FingerprintCount, 0, len(fingerprints))
	for fp, agg := range fingerprints {
		recurring = append(recurring, FingerprintCount{
			Fingerprint: fp,
			Count:       agg.count,
			Category:    agg.category,
			Summary:     agg.summary,
		})
	}
	sort.SliceStable(recurring, func(i, j int) bool {
		if recurring[i].Count != recurring[j].Count {
			return recurring[i].Count > recurring[j].Count
		}
		return recurring[i].Fingerprint < recurring[j].Fingerprint
	})
	if len(recurring) > topRecurringLimit {
		recurring = recurring[:topRecurringLimit]
	}
	s.TopRecurring = recurring
	return s
}

// rankLabels turns a label→count map into a count-descending, label-ascending
// ranking. A non-zero limit caps the number of rows returned.
func rankLabels(counts map[string]int, limit int) []LabelCount {
	out := make([]LabelCount, 0, len(counts))
	for label, n := range counts {
		out = append(out, LabelCount{Label: label, Count: n})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// WeeklyIncident is the artifact notify-weekly.sh reads to render the card. It
// carries both the deterministic stats and the synthesized narrative.
type WeeklyIncident struct {
	GeneratedAt    time.Time `json:"generatedAt"`
	WindowDays     int       `json:"windowDays"`
	WindowStart    time.Time `json:"windowStart"`
	TotalIncidents int       `json:"totalIncidents"`

	Headline        string   `json:"headline"`
	Narrative       string   `json:"narrative"`
	KeyTrends       []string `json:"keyTrends,omitempty"`
	Recommendations []string `json:"recommendations,omitempty"`

	Stats Stats `json:"stats"`
}

// Build assembles the WeeklyIncident from the window metadata, stats, and the
// synthesized summary. now and windowStart are injected for deterministic tests.
func Build(now, windowStart time.Time, windowDays int, stats Stats, sum Summary) WeeklyIncident {
	return WeeklyIncident{
		GeneratedAt:     now.UTC(),
		WindowDays:      windowDays,
		WindowStart:     windowStart.UTC(),
		TotalIncidents:  stats.TotalIncidents,
		Headline:        sum.Headline,
		Narrative:       sum.Narrative,
		KeyTrends:       sum.KeyTrends,
		Recommendations: sum.Recommendations,
		Stats:           stats,
	}
}

// RenderMarkdown produces the human-readable weekly-report.md body.
func RenderMarkdown(wi WeeklyIncident) string {
	var b strings.Builder

	b.WriteString("## ACN Failure Analysis — Weekly Trends\n\n")
	fmt.Fprintf(&b, "**Window:** last %d days (since %s)  |  **Incidents:** %d (analyzed: %d, analysis failed: %d)\n\n",
		wi.WindowDays, wi.WindowStart.Format("2006-01-02"), wi.TotalIncidents, wi.Stats.AnalyzedCount, wi.Stats.FailedCount)

	if strings.TrimSpace(wi.Headline) != "" {
		fmt.Fprintf(&b, "**%s**\n\n", strings.TrimSpace(wi.Headline))
	}

	if strings.TrimSpace(wi.Narrative) != "" {
		b.WriteString("### Trends\n\n")
		b.WriteString(strings.TrimSpace(wi.Narrative))
		b.WriteString("\n\n")
	}

	if len(wi.KeyTrends) > 0 {
		b.WriteString("### Key trends\n\n")
		for _, t := range wi.KeyTrends {
			fmt.Fprintf(&b, "- %s\n", t)
		}
		b.WriteString("\n")
	}

	if len(wi.Recommendations) > 0 {
		b.WriteString("### Recommendations\n\n")
		for _, r := range wi.Recommendations {
			fmt.Fprintf(&b, "- %s\n", r)
		}
		b.WriteString("\n")
	}

	writeCounts(&b, "By category", wi.Stats.CategoryCounts)
	writeRecurring(&b, wi.Stats.TopRecurring)
	writeCounts(&b, "By suggested owner", wi.Stats.TopOwners)
	writeCounts(&b, "By pipeline", wi.Stats.Pipelines)

	return b.String()
}

func writeCounts(b *strings.Builder, heading string, rows []LabelCount) {
	if len(rows) == 0 {
		return
	}
	fmt.Fprintf(b, "### %s\n\n", heading)
	for _, r := range rows {
		fmt.Fprintf(b, "- %s — %d\n", r.Label, r.Count)
	}
	b.WriteString("\n")
}

func writeRecurring(b *strings.Builder, rows []FingerprintCount) {
	if len(rows) == 0 {
		return
	}
	b.WriteString("### Top recurring signatures\n\n")
	for _, r := range rows {
		fp := r.Fingerprint
		if len(fp) > 12 {
			fp = fp[:12]
		}
		fmt.Fprintf(b, "- `%s` ×%d (%s)", fp, r.Count, emptyDash(r.Category))
		if s := oneLine(r.Summary); s != "" {
			fmt.Fprintf(b, " — %s", s)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

// WriteFiles writes weekly-report.md and weekly-incident.json into dir.
func WriteFiles(dir string, wi WeeklyIncident) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	md := RenderMarkdown(wi)
	if err := os.WriteFile(filepath.Join(dir, MarkdownFile), []byte(md), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", MarkdownFile, err)
	}

	data, err := json.MarshalIndent(wi, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling weekly incident: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, JSONFile), append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", JSONFile, err)
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func emptyDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func oneLine(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
}
