// This file implements the LLM-backed classification path. The classifier
// builds a grounded prompt, asks a ChatCompleter for a schema-constrained JSON
// answer, and validates it. The concrete Azure OpenAI ChatCompleter lives in
// aoai.go; tests use a fake.
package classify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

// maxExcerptChars caps how much of each evidence excerpt is sent to the model.
const maxExcerptChars = 1500

// maxTotalExcerptChars caps the combined excerpt payload across files.
const maxTotalExcerptChars = 6000

// Schema describes the JSON shape the model must return.
type Schema struct {
	Name       string
	Definition json.RawMessage
}

// ChatCompleter is the minimal LLM capability the classifier needs. Keeping it
// here (consumer-side) decouples classification from any specific SDK.
type ChatCompleter interface {
	Complete(ctx context.Context, system, user string, schema *Schema) (string, error)
}

// EvidenceProvider satisfies the model's allow-listed requests for additional
// evidence during the bounded deepen pass. Ids are of the form "source:<path>"
// (HEAD content of a changed file) or "log:<name>" (a collected evidence file).
// A provider returns ok=false for anything outside its allow-list, so the model
// can never pull arbitrary files.
type EvidenceProvider interface {
	Provide(ctx context.Context, id string) (content string, ok bool)
}

// LLMClassifier produces a Classification via a ChatCompleter, grounded by the
// fingerprint, signature matches, scenario, change under test, and trimmed
// evidence. When an EvidenceProvider is wired and the first answer is weak, it
// makes exactly one additional, context-preserving "deepen" call.
type LLMClassifier struct {
	client   ChatCompleter
	provider EvidenceProvider
}

// NewLLMClassifier returns a classifier backed by client.
func NewLLMClassifier(client ChatCompleter) *LLMClassifier {
	return &LLMClassifier{client: client}
}

// WithEvidence enables the bounded deepen pass by wiring an EvidenceProvider.
// Without one, Classify always resolves in a single call.
func (c *LLMClassifier) WithEvidence(p EvidenceProvider) *LLMClassifier {
	c.provider = p
	return c
}

// deepenConfidenceThreshold is the confidence below which a first answer is
// considered weak enough to warrant the single deepen pass.
const deepenConfidenceThreshold = 0.75

// maxDeepenRequests caps how many evidence ids the deepen pass will satisfy, so
// one extra call stays bounded in size as well as count.
const maxDeepenRequests = 4

// Classify asks the model to categorize the failure and validates the result.
// It runs a single grounded pass; if that answer is low-confidence or its fix is
// generic and an EvidenceProvider can satisfy the model's evidence requests, it
// makes exactly ONE additional call that re-sends the full original context plus
// the requested evidence. It never loops beyond that one deepen call.
func (c *LLMClassifier) Classify(ctx context.Context, rc model.RunContext, ev model.Evidence, fp model.Fingerprint, matches []model.SignatureMatch, prior PriorContext) (model.Classification, error) {
	res, err := c.run(ctx, rc, ev, fp, matches, prior, nil)
	if err != nil {
		return model.Classification{}, err
	}

	if c.provider != nil && needsDeepen(res) {
		if extra := c.satisfyRequests(ctx, res.EvidenceRequest); len(extra) > 0 {
			if deepened, derr := c.run(ctx, rc, ev, fp, matches, prior, extra); derr == nil {
				res = deepened
			}
		}
	}
	return res.toClassification()
}

// run builds the grounded prompt (optionally augmented with deepen evidence),
// calls the model, and parses the schema-constrained JSON answer.
func (c *LLMClassifier) run(ctx context.Context, rc model.RunContext, ev model.Evidence, fp model.Fingerprint, matches []model.SignatureMatch, prior PriorContext, extra map[string]string) (llmResult, error) {
	raw, err := c.client.Complete(ctx, systemPrompt(), userPrompt(rc, ev, fp, matches, prior, extra), classificationSchema())
	if err != nil {
		return llmResult{}, fmt.Errorf("llm completion: %w", err)
	}
	var res llmResult
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return llmResult{}, fmt.Errorf("parsing llm response: %w", err)
	}
	return res, nil
}

// satisfyRequests resolves the model's allow-listed evidence ids through the
// provider, keeping only those the provider actually serves, capped for size.
func (c *LLMClassifier) satisfyRequests(ctx context.Context, ids []string) map[string]string {
	extra := map[string]string{}
	for _, id := range ids {
		if len(extra) >= maxDeepenRequests {
			break
		}
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if content, ok := c.provider.Provide(ctx, id); ok && strings.TrimSpace(content) != "" {
			extra[id] = content
		}
	}
	return extra
}

