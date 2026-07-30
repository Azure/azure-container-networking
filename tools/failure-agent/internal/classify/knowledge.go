package classify

import (
	"fmt"
	"strings"
)

// PriorContext is knowledge about earlier occurrences of a failure, retrieved
// from the knowledge store and injected to ground the model. Resolved entries
// carry validated fixes; Unresolved entries are in-flight and labeled as such so
// the model treats them as context, not as confirmed answers.
type PriorContext struct {
	Resolved   []PriorIncident
	Unresolved []PriorIncident
}

// PriorIncident is a single earlier incident summarized for the prompt.
type PriorIncident struct {
	Fingerprint string
	Category    string
	Summary     string
	Fix         string
	Status      string
}

func (p PriorContext) isEmpty() bool {
	return len(p.Resolved) == 0 && len(p.Unresolved) == 0
}

// writePriorContext renders prior knowledge into the user prompt.
func writePriorContext(b *strings.Builder, p PriorContext) {
	if p.isEmpty() {
		return
	}
	if len(p.Resolved) > 0 {
		b.WriteString("## Prior validated resolutions for similar failures\n")
		b.WriteString("Prefer these when they clearly match the current evidence.\n")
		for _, pi := range p.Resolved {
			fmt.Fprintf(b, "- [%s] %s\n", pi.Category, oneLinePrompt(pi.Summary))
			if strings.TrimSpace(pi.Fix) != "" {
				fmt.Fprintf(b, "  - Validated fix: %s\n", oneLinePrompt(pi.Fix))
			}
		}
		b.WriteString("\n")
	}
	if len(p.Unresolved) > 0 {
		b.WriteString("## Related in-flight incidents (NOT yet validated — context only)\n")
		for _, pi := range p.Unresolved {
			fmt.Fprintf(b, "- [%s, status=%s] %s\n", pi.Category, pi.Status, oneLinePrompt(pi.Summary))
		}
		b.WriteString("\n")
	}
}

func oneLinePrompt(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
}
