// Package fixprompt renders "fix.md": a Copilot-ready prompt describing a
// high-confidence regression and the fix direction, plus a machine-readable
// metadata header. The failure-agent only writes it as an artifact; nothing is
// dispatched from Azure DevOps (which cannot obtain a GitHub token). An
// independent, manually-triggered GitHub workflow later pulls the artifact and
// turns fix.md into a draft pull request assigned to the GitHub Copilot coding
// agent.
package fixprompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

// FileName is the artifact the agent writes and the workflow looks for.
const FileName = "fix.md"

// fingerprintShortLen bounds the fingerprint slice used in the title and branch.
const fingerprintShortLen = 12

// maxSnippets bounds how many evidence snippets are embedded in the prompt so it
// stays a focused instruction rather than a log dump.
const maxSnippets = 3

// ShouldEmit reports whether a fix.md should be written for this run. A fix
// prompt is only useful for a non-PR build (PR builds already get the inline PR
// comment) that the LLM analyzed with high confidence as a regression in the
// change under test and for which it proposed a fix direction to ground the
// prompt. This mirrors the gate the draft-PR workflow expects.
func ShouldEmit(rc model.RunContext, inc model.Incident) bool {
	return !rc.IsPR &&
		inc.AnalysisStatus == model.StatusAnalyzed &&
		inc.Category == model.CategoryPRRegression &&
		inc.ConfidenceBand == model.BandHigh &&
		strings.TrimSpace(inc.ProposedFix) != ""
}

// Title is the draft-PR title derived from the incident. It is also emitted in
// the metadata header so the workflow does not have to reconstruct it.
func Title(inc model.Incident) string {
	pipeline := strings.TrimSpace(inc.PipelineName)
	if pipeline == "" {
		pipeline = "pipeline"
	}
	return fmt.Sprintf("[FAA] Draft fix: %s regression (%s)", pipeline, shortFingerprint(inc.Fingerprint))
}

// WriteFile renders the prompt and writes it as fix.md into dir, returning the
// path written.
func WriteFile(dir string, inc model.Incident) (string, error) {
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte(Render(inc)), 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", FileName, err)
	}
	return path, nil
}

// Render produces the fix.md body: a metadata header the workflow parses, then a
// grounded instruction for the coding agent to author the smallest correct fix
// and keep the pull request a draft.
func Render(inc model.Incident) string {
	var b strings.Builder

	writeMetadata(&b, inc)

	fmt.Fprintf(&b, "# %s\n\n", Title(inc))
	b.WriteString("A non-PR Azure DevOps pipeline build failed and the Failure Analysis Agent " +
		"classified it as a **high-confidence code regression** in the change under test. " +
		"Author the smallest correct fix and open it as a **draft** pull request for human " +
		"review — do not mark it ready for review.\n\n")

	b.WriteString("## Failure context\n\n")
	b.WriteString("| Field | Value |\n|---|---|\n")
	writeRow(&b, "Pipeline", inc.PipelineName)
	writeRow(&b, "Stage / Job", strings.Join(nonEmpty(inc.Stage, inc.Job), " / "))
	writeRow(&b, "Failing commit", inc.Commit)
	writeRow(&b, "Fingerprint", inc.Fingerprint)
	writeRow(&b, "Category", fmt.Sprintf("%s (%s, %.2f)", inc.Category, inc.ConfidenceBand, inc.Confidence))
	b.WriteString("\n")

	b.WriteString("## Root cause\n\n")
	fmt.Fprintf(&b, "%s\n\n", emptyDash(inc.RootCauseSummary))

	if strings.TrimSpace(inc.NodeAssessment) != "" {
		b.WriteString("## Node / nodepool health\n\n")
		fmt.Fprintf(&b, "%s\n\n", inc.NodeAssessment)
	}

	b.WriteString("## Proposed fix direction\n\n")
	fmt.Fprintf(&b, "%s\n\n", emptyDash(inc.ProposedFix))

	if strings.TrimSpace(inc.RecommendedAction) != "" {
		b.WriteString("## Recommended action\n\n")
		fmt.Fprintf(&b, "%s\n\n", inc.RecommendedAction)
	}

	if len(inc.TopEvidence) > 0 {
		b.WriteString("## Top evidence\n\n")
		for _, e := range inc.TopEvidence {
			if strings.TrimSpace(e) == "" {
				continue
			}
			fmt.Fprintf(&b, "- `%s`\n", strings.ReplaceAll(e, "`", "'"))
		}
		b.WriteString("\n")
	}

	writeSnippets(&b, inc.ErrorSnippets)

	b.WriteString("## Instructions for the coding agent\n\n")
	b.WriteString("- Implement the smallest change that resolves the root cause above; do not refactor unrelated code.\n")
	b.WriteString("- Keep the pull request a **draft**. Do not mark it ready for review.\n")
	fmt.Fprintf(&b, "- Reference fingerprint `%s` in the pull request description.\n", inc.Fingerprint)
	b.WriteString("- If the proposed direction is wrong, say why in the PR and implement the correct fix instead.\n")

	return b.String()
}

// writeMetadata emits the machine-readable header the workflow parses. It is an
// HTML comment so it stays invisible in rendered markdown. Values are forced to
// a single line so the header remains line-oriented for shell parsing.
func writeMetadata(b *strings.Builder, inc model.Incident) {
	b.WriteString("<!-- faa-fix:v1\n")
	writeMetaField(b, "fingerprint", inc.Fingerprint)
	writeMetaField(b, "title", Title(inc))
	writeMetaField(b, "category", string(inc.Category))
	writeMetaField(b, "confidence", fmt.Sprintf("%.2f", inc.Confidence))
	writeMetaField(b, "confidenceBand", string(inc.ConfidenceBand))
	writeMetaField(b, "pipeline", inc.PipelineName)
	writeMetaField(b, "buildId", inc.BuildID)
	writeMetaField(b, "commit", inc.Commit)
	writeMetaField(b, "owner", inc.RecommendedOwner)
	b.WriteString("-->\n\n")
}

func writeMetaField(b *strings.Builder, key, value string) {
	fmt.Fprintf(b, "%s: %s\n", key, oneLine(value))
}

func writeSnippets(b *strings.Builder, snippets []model.ErrorSnippet) {
	if len(snippets) == 0 {
		return
	}
	b.WriteString("## Evidence snippets\n\n")
	for i, sn := range snippets {
		if i >= maxSnippets {
			break
		}
		fmt.Fprintf(b, "**%s:%d**\n\n", sn.File, sn.Line)
		b.WriteString("```text\n")
		b.WriteString(sn.Snippet)
		b.WriteString("\n```\n\n")
	}
}

func writeRow(b *strings.Builder, key, val string) {
	fmt.Fprintf(b, "| %s | %s |\n", key, emptyDash(val))
}

func shortFingerprint(fp string) string {
	if len(fp) > fingerprintShortLen {
		return fp[:fingerprintShortLen]
	}
	return fp
}

// oneLine collapses a value to a single line so it is safe inside the
// line-oriented metadata header.
func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
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