// needsDeepen reports whether a first answer is weak enough to justify the one
// deepen call: low confidence, or a fix too generic to act on.
func needsDeepen(res llmResult) bool {
	return res.Confidence < deepenConfidenceThreshold || isGenericFix(res.ProposedFix)
}

// fixCommandRE matches concrete command verbs in a proposed fix.
var fixCommandRE = regexp.MustCompile(`(?i)\b(kubectl|make|go test|go build|go run|az |git |helm|docker|curl)\b`)

// isGenericFix reports whether a proposed fix lacks the specificity an engineer
// needs to act — no file reference and no command, or simply too short.
func isGenericFix(fix string) bool {
	f := strings.TrimSpace(fix)
	if len(f) < 80 {
		return true
	}
	hasFile := strings.Contains(f, "/") || strings.Contains(f, ".go") ||
		strings.Contains(f, ".yaml") || strings.Contains(f, ".yml")
	hasCommand := strings.Contains(f, "`") || fixCommandRE.MatchString(f)
	return !hasFile && !hasCommand
}

type llmResult struct {
	Category         string   `json:"category"`
	Confidence       float64  `json:"confidence"`
	RootCauseSummary string   `json:"rootCauseSummary"`
	TopEvidence      []string `json:"topEvidence"`
	RecommendedOwner string   `json:"recommendedOwner"`
	ProposedFix      string   `json:"proposedFix"`
	NodeAssessment   string   `json:"nodeAssessment"`
	// EvidenceRequest names allow-listed evidence the model would need to raise
	// its confidence, of the form "source:<changed-file>" or "log:<name>". It is
	// honored at most once, in the bounded deepen pass.
	EvidenceRequest []string `json:"evidenceRequest"`
}

func (r llmResult) toClassification() (model.Classification, error) {
	cat := model.FailureCategory(r.Category)
	if !validCategory(cat) {
		return model.Classification{}, fmt.Errorf("invalid category %q from llm", r.Category)
	}
	if r.Confidence < 0 || r.Confidence > 1 {
		return model.Classification{}, fmt.Errorf("confidence %v out of range from llm", r.Confidence)
	}
	if strings.TrimSpace(r.RootCauseSummary) == "" {
		return model.Classification{}, errors.New("llm returned empty rootCauseSummary")
	}
	return model.Classification{
		Category:         cat,
		Confidence:       r.Confidence,
		RootCauseSummary: r.RootCauseSummary,
		TopEvidence:      r.TopEvidence,
		RecommendedOwner: r.RecommendedOwner,
		ProposedFix:      r.ProposedFix,
		NodeAssessment:   r.NodeAssessment,
		Source:           "llm",
	}, nil
}

func validCategory(c model.FailureCategory) bool {
	switch c {
	case model.CategoryPRRegression,
		model.CategoryClusterBringupFailure,
		model.CategoryPipelineInfraConfig,
		model.CategoryKnownFlake,
		model.CategoryUnknownNeedsHuman:
		return true
	default:
		return false
	}
}

func classificationSchema() *Schema {
	def := `{
  "type": "object",
  "additionalProperties": false,
  "required": ["category", "confidence", "rootCauseSummary", "topEvidence", "recommendedOwner", "proposedFix", "nodeAssessment", "evidenceRequest"],
  "properties": {
    "category": {"type": "string", "enum": ["pr_regression", "cluster_bringup_failure", "pipeline_infra_config", "known_flake", "unknown_needs_human"]},
    "confidence": {"type": "number", "minimum": 0, "maximum": 1},
    "rootCauseSummary": {"type": "string"},
    "topEvidence": {"type": "array", "items": {"type": "string"}},
    "recommendedOwner": {"type": "string"},
    "proposedFix": {"type": "string"},
    "nodeAssessment": {"type": "string"},
    "evidenceRequest": {"type": "array", "items": {"type": "string"}}
  }
}`
	return &Schema{Name: "failure_classification", Definition: json.RawMessage(def)}
}

