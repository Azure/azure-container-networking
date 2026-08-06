package ask

import (
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/classify"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

// systemPrompt frames ask mode as grounded question-answering, not re-triage.
func systemPrompt() string {
	return "You are an expert Azure Container Networking (ACN) CI failure analyst " +
		"answering a follow-up question from an engineer about a specific failed pipeline run. " +
		"You are given the engineer's question, the prior automated FAA analysis of that run " +
		"(its verdict and human-readable report), and excerpts from the run's collected evidence bundle. " +
		"Answer the question directly and concisely in Markdown, grounded strictly in the provided " +
		"prior analysis and evidence. Cite the specific artifacts or evidence lines you rely on by name. " +
		"When the provided material is insufficient to answer confidently, say so explicitly and state " +
		"what additional evidence would be needed — do not speculate or fabricate. " +
		"This is a grounded question-answer task: do NOT re-triage the failure, and do NOT output a new " +
		"classification, category, confidence score, or JSON — respond only with the prose answer."
}

// userPrompt assembles the question, prior verdict, and evidence excerpts.
func userPrompt(question string, prior PriorAnalysis, ev model.Evidence) string {
	var b strings.Builder

	b.WriteString("## Question\n")
	b.WriteString(strings.TrimSpace(question))
	b.WriteString("\n\n")

	writePriorAnalysis(&b, prior)

	if len(ev.TopErrorLines) > 0 {
		b.WriteString("## Top error lines\n")
		for _, l := range ev.TopErrorLines {
			fmt.Fprintf(&b, "- %s\n", l)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Evidence excerpts\n")
	if excerpts := classify.EvidenceExcerpts(ev); strings.TrimSpace(excerpts) != "" {
		b.WriteString(excerpts)
	} else {
		b.WriteString("(no evidence excerpts available)\n")
	}

	return b.String()
}

// writePriorAnalysis renders the prior FAA verdict and report into the prompt,
// noting explicitly when none is available.
func writePriorAnalysis(b *strings.Builder, prior PriorAnalysis) {
	if prior.Incident == nil && strings.TrimSpace(prior.Report) == "" {
		b.WriteString("## Prior FAA analysis\n(none available for this build)\n\n")
		return
	}

	b.WriteString("## Prior FAA analysis (verdict)\n")
	if inc := prior.Incident; inc != nil {
		if inc.Category != "" {
			fmt.Fprintf(b, "- Category: %s\n", inc.Category)
		}
		fmt.Fprintf(b, "- Confidence: %.2f (%s)\n", inc.Confidence, inc.ConfidenceBand)
		if inc.RootCauseSummary != "" {
			fmt.Fprintf(b, "- Root cause: %s\n", oneLine(inc.RootCauseSummary))
		}
		if inc.ProposedFix != "" {
			fmt.Fprintf(b, "- Proposed fix: %s\n", oneLine(inc.ProposedFix))
		}
		if inc.NodeAssessment != "" {
			fmt.Fprintf(b, "- Node assessment: %s\n", oneLine(inc.NodeAssessment))
		}
		b.WriteString("\n")
	}
	if r := strings.TrimSpace(prior.Report); r != "" {
		b.WriteString("### Prior report.md\n")
		b.WriteString(r)
		b.WriteString("\n\n")
	}
}

func oneLine(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
}
