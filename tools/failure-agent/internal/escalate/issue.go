package escalate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

// maxSnippets bounds how many evidence snippets are embedded in the issue so it
// stays a readable bug report rather than a log dump. The full evidence remains
// in the pipeline artifact.
const maxSnippets = 3

// FingerprintMarker is the hidden marker the GitHub workflow greps for when
// deciding whether an issue already exists for this failure. It is matched by
// scanning issue bodies directly rather than through GitHub's search index, so
// an HTML comment is safe and keeps it out of the rendered issue.
func FingerprintMarker(fingerprint string) string {
	return "<!-- acn-faa-fingerprint:" + fingerprint + " -->"
}

// Title is the GitHub issue title. The model supplies the descriptive half; the
// prefix makes FAA-raised issues filterable and the short fingerprint keeps two
// visually similar failures distinguishable in a list.
func Title(inc model.Incident, e model.Escalation) string {
	title := oneLine(e.Title)
	if title == "" {
		title = "Pipeline failure needs a code fix"
	}
	return fmt.Sprintf("[FAA] %s (%s)", title, shortFingerprint(inc.Fingerprint))
}

// WriteFile renders the issue and writes it as issue.md into dir, returning the
// path written.
func WriteFile(dir string, inc model.Incident, e model.Escalation) (string, error) {
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte(Render(inc, e)), 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", FileName, err)
	}
	return path, nil
}

// Render produces the issue.md body: a metadata header the GitHub workflow
// parses, then the bug report itself, grounded in the analysis the gate saw.
func Render(inc model.Incident, e model.Escalation) string {
	var b strings.Builder

	writeMetadata(&b, inc, e)
	b.WriteString(FingerprintMarker(inc.Fingerprint) + "\n\n")

	b.WriteString("The ACN Failure Analysis Agent analyzed a failed Azure DevOps pipeline run and " +
		"judged that it needs a code change in this repository. The analysis is a **hypothesis**: " +
		"confirm the cited evidence before implementing anything.\n\n")

	b.WriteString("## Why this was raised\n\n")
	fmt.Fprintf(&b, "%s\n\n", emptyDash(e.Reason))

	b.WriteString("## Failure context\n\n")
	b.WriteString("| Field | Value |\n|---|---|\n")
	writeRow(&b, "Pipeline", inc.PipelineName)
	writeRow(&b, "Stage / Job", strings.Join(nonEmpty(inc.Stage, inc.Job), " / "))
	writeRow(&b, "Scenario", strings.Join(nonEmpty(inc.ClusterType, inc.OS, inc.CNI), " / "))
	writeRow(&b, "Region", inc.Region)
	writeRow(&b, "Build", strings.Join(nonEmpty(inc.BuildNumber, inc.BuildID), " / "))
	writeRow(&b, "Failing commit", inc.Commit)
	writeRow(&b, "Fingerprint", inc.Fingerprint)
	writeRow(&b, "FAA category", fmt.Sprintf("%s (%s, %.2f)", inc.Category, inc.ConfidenceBand, inc.Confidence))
	b.WriteString("\n")

	b.WriteString("## Root cause (as analyzed)\n\n")
	fmt.Fprintf(&b, "%s\n\n", emptyDash(inc.RootCauseSummary))
	if strings.TrimSpace(inc.FailingUnit) != "" {
		fmt.Fprintf(&b, "**Failing unit:** %s\n\n", inc.FailingUnit)
	}
	if strings.TrimSpace(inc.NodeAssessment) != "" {
		b.WriteString("**Node / nodepool health:** " + oneLine(inc.NodeAssessment) + "\n\n")
	}

	writeCitedSources(&b, inc.RootCauseSources)
	writeFixDirection(&b, e)
	writeList(&b, "## Files to look at first", "_The analysis did not identify specific files._", backticked(e.SuggestedFiles))
	writeList(&b, "## Open questions before fixing", "", e.Blockers)
	writeList(&b, "## Unexplained signals", "", inc.KnownUnknowns)
	writeList(&b, "## Top evidence", "", backticked(inc.TopEvidence))
	writeSnippets(&b, inc.ErrorSnippets)

	b.WriteString("## For the implementing agent\n\n")
	b.WriteString("Follow the `acn-faa-fix-from-issue` skill in `.github/skills/`. In short:\n\n")
	b.WriteString("- Verify the cited evidence before changing anything. If the diagnosis is wrong, say so here and fix the real cause instead.\n")
	b.WriteString("- Make the smallest change that resolves the root cause. Do not refactor unrelated code.\n")
	b.WriteString("- Follow the repository conventions in `agents.md` (root-to-leaf, closest wins) and the relevant `acn-go-*` skills.\n")
	b.WriteString("- Open the fix as a **draft** pull request and leave it for human review.\n")
	fmt.Fprintf(&b, "- Reference fingerprint `%s` and link back to this issue in the pull request description.\n\n", inc.Fingerprint)

	b.WriteString("_Raised automatically by the ACN Failure Analysis Agent. " +
		"The full evidence bundle is attached to the Azure DevOps build._\n")

	return b.String()
}

