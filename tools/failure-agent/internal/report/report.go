// Package report assembles the final Incident from the analysis stages and
// renders the two artifacts the agent emits: a human-readable report.md and a
// machine-readable incident.json.
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/policy"
)

// MarkdownFile and JSONFile are the artifact names written by WriteFiles.
const (
	MarkdownFile = "report.md"
	JSONFile     = "incident.json"
)

// Build assembles the Incident, applying policy for the retention decision and
// recommended action. now is injected for deterministic output in tests.
func Build(now time.Time, rc model.RunContext, fp model.Fingerprint, c model.Classification, matches []model.SignatureMatch, ev model.Evidence) model.Incident {
	retention := policy.Retention(c.Category, c.Confidence)
	commit := rc.SourceCommitID
	if commit == "" {
		commit = rc.CommitID
	}

	return model.Incident{
		GeneratedAt:          now.UTC(),
		PipelineName:         rc.PipelineName,
		BuildID:              rc.BuildID,
		BuildNumber:          rc.BuildNumber,
		Repository:           rc.Repository,
		PullRequestNumber:    rc.PullRequestNumber,
		Commit:               commit,
		Stage:                rc.StageName,
		Job:                  rc.JobName,
		ClusterName:          rc.ClusterName,
		ClusterType:          rc.ClusterType,
		Region:               rc.Region,
		OS:                   rc.OS,
		CNI:                  rc.CNI,
		Fingerprint:          fp.Hash,
		Category:             c.Category,
		Confidence:           c.Confidence,
		ConfidenceBand:       policy.Band(c.Confidence),
		RootCauseSummary:     c.RootCauseSummary,
		FinalVerdict:         c.FinalVerdict,
		RecommendedOwner:     c.RecommendedOwner,
		NodeAssessment:       c.NodeAssessment,
		TopAnomaly:           c.TopAnomaly,
		FailingUnit:          c.FailingUnit,
		CausalChain:          c.CausalChain,
		SymptomVsCause:       c.SymptomVsCause,
		Falsification:        c.Falsification,
		EvidenceGaps:         c.EvidenceGaps,
		KnownUnknowns:        c.KnownUnknowns,
		TopEvidence:          c.TopEvidence,
		SignatureMatches:     matches,
		EvidenceFiles:        ev.Files,
		ErrorSnippets:        ev.ErrorSnippets,
		RetentionDecision:    retention,
		RecommendedAction:    policy.RecommendedAction(c.Category, matches, retention),
		ProposedFix:          c.ProposedFix,
		AnalysisStatus:       model.StatusAnalyzed,
		ClassificationSource: c.Source,
	}
}

// CommentMarker is the hidden HTML marker keyed by fingerprint, used by the PR
// write-back to upsert (rather than duplicate) comments across reruns.
func CommentMarker(fingerprint string) string {
	return fmt.Sprintf("<!-- acn-failure-agent:%s -->", fingerprint)
}

