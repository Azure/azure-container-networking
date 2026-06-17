// Package classify produces the root-cause assessment. This file implements the
// deterministic path used by --dry-run (and as a fallback): it derives the
// classification from signature matches alone, with no LLM call. The LLM-backed
// classifier is added in a later phase behind a consumer-side interface.
package classify

import (
	"fmt"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

// topEvidenceCount is how many error lines are surfaced as evidence.
const topEvidenceCount = 5

// Deterministic derives a classification from signature matches only. When a
// signature matches it adopts that category/confidence/owner; otherwise the
// failure is marked as needing human triage.
func Deterministic(_ model.RunContext, ev model.Evidence, matches []model.SignatureMatch) model.Classification {
	c := model.Classification{
		TopEvidence: topEvidence(ev),
		Source:      "deterministic",
	}

	if len(matches) > 0 {
		top := matches[0]
		c.Category = top.Category
		c.Confidence = top.Confidence
		c.RecommendedOwner = top.Owner
		c.RootCauseSummary = summaryFromSignature(top, ev)
		return c
	}

	c.Category = model.CategoryUnknownNeedsHuman
	c.Confidence = 0
	c.RootCauseSummary = summaryFromEvidence(ev)
	return c
}

func summaryFromSignature(m model.SignatureMatch, ev model.Evidence) string {
	if len(ev.TopErrorLines) > 0 {
		return fmt.Sprintf("%s First observed error: %q", m.Description, ev.TopErrorLines[0])
	}
	return m.Description
}

func summaryFromEvidence(ev model.Evidence) string {
	if len(ev.TopErrorLines) > 0 {
		return fmt.Sprintf("Unclassified failure; no known signature matched. First observed error: %q", ev.TopErrorLines[0])
	}
	return "Unclassified failure; no error lines were extracted from the evidence bundle."
}

func topEvidence(ev model.Evidence) []string {
	n := topEvidenceCount
	if len(ev.TopErrorLines) < n {
		n = len(ev.TopErrorLines)
	}
	out := make([]string, n)
	copy(out, ev.TopErrorLines[:n])
	return out
}