func systemPrompt() string {
	return "You are an expert Azure Container Networking (ACN) CI failure analyst. " +
		"Given evidence from a failed pipeline run, identify the single most likely root-cause category, " +
		"a concise root-cause summary, the most relevant evidence lines, a recommended owning team, " +
		"and a proposed fix. " +
		"Categories: pr_regression (the change under test broke it), cluster_bringup_failure (provisioning/readiness), " +
		"pipeline_infra_config (agent/quota/credentials/connectivity, not product code), known_flake (recognized intermittent), " +
		"unknown_needs_human (cannot determine). Treat the deterministic signature pre-matches as strong hints, not ground truth. " +
		"Always investigate node and nodepool health before blaming the change under test: inspect node Ready/NotReady status, " +
		"reboots, reimage, resource pressure (MemoryPressure/DiskPressure/PIDPressure), evictions, and node-scoped events. " +
		"A component restart (for example CNS logging \"caught exit signal terminated\" followed by a restart) is expected when a node " +
		"reboots, is reimaged, drains, or goes NotReady; when such a restart coincides with a node lifecycle event, prefer " +
		"pipeline_infra_config or cluster_bringup_failure over pr_regression. Record your node/nodepool findings in nodeAssessment " +
		"(state explicitly if the nodes were healthy and node health was not a factor). " +
		"When prior validated resolutions are provided and clearly match the evidence, prefer them; treat in-flight (unvalidated) incidents as context only. " +
		"\n\n" +
		"USE THE CHANGE UNDER TEST. When a diff, commits, and changed-file source are provided, correlate the failure with the specific lines that changed. " +
		"If you conclude pr_regression, your proposedFix MUST name the exact file(s) and function/line(s) from the diff and describe the concrete edit. " +
		"\n\n" +
		"PROPOSED FIX QUALITY IS THE PRIORITY. Never answer with generic advice such as \"check the CNS logs\" or \"investigate pool exhaustion\". " +
		"A usable proposedFix (a) names concrete file path(s) and, where possible, the function or line, (b) states the specific change or command to run " +
		"(e.g. a kubectl/make/go command, a config value, a code edit), and (c) ends with a verification step (the exact test, command, or signal that confirms the fix). " +
		"Ground every claim in the provided evidence, diff, and versions — do not invent identifiers that do not appear in the inputs. " +
		"\n\n" +
		"IF YOU CANNOT YET WRITE A SPECIFIC FIX, request the minimum extra evidence via evidenceRequest using ONLY these forms: " +
		"\"source:<changed-file-path>\" (full current content of a file listed under Change under test) or \"log:<evidence-file-name>\" (a file listed under Evidence excerpts). " +
		"Request only what would change your answer; otherwise return an empty evidenceRequest. You get at most one round of additional evidence, so choose well. " +
		"Respond strictly in the required JSON schema."
}