// RenderMarkdown produces the report body. The hidden marker is the first line
// so the same body can be posted idempotently as a PR comment.
func RenderMarkdown(inc model.Incident) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s\n", CommentMarker(inc.Fingerprint))
	b.WriteString("## ACN Pipeline Failure Analysis\n\n")
	if inc.AnalysisStatus == model.StatusAnalysisFailed {
		b.WriteString("> ⚠️ **Automated analysis failed.** The evidence below was collected but the AI classifier could not produce a result. Human triage is required.\n")
		if inc.AnalysisError != "" {
			fmt.Fprintf(&b, ">\n> _Reason: %s_\n", strings.ReplaceAll(inc.AnalysisError, "\n", " "))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "**Category:** `%s`  |  **Confidence:** %s (%.2f)  |  **Fingerprint:** `%s`\n\n",
		inc.Category, inc.ConfidenceBand, inc.Confidence, inc.Fingerprint)

	writeFinalVerdict(&b, inc.FinalVerdict)

	b.WriteString("### Where\n\n")
	b.WriteString("| Field | Value |\n|---|---|\n")
	writeRow(&b, "Pipeline", inc.PipelineName)
	writeRow(&b, "Stage / Job", strings.TrimSpace(inc.Stage+" / "+inc.Job))
	writeRow(&b, "Cluster", inc.ClusterName)
	writeRow(&b, "Scenario", strings.TrimSpace(strings.Join(nonEmpty(inc.ClusterType, inc.OS, inc.CNI), " / ")))
	writeRow(&b, "Region", inc.Region)
	if inc.PullRequestNumber != "" {
		writeRow(&b, "Pull Request", "#"+inc.PullRequestNumber)
	}
	writeRow(&b, "Commit", inc.Commit)
	b.WriteString("\n")

	if strings.TrimSpace(inc.TopAnomaly) != "" {
		b.WriteString("### Most severe anomaly\n\n")
		fmt.Fprintf(&b, "%s\n\n", inc.TopAnomaly)
	}

	b.WriteString("### Likely root cause\n\n")
	fmt.Fprintf(&b, "%s\n\n", emptyDash(inc.RootCauseSummary))
	if strings.TrimSpace(inc.FailingUnit) != "" {
		fmt.Fprintf(&b, "**Failing unit:** %s\n\n", inc.FailingUnit)
	}

	writeCausalChain(&b, inc.CausalChain)
	writeSymptomVsCause(&b, inc.SymptomVsCause)
	writeFalsification(&b, inc.Falsification)

	if strings.TrimSpace(inc.NodeAssessment) != "" {
		b.WriteString("### Node / nodepool health\n\n")
		fmt.Fprintf(&b, "%s\n\n", inc.NodeAssessment)
	}

	b.WriteString("### Top evidence\n\n")
	if len(inc.TopEvidence) == 0 {
		b.WriteString("_No error lines extracted._\n\n")
	} else {
		for _, e := range inc.TopEvidence {
			fmt.Fprintf(&b, "- `%s`\n", strings.ReplaceAll(e, "`", "'"))
		}
		b.WriteString("\n")
	}

	if len(inc.ErrorSnippets) > 0 {
		b.WriteString("### Evidence snippets\n\n")
		for _, sn := range inc.ErrorSnippets {
			fmt.Fprintf(&b, "**%s:%d**\n\n", sn.File, sn.Line)
			b.WriteString("```text\n")
			b.WriteString(sn.Snippet)
			b.WriteString("\n```\n\n")
		}
	}

	if len(inc.SignatureMatches) > 0 {
		b.WriteString("### Matched signatures\n\n")
		for _, m := range inc.SignatureMatches {
			fmt.Fprintf(&b, "- **%s** (`%s`, %.2f) — %s\n", m.ID, m.Category, m.Confidence, m.Description)
		}
		b.WriteString("\n")
	}

	writeEvidenceGaps(&b, inc.EvidenceGaps)
	writeKnownUnknowns(&b, inc.KnownUnknowns)

	if inc.ProposedFix != "" {
		b.WriteString("### Proposed fix\n\n")
		fmt.Fprintf(&b, "%s\n\n", inc.ProposedFix)
	}

	b.WriteString("### Recommended next action\n\n")
	fmt.Fprintf(&b, "%s\n\n", emptyDash(inc.RecommendedAction))
	if inc.RecommendedOwner != "" {
		if strings.TrimSpace(inc.FailingUnit) != "" {
			fmt.Fprintf(&b, "**Suggested owner:** %s (owns the failing unit: %s)\n\n", inc.RecommendedOwner, inc.FailingUnit)
		} else {
			fmt.Fprintf(&b, "**Suggested owner:** %s\n\n", inc.RecommendedOwner)
		}
	}
	fmt.Fprintf(&b, "**Retention recommendation:** `%s` (advisory only — teardown is unaffected)\n\n", inc.RetentionDecision)

	fmt.Fprintf(&b, "_Classification source: %s. %d evidence file(s) collected._\n",
		inc.ClassificationSource, len(inc.EvidenceFiles))

	return b.String()
}

// WriteFiles writes report.md and incident.json into dir, creating it if needed.
func WriteFiles(dir string, inc model.Incident) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	md := RenderMarkdown(inc)
	if err := os.WriteFile(filepath.Join(dir, MarkdownFile), []byte(md), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", MarkdownFile, err)
	}

	data, err := json.MarshalIndent(inc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling incident: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, JSONFile), append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", JSONFile, err)
	}
	return nil
}

func writeRow(b *strings.Builder, key, val string) {
	fmt.Fprintf(b, "| %s | %s |\n", key, emptyDash(val))
}

func emptyDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func nonEmpty(vals ...string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}

// writeFinalVerdict renders the self-contained human verdict before details.
func writeFinalVerdict(b *strings.Builder, verdict string) {
	if strings.TrimSpace(verdict) == "" {
		return
	}
	b.WriteString("### Final verdict\n\n")
	b.WriteString(strings.TrimSpace(verdict))
	b.WriteString("\n\n")
}

// writeCausalChain renders the ordered, timestamped, cited cause->effect chain.
func writeCausalChain(b *strings.Builder, hops []model.CausalHop) {
	if len(hops) == 0 {
		return
	}
	b.WriteString("### Causal chain\n\n")
	for i, h := range hops {
		fmt.Fprintf(b, "%d. %s", i+1, emptyDash(h.Step))
		if ts := strings.TrimSpace(h.Timestamp); ts != "" {
			fmt.Fprintf(b, " _(%s)_", ts)
		}
		if cite := strings.TrimSpace(h.Citation); cite != "" {
			fmt.Fprintf(b, " — cite: `%s`", oneLine(cite))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

// writeSymptomVsCause renders the symptom/cause classification table.
func writeSymptomVsCause(b *strings.Builder, rows []model.SymptomCause) {
	if len(rows) == 0 {
		return
	}
	b.WriteString("### Symptom vs cause\n\n")
	b.WriteString("| Signal | Classification | Why |\n|---|---|---|\n")
	for _, r := range rows {
		fmt.Fprintf(b, "| %s | %s | %s |\n", cell(r.Signal), cell(r.Classification), cell(r.Justification))
	}
	b.WriteString("\n")
}

// writeFalsification renders the disconfirmation test applied to the hypothesis.
func writeFalsification(b *strings.Builder, f *model.Falsification) {
	if f == nil {
		return
	}
	b.WriteString("### Falsification\n\n")
	if strings.TrimSpace(f.Hypothesis) != "" {
		fmt.Fprintf(b, "**Hypothesis tested:** %s\n\n", f.Hypothesis)
	}
	if strings.TrimSpace(f.IfTrueExpect) != "" {
		fmt.Fprintf(b, "- If true, expect: %s\n", f.IfTrueExpect)
	}
	if strings.TrimSpace(f.IfFalseExpect) != "" {
		fmt.Fprintf(b, "- If false, expect: %s\n", f.IfFalseExpect)
	}
	if strings.TrimSpace(f.CorrelationResult) != "" {
		fmt.Fprintf(b, "- Observed correlation: %s\n", f.CorrelationResult)
	}
	if strings.TrimSpace(f.Outcome) != "" {
		fmt.Fprintf(b, "\n**Outcome:** `%s`\n", f.Outcome)
	}
	b.WriteString("\n")
}

// writeEvidenceGaps renders missing/expired evidence and how to capture it next run.
func writeEvidenceGaps(b *strings.Builder, gaps []model.EvidenceGap) {
	if len(gaps) == 0 {
		return
	}
	b.WriteString("### Evidence gaps\n\n")
	for _, g := range gaps {
		fmt.Fprintf(b, "- **%s**", emptyDash(g.Missing))
		if strings.TrimSpace(g.WhereItLives) != "" {
			fmt.Fprintf(b, " — lives in: %s", g.WhereItLives)
		}
		if strings.TrimSpace(g.WhyMissing) != "" {
			fmt.Fprintf(b, "; missing because: %s", g.WhyMissing)
		}
		b.WriteString("\n")
		if strings.TrimSpace(g.HowToCapture) != "" {
			fmt.Fprintf(b, "  - Capture next run: `%s`\n", oneLine(g.HowToCapture))
		}
	}
	b.WriteString("\n")
}

// writeKnownUnknowns renders the calibrated known-unknowns holding confidence down.
func writeKnownUnknowns(b *strings.Builder, items []string) {
	if len(items) == 0 {
		return
	}
	b.WriteString("### Known-unknowns\n\n")
	b.WriteString("Unexplained anomalies or disconfirming evidence that hold the confidence down:\n\n")
	for _, it := range items {
		fmt.Fprintf(b, "- %s\n", it)
	}
	b.WriteString("\n")
}

// cell escapes a value for a Markdown table cell.
func cell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func oneLine(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
}
