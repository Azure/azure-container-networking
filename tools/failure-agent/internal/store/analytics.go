package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// FingerprintStat is a recurring failure and how often it has been seen.
type FingerprintStat struct {
	Fingerprint string
	Occurrences int
	LastSeen    time.Time
	Category    string
	Summary     string
}

// CountStat is a labeled count used for category and pipeline hotspots.
type CountStat struct {
	Label string
	Count int
}

// FlakinessReport is the cross-run aggregate used to spot ACN pipeline trends.
type FlakinessReport struct {
	GeneratedAt        time.Time
	TotalIncidents     int
	RecurringThreshold int
	TopFingerprints    []FingerprintStat
	CategoryHotspots   []CountStat
	PipelineHotspots   []CountStat
}

// recurringThreshold is the occurrence count at or above which a fingerprint is
// considered recurring (i.e. a flakiness signal rather than a one-off).
const recurringThreshold = 2

// Flakiness aggregates the knowledge base into a trends report. limit caps the
// number of rows per section.
func (s *Store) Flakiness(ctx context.Context, limit int) (FlakinessReport, error) {
	rep := FlakinessReport{
		GeneratedAt:        s.now().UTC(),
		RecurringThreshold: recurringThreshold,
	}

	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM incidents`).Scan(&rep.TotalIncidents); err != nil {
		return FlakinessReport{}, fmt.Errorf("counting incidents: %w", err)
	}

	fps, err := s.topFingerprints(ctx, limit)
	if err != nil {
		return FlakinessReport{}, err
	}
	rep.TopFingerprints = fps

	cat, err := s.countBy(ctx, "category", limit)
	if err != nil {
		return FlakinessReport{}, err
	}
	rep.CategoryHotspots = cat

	pipe, err := s.countBy(ctx, "pipeline", limit)
	if err != nil {
		return FlakinessReport{}, err
	}
	rep.PipelineHotspots = pipe

	return rep, nil
}

func (s *Store) topFingerprints(ctx context.Context, limit int) ([]FingerprintStat, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT f.hash, f.occurrence_count, f.last_seen,
       COALESCE(i.category, ''), COALESCE(i.summary, '')
FROM fingerprints f
LEFT JOIN incidents i ON i.id = (
	SELECT id FROM incidents WHERE fingerprint = f.hash ORDER BY created_at DESC LIMIT 1
)
WHERE f.occurrence_count >= ?
ORDER BY f.occurrence_count DESC, f.last_seen DESC
LIMIT ?`, recurringThreshold, limit)
	if err != nil {
		return nil, fmt.Errorf("querying recurring fingerprints: %w", err)
	}
	defer rows.Close()

	var out []FingerprintStat
	for rows.Next() {
		var fs FingerprintStat
		if err := rows.Scan(&fs.Fingerprint, &fs.Occurrences, &fs.LastSeen, &fs.Category, &fs.Summary); err != nil {
			return nil, fmt.Errorf("scanning fingerprint stat: %w", err)
		}
		out = append(out, fs)
	}
	return out, rows.Err()
}

// countBy groups incidents by an allow-listed column. The column is never user
// input; only "category" and "pipeline" are permitted.
func (s *Store) countBy(ctx context.Context, column string, limit int) ([]CountStat, error) {
	var col string
	switch column {
	case "category":
		col = "category"
	case "pipeline":
		col = "pipeline"
	default:
		return nil, fmt.Errorf("unsupported group column %q", column)
	}

	//nolint:gosec // col is allow-listed above, not user input.
	q := fmt.Sprintf(`SELECT %s, COUNT(*) FROM incidents GROUP BY %s ORDER BY COUNT(*) DESC, %s LIMIT ?`, col, col, col)
	rows, err := s.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("counting by %s: %w", col, err)
	}
	defer rows.Close()

	var out []CountStat
	for rows.Next() {
		var cs CountStat
		if err := rows.Scan(&cs.Label, &cs.Count); err != nil {
			return nil, fmt.Errorf("scanning count stat: %w", err)
		}
		out = append(out, cs)
	}
	return out, rows.Err()
}

// RenderFlakiness renders the report as Markdown for dashboarding or review.
func RenderFlakiness(rep FlakinessReport) string {
	var b strings.Builder
	b.WriteString("# ACN Pipeline Flakiness Insights\n\n")
	fmt.Fprintf(&b, "_Generated %s — %d incidents recorded._\n\n", rep.GeneratedAt.Format(time.RFC3339), rep.TotalIncidents)

	b.WriteString("## Recurring failures\n\n")
	if len(rep.TopFingerprints) == 0 {
		fmt.Fprintf(&b, "_No fingerprint has recurred (threshold %d+ occurrences) yet._\n\n", rep.RecurringThreshold)
	} else {
		b.WriteString("| Fingerprint | Occurrences | Category | Last seen | Summary |\n|---|---|---|---|---|\n")
		for _, f := range rep.TopFingerprints {
			fmt.Fprintf(&b, "| `%s` | %d | %s | %s | %s |\n",
				f.Fingerprint, f.Occurrences, dash(f.Category), f.LastSeen.Format(time.RFC3339), dash(oneLine(f.Summary)))
		}
		b.WriteString("\n")
	}

	writeCounts(&b, "Category hotspots", rep.CategoryHotspots)
	writeCounts(&b, "Pipeline hotspots", rep.PipelineHotspots)
	return b.String()
}

func writeCounts(b *strings.Builder, title string, stats []CountStat) {
	fmt.Fprintf(b, "## %s\n\n", title)
	if len(stats) == 0 {
		b.WriteString("_No data yet._\n\n")
		return
	}
	b.WriteString("| Label | Incidents |\n|---|---|\n")
	for _, s := range stats {
		fmt.Fprintf(b, "| %s | %d |\n", dash(s.Label), s.Count)
	}
	b.WriteString("\n")
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func oneLine(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "|", "\\|")
}
