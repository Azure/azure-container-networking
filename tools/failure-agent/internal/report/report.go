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
		RecommendedOwner:     c.RecommendedOwner,
		NodeAssessment:       c.NodeAssessment,
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

	b.WriteString("### Likely root cause\n\n")
	fmt.Fprintf(&b, "%s\n\n", emptyDash(inc.RootCauseSummary))

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

	if inc.ProposedFix != "" {
		b.WriteString("### Proposed fix\n\n")
		fmt.Fprintf(&b, "%s\n\n", inc.ProposedFix)
	}

	b.WriteString("### Recommended next action\n\n")
	fmt.Fprintf(&b, "%s\n\n", emptyDash(inc.RecommendedAction))
	if inc.RecommendedOwner != "" {
		fmt.Fprintf(&b, "**Suggested owner:** %s\n\n", inc.RecommendedOwner)
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
