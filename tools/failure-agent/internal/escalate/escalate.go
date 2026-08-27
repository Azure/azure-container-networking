// Package escalate is the failure-agent's escalation gate: a second, narrow LLM
// call that runs after the incident has been assembled and decides whether the
// failure warrants a GitHub issue asking for a code fix.
//
// It exists because the earlier design gated this artifact on the classifier's
// own category and confidence, which answers the wrong question. "How sure is the
// agent that this is a PR regression" is not the same as "would an engineer open
// a bug and start editing code", and the two diverge in both directions: a
// low-confidence finding that cites a specific line is actionable, while a
// high-confidence quota exhaustion is not. The gate is given the finished report
// and asked the second question directly.
//
// The agent only ever writes issue.md as an artifact. Azure DevOps cannot obtain
// a GitHub token, so nothing is dispatched from the pipeline; an independent
// GitHub workflow pulls the artifact and raises the issue.
package escalate

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

// FileName is the artifact the agent writes and the GitHub workflow looks for.
const FileName = "issue.md"

// fingerprintShortLen bounds the fingerprint slice shown in the issue title.
const fingerprintShortLen = 12

// maxReportChars bounds how much of the rendered report is sent to the gate.
// The report embeds evidence snippets, so a pathological bundle could otherwise
// dominate the prompt.
const maxReportChars = 20000

// maxChangedFiles bounds the changed-file list in the prompt. The gate needs to
// know whether the failing mechanism is reachable from the change under test,
// which the head of the list answers; it does not need an exhaustive inventory.
const maxChangedFiles = 60

// allowedLabels is the closed set of labels the gate may request. issue.md is
// produced by a model inside Azure DevOps and consumed by a GitHub workflow that
// passes labels to `gh issue create`, so the set is constrained here at the model
// boundary rather than trusted downstream. The workflow re-checks it.
var allowedLabels = []string{
	"bug",
	"regression",
	"cni",
	"cns",
	"npm",
	"ipam",
	"windows",
	"linux",
	"test-infra",
}

// AllowedLabels returns the labels the gate is permitted to request.
func AllowedLabels() []string {
	out := make([]string, len(allowedLabels))
	copy(out, allowedLabels)
	return out
}

// Completer is the minimal LLM capability the gate needs. It matches the
// per-run classifier's ChatCompleter so the same Azure OpenAI client backs both
// paths; declaring it here (consumer-side) keeps escalate decoupled from any
// specific SDK.
type Completer interface {
	Complete(ctx context.Context, system, user string, schema *classify.Schema) (string, error)
}

// ShouldConsider reports whether the gate is worth consulting at all, and when
// it is not, why. PR builds already get an inline PR comment and their fix
// belongs in the pull request under review; an incident whose analysis failed has
// no conclusion to escalate. Everything else — category, confidence, signature
// matches — is deliberately left to the model. The reason is recorded on the
// incident so a skipped gate stays distinguishable from a declined escalation.
func ShouldConsider(rc model.RunContext, inc model.Incident) (bool, string) {
	if rc.IsPR {
		return false, "Pull-request build: the analysis is posted to the pull request and the fix belongs in the change under review."
	}
	if inc.AnalysisStatus != model.StatusAnalyzed {
		return false, "Analysis did not produce a classification, so there is no conclusion to escalate."
	}
	return true, ""
}

// Gate decides whether an incident warrants a GitHub issue.
type Gate struct {
	client Completer
}

// NewGate returns a gate backed by client.
func NewGate(client Completer) *Gate {
	return &Gate{client: client}
}

// Decide asks the model to rule on the incident. A malformed or out-of-contract
// response is an error so the caller can record that the gate failed rather than
// silently reporting "no issue needed".
func (g *Gate) Decide(ctx context.Context, rc model.RunContext, inc model.Incident, reportMD string) (model.Escalation, error) {
	raw, err := g.client.Complete(ctx, escalationPolicy, userPrompt(rc, inc, reportMD), escalationSchema())
	if err != nil {
		return model.Escalation{}, fmt.Errorf("escalation completion: %w", err)
	}

	var res gateResult
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return model.Escalation{}, fmt.Errorf("parsing escalation response: %w", err)
	}
	return res.toEscalation()
}

type gateResult struct {
	Needed         bool     `json:"needed"`
	Reason         string   `json:"reason"`
	Title          string   `json:"title"`
	Labels         []string `json:"labels"`
	FixDirection   string   `json:"fixDirection"`
	SuggestedFiles []string `json:"suggestedFiles"`
	Blockers       []string `json:"blockers"`
}