// writeMetadata emits the machine-readable header the GitHub workflow parses. It
// is an HTML comment so it stays invisible in the rendered issue. Values are
// forced to a single line so the header remains line-oriented for shell parsing.
func writeMetadata(b *strings.Builder, inc model.Incident, e model.Escalation) {
	b.WriteString("<!-- faa-issue:v1\n")
	writeMetaField(b, "fingerprint", inc.Fingerprint)
	writeMetaField(b, "title", Title(inc, e))
	writeMetaField(b, "labels", strings.Join(e.Labels, ","))
	writeMetaField(b, "category", string(inc.Category))
	writeMetaField(b, "confidence", fmt.Sprintf("%.2f", inc.Confidence))
	writeMetaField(b, "pipeline", inc.PipelineName)
	writeMetaField(b, "buildId", inc.BuildID)
	writeMetaField(b, "commit", inc.Commit)
	writeMetaField(b, "owner", inc.RecommendedOwner)
	b.WriteString("-->\n\n")
}

func writeMetaField(b *strings.Builder, key, value string) {
	fmt.Fprintf(b, "%s: %s\n", key, oneLine(value))
}

func writeCitedSources(b *strings.Builder, refs []model.RootCauseRef) {
	if len(refs) == 0 {
		return
	}
	b.WriteString("## Cited evidence\n\n")
	b.WriteString("Locations the analysis pinned the cause to. Confirm these first.\n\n")
	for _, r := range refs {
		fmt.Fprintf(b, "**`%s:%d`**", r.File, r.Line)
		if r.EndLine > r.Line {
			fmt.Fprintf(b, " (through line %d)", r.EndLine)
		}
		b.WriteString("\n\n")
		if strings.TrimSpace(r.Explanation) != "" {
			fmt.Fprintf(b, "%s\n\n", r.Explanation)
		}
		if strings.TrimSpace(r.Snippet) != "" {
			b.WriteString("```text\n")
			b.WriteString(r.Snippet)
			b.WriteString("\n```\n\n")
		}
	}
}

func writeFixDirection(b *strings.Builder, e model.Escalation) {
	b.WriteString("## Fix direction\n\n")
	if strings.TrimSpace(e.FixDirection) != "" {
		fmt.Fprintf(b, "%s\n\n", e.FixDirection)
		return
	}
	b.WriteString("_The analysis did not propose a specific direction. Start from the cited evidence above._\n\n")
}

// writeList renders a bulleted section, skipping it entirely when items is empty
// unless an explicit empty-state note is supplied.
func writeList(b *strings.Builder, heading, emptyNote string, items []string) {
	if len(items) == 0 {
		if emptyNote == "" {
			return
		}
		fmt.Fprintf(b, "%s\n\n%s\n\n", heading, emptyNote)
		return
	}
	fmt.Fprintf(b, "%s\n\n", heading)
	for _, it := range items {
		fmt.Fprintf(b, "- %s\n", it)
	}
	b.WriteString("\n")
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

// backticked wraps each item in code formatting, escaping any backticks it
// already contains so a log line cannot break out of the span.
func backticked(items []string) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		if strings.TrimSpace(it) == "" {
			continue
		}
		out = append(out, "`"+strings.ReplaceAll(it, "`", "'")+"`")
	}
	return out
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
