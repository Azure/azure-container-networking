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

// LLMClassifier produces a Classification via a ChatCompleter, grounded by the
// fingerprint, signature matches, scenario, and trimmed evidence.
type LLMClassifier struct {
	client ChatCompleter
}

// NewLLMClassifier returns a classifier backed by client.
func NewLLMClassifier(client ChatCompleter) *LLMClassifier {
	return &LLMClassifier{client: client}
}

// Classify asks the model to categorize the failure and validates the result.
// A malformed or out-of-contract response is an error so the caller can fail.
func (c *LLMClassifier) Classify(ctx context.Context, rc model.RunContext, ev model.Evidence, fp model.Fingerprint, matches []model.SignatureMatch, prior PriorContext) (model.Classification, error) {
	raw, err := c.client.Complete(ctx, systemPrompt(), userPrompt(rc, ev, fp, matches, prior), classificationSchema())
	if err != nil {
		return model.Classification{}, fmt.Errorf("llm completion: %w", err)
	}

	var res llmResult
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return model.Classification{}, fmt.Errorf("parsing llm response: %w", err)
	}
	return res.toClassification()
}

type llmResult struct {
	Category         string   `json:"category"`
	Confidence       float64  `json:"confidence"`
	RootCauseSummary string   `json:"rootCauseSummary"`
	TopEvidence      []string `json:"topEvidence"`
	RecommendedOwner string   `json:"recommendedOwner"`
	ProposedFix      string   `json:"proposedFix"`
	NodeAssessment   string   `json:"nodeAssessment"`
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
  "required": ["category", "confidence", "rootCauseSummary", "topEvidence", "recommendedOwner", "proposedFix", "nodeAssessment"],
  "properties": {
    "category": {"type": "string", "enum": ["pr_regression", "cluster_bringup_failure", "pipeline_infra_config", "known_flake", "unknown_needs_human"]},
    "confidence": {"type": "number", "minimum": 0, "maximum": 1},
    "rootCauseSummary": {"type": "string"},
    "topEvidence": {"type": "array", "items": {"type": "string"}},
    "recommendedOwner": {"type": "string"},
    "proposedFix": {"type": "string"},
    "nodeAssessment": {"type": "string"}
  }
}`
	return &Schema{Name: "failure_classification", Definition: json.RawMessage(def)}
}

func systemPrompt() string {
	return "You are an expert Azure Container Networking (ACN) CI failure analyst. " +
		"Given evidence from a failed pipeline run, identify the single most likely root-cause category, " +
		"a concise root-cause summary, the most relevant evidence lines, a recommended owning team, " +
		"and a proposed fix with concrete, actionable steps the developer should take to resolve the failure. " +
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
		"Base your answer only on the provided evidence and respond strictly in the required JSON schema."
}

func userPrompt(rc model.RunContext, ev model.Evidence, fp model.Fingerprint, matches []model.SignatureMatch, prior PriorContext) string {
	var b strings.Builder

	b.WriteString("## Scenario\n")
	fmt.Fprintf(&b, "Pipeline: %s\n", rc.PipelineName)
	fmt.Fprintf(&b, "Stage/Job: %s / %s\n", rc.StageName, rc.JobName)
	fmt.Fprintf(&b, "Cluster: %s (type=%s, os=%s, cni=%s, region=%s)\n", rc.ClusterName, rc.ClusterType, rc.OS, rc.CNI, rc.Region)
	if rc.IsPR {
		fmt.Fprintf(&b, "Pull request: #%s (source=%s target=%s)\n", rc.PullRequestNumber, rc.SourceBranch, rc.TargetBranch)
	}
	if len(rc.ChangedFiles) > 0 {
		b.WriteString("Changed files:\n")
		for _, f := range rc.ChangedFiles {
			fmt.Fprintf(&b, "- %s\n", f)
		}
	}
	fmt.Fprintf(&b, "Fingerprint: %s\n\n", fp.Hash)

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

	return b.String()
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