func (r gateResult) toEscalation() (model.Escalation, error) {
	// Reason is required on both outcomes: a decision not to escalate is
	// reviewed as often as a decision to, and an unexplained "no" is
	// indistinguishable from a bug in this gate.
	if strings.TrimSpace(r.Reason) == "" {
		return model.Escalation{}, errors.New("escalation response has no reason")
	}
	if r.Needed && strings.TrimSpace(r.Title) == "" {
		return model.Escalation{}, errors.New("escalation response needs an issue but has no title")
	}

	e := model.Escalation{
		Needed: r.Needed,
		Reason: strings.TrimSpace(r.Reason),
		Source: model.EscalationLLM,
	}
	if !r.Needed {
		return e, nil
	}

	e.Title = strings.TrimSpace(r.Title)
	e.Labels = filterLabels(r.Labels)
	e.FixDirection = strings.TrimSpace(r.FixDirection)
	e.SuggestedFiles = trimAll(r.SuggestedFiles)
	e.Blockers = trimAll(r.Blockers)
	return e, nil
}

// filterLabels keeps only allowed labels, deduplicated and in the model's order.
func filterLabels(labels []string) []string {
	seen := make(map[string]bool, len(labels))
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		l = strings.ToLower(strings.TrimSpace(l))
		if l == "" || seen[l] || !isAllowedLabel(l) {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isAllowedLabel(l string) bool {
	for _, a := range allowedLabels {
		if a == l {
			return true
		}
	}
	return false
}

func trimAll(vals []string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func escalationSchema() *classify.Schema {
	def := `{
  "type": "object",
  "additionalProperties": false,
  "required": ["needed", "reason", "title", "labels", "fixDirection", "suggestedFiles", "blockers"],
  "properties": {
    "needed": {"type": "boolean"},
    "reason": {"type": "string"},
    "title": {"type": "string"},
    "labels": {"type": "array", "items": {"type": "string", "enum": ["bug", "regression", "cni", "cns", "npm", "ipam", "windows", "linux", "test-infra"]}},
    "fixDirection": {"type": "string"},
    "suggestedFiles": {"type": "array", "items": {"type": "string"}},
    "blockers": {"type": "array", "items": {"type": "string"}}
  }
}`
	return &classify.Schema{Name: "escalation_decision", Definition: json.RawMessage(def)}
}

// escalationPolicy is the gate's decision contract: escalate on a mechanism that
// is fixable by editing this repository, decline on environmental, credential,
// or unowned causes, and treat the classifier's category and confidence as
// evidence rather than as the gate.
//
//go:embed escalation-playbook.md
var escalationPolicy string

// userPrompt gives the gate the same view a human reviewer gets — the rendered
// report — plus the change under test, which the report does not carry and which
// is the clearest signal for whether the failing mechanism is reachable from code
// in this repository.
func userPrompt(rc model.RunContext, inc model.Incident, reportMD string) string {
	var b strings.Builder

	b.WriteString("## Incident summary\n")
	fmt.Fprintf(&b, "Fingerprint: %s\n", inc.Fingerprint)
	fmt.Fprintf(&b, "Pipeline: %s\n", inc.PipelineName)
	fmt.Fprintf(&b, "Stage/Job: %s / %s\n", inc.Stage, inc.Job)
	fmt.Fprintf(&b, "Scenario: type=%s os=%s cni=%s region=%s\n", inc.ClusterType, inc.OS, inc.CNI, inc.Region)
	fmt.Fprintf(&b, "Category: %s (confidence %.2f, band %s)\n", inc.Category, inc.Confidence, inc.ConfidenceBand)
	b.WriteString("Category and confidence are context, not the decision. Rule on the mechanism and who owns it.\n\n")

	writeChangeUnderTest(&b, rc)
	writeRootCauseSources(&b, inc.RootCauseSources)

	b.WriteString("## Analysis report\n")
	b.WriteString("This is the report.md a human would read.\n\n")
	b.WriteString(truncate(reportMD, maxReportChars))
	b.WriteString("\n")

	return b.String()
}

func writeChangeUnderTest(b *strings.Builder, rc model.RunContext) {
	if len(rc.ChangedFiles) == 0 {
		b.WriteString("## Change under test\n")
		b.WriteString("No change context was captured for this build. Do not infer from its absence that the failure is environmental.\n\n")
		return
	}

	b.WriteString("## Change under test\n")
	b.WriteString("Files edited by the change this build was testing:\n")
	for i, f := range rc.ChangedFiles {
		if i >= maxChangedFiles {
			fmt.Fprintf(b, "- ...and %d more\n", len(rc.ChangedFiles)-maxChangedFiles)
			break
		}
		fmt.Fprintf(b, "- %s\n", f)
	}
	b.WriteString("\n")
}

func writeRootCauseSources(b *strings.Builder, refs []model.RootCauseRef) {
	if len(refs) == 0 {
		b.WriteString("## Cited root-cause locations\n")
		b.WriteString("The analysis pinned the cause to no specific artifact line. Weigh that when judging whether an implementer would have somewhere to start.\n\n")
		return
	}

	b.WriteString("## Cited root-cause locations\n")
	for _, r := range refs {
		fmt.Fprintf(b, "- %s:%d", r.File, r.Line)
		if r.Explanation != "" {
			fmt.Fprintf(b, " — %s", oneLine(r.Explanation))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n\n_[report truncated]_\n"
}