func userPrompt(rc model.RunContext, ev model.Evidence, fp model.Fingerprint, matches []model.SignatureMatch, prior PriorContext, extra map[string]string) string {
	var b strings.Builder

	b.WriteString("## Scenario\n")
	fmt.Fprintf(&b, "Pipeline: %s\n", rc.PipelineName)
	fmt.Fprintf(&b, "Stage/Job: %s / %s\n", rc.StageName, rc.JobName)
	fmt.Fprintf(&b, "Cluster: %s (type=%s, os=%s, cni=%s, region=%s)\n", rc.ClusterName, rc.ClusterType, rc.OS, rc.CNI, rc.Region)
	if rc.IsPR {
		fmt.Fprintf(&b, "Pull request: #%s (source=%s target=%s)\n", rc.PullRequestNumber, rc.SourceBranch, rc.TargetBranch)
	}
	fmt.Fprintf(&b, "Fingerprint: %s\n\n", fp.Hash)

	writeVersions(&b, rc.Versions)
	writeCodeContext(&b, rc.CodeContext)
	writePriorContext(&b, prior)

	if len(matches) > 0 {
		b.WriteString("## Candidate known signatures (deterministic pre-match)\n")
		for _, m := range matches {
			fmt.Fprintf(&b, "- %s [%s, conf=%.2f]: %s\n", m.ID, m.Category, m.Confidence, m.Description)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Top error lines\n")
	for _, l := range ev.TopErrorLines {
		fmt.Fprintf(&b, "- %s\n", l)
	}

	b.WriteString("\n## Evidence excerpts\n")
	writeExcerpts(&b, ev.Excerpts)

	writeExtraEvidence(&b, extra)

	return b.String()
}

// maxDiffPromptChars caps how much of the diff is sent to the model.
const maxDiffPromptChars = 12000

// maxSourcePromptChars caps how much of each changed-file source excerpt is sent.
const maxSourcePromptChars = 3000

// writeVersions renders the component versions in effect for the run.
func writeVersions(b *strings.Builder, versions map[string]string) {
	if len(versions) == 0 {
		return
	}
	b.WriteString("## Environment versions\n")
	keys := make([]string, 0, len(versions))
	for k := range versions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(b, "- %s: %s\n", k, versions[k])
	}
	b.WriteString("\n")
}

// writeCodeContext renders the change under test: changed files, commits, the
// diff against the base branch, and any changed-file source excerpts.
func writeCodeContext(b *strings.Builder, cc model.CodeContext) {
	if cc.IsEmpty() {
		return
	}
	fmt.Fprintf(b, "## Change under test (diff %s...%s)\n", cc.BaseRef, cc.HeadRef)

	if len(cc.ChangedFiles) > 0 {
		b.WriteString("Changed files:\n")
		for _, f := range cc.ChangedFiles {
			fmt.Fprintf(b, "- %s\n", f)
		}
	}
	if len(cc.Commits) > 0 {
		b.WriteString("Commits:\n")
		for _, c := range cc.Commits {
			short := c.SHA
			if len(short) > 12 {
				short = short[:12]
			}
			fmt.Fprintf(b, "- %s %s\n", short, c.Subject)
		}
	}
	if cc.Diff != "" {
		b.WriteString("\nDiff (truncated to budget):\n```diff\n")
		b.WriteString(clip(cc.Diff, maxDiffPromptChars))
		b.WriteString("\n```\n")
	}
	if len(cc.SourceExcerpts) > 0 {
		b.WriteString("\nChanged-file source at HEAD:\n")
		names := make([]string, 0, len(cc.SourceExcerpts))
		for name := range cc.SourceExcerpts {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(b, "### %s\n%s\n", name, clip(cc.SourceExcerpts[name], maxSourcePromptChars))
		}
	}
	b.WriteString("\n")
}

// writeExtraEvidence renders the allow-listed evidence served during the deepen
// pass, clearly labeled as the response to the model's own request.
func writeExtraEvidence(b *strings.Builder, extra map[string]string) {
	if len(extra) == 0 {
		return
	}
	b.WriteString("\n## Additional evidence you requested (final round — give your definitive answer now)\n")
	names := make([]string, 0, len(extra))
	for name := range extra {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(b, "### %s\n%s\n", name, clip(extra[name], maxSourcePromptChars))
	}
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n... [truncated]"
}

// nodeEvidenceKeys are excerpt names that describe node/nodepool health. They
// are emitted before the alphabetical remainder so the node-lifecycle signal is
// never starved out of the prompt by the total excerpt budget.
var nodeEvidenceKeys = []string{
	"live/nodes",
	"live/node-conditions",
	"live/node-events",
	"live/events",
	"node-status.txt",
	"node-network-configs.txt",
}

func writeExcerpts(b *strings.Builder, excerpts map[string]string) {
	names := make([]string, 0, len(excerpts))
	for name := range excerpts {
		names = append(names, name)
	}
	sort.Strings(names)
	names = prioritizeNodeEvidence(names)

	total := 0
	for _, name := range names {
		if total >= maxTotalExcerptChars {
			break
		}
		chunk := excerpts[name]
		if len(chunk) > maxExcerptChars {
			chunk = chunk[:maxExcerptChars]
		}
		fmt.Fprintf(b, "### %s\n%s\n", name, chunk)
		total += len(chunk)
	}
}

// prioritizeNodeEvidence moves present node-evidence keys to the front of names,
// preserving the relative order of everything else.
func prioritizeNodeEvidence(names []string) []string {
	priority := make(map[string]bool, len(nodeEvidenceKeys))
	for _, k := range nodeEvidenceKeys {
		priority[k] = true
	}
	ordered := make([]string, 0, len(names))
	for _, k := range nodeEvidenceKeys {
		if _, ok := indexOf(names, k); ok {
			ordered = append(ordered, k)
		}
	}
	for _, n := range names {
		if !priority[n] {
			ordered = append(ordered, n)
		}
	}
	return ordered
}

func indexOf(names []string, target string) (int, bool) {
	for i, n := range names {
		if n == target {
			return i, true
		}
	}
	return 0, false
}
