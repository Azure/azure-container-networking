// This file implements the LLM-backed classification path. The classifier
// builds a grounded prompt, asks a ChatCompleter for a schema-constrained JSON
// answer, and validates it. The concrete Azure OpenAI ChatCompleter lives in
// aoai.go; tests use a fake.
package classify

import (
	"context"
	_ "embed"
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

// excerptTierReserve caps how much of the total budget one evidence tier may
// take before the others have been offered their share.
const excerptTierReserve = maxTotalExcerptChars / 3

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
	Category         string               `json:"category"`
	Confidence       float64              `json:"confidence"`
	RootCauseSummary string               `json:"rootCauseSummary"`
	FinalVerdict     string               `json:"finalVerdict"`
	TopAnomaly       string               `json:"topAnomaly"`
	FailingUnit      string               `json:"failingUnit"`
	TopEvidence      []string             `json:"topEvidence"`
	CausalChain      []model.CausalHop    `json:"causalChain"`
	SymptomVsCause   []model.SymptomCause `json:"symptomVsCause"`
	Falsification    *model.Falsification `json:"falsification"`
	EvidenceGaps     []model.EvidenceGap  `json:"evidenceGaps"`
	KnownUnknowns    []string             `json:"knownUnknowns"`
	RootCauseSources []model.RootCauseRef `json:"rootCauseSources"`
	RecommendedOwner string               `json:"recommendedOwner"`
	ProposedFix      string               `json:"proposedFix"`
	NodeAssessment   string               `json:"nodeAssessment"`
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
		FinalVerdict:     r.FinalVerdict,
		TopAnomaly:       r.TopAnomaly,
		FailingUnit:      r.FailingUnit,
		TopEvidence:      r.TopEvidence,
		CausalChain:      r.CausalChain,
		SymptomVsCause:   r.SymptomVsCause,
		Falsification:    nilIfEmpty(r.Falsification),
		EvidenceGaps:     r.EvidenceGaps,
		KnownUnknowns:    r.KnownUnknowns,
		RootCauseSources: r.RootCauseSources,
		RecommendedOwner: r.RecommendedOwner,
		ProposedFix:      r.ProposedFix,
		NodeAssessment:   r.NodeAssessment,
		Source:           "llm",
	}, nil
}

// nilIfEmpty drops a falsification object that the model returned with no
// meaningful content so it does not render an empty section.
func nilIfEmpty(f *model.Falsification) *model.Falsification {
	if f == nil {
		return nil
	}
	if strings.TrimSpace(f.Hypothesis) == "" && strings.TrimSpace(f.CorrelationResult) == "" &&
		strings.TrimSpace(f.Outcome) == "" && strings.TrimSpace(f.IfTrueExpect) == "" &&
		strings.TrimSpace(f.IfFalseExpect) == "" {
		return nil
	}
	return f
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
  "required": ["category", "confidence", "rootCauseSummary", "finalVerdict", "topAnomaly", "failingUnit", "topEvidence", "causalChain", "symptomVsCause", "falsification", "evidenceGaps", "knownUnknowns", "rootCauseSources", "recommendedOwner", "proposedFix", "nodeAssessment"],
  "properties": {
    "category": {"type": "string", "enum": ["pr_regression", "cluster_bringup_failure", "pipeline_infra_config", "known_flake", "unknown_needs_human"]},
    "confidence": {"type": "number", "minimum": 0, "maximum": 1},
    "rootCauseSummary": {"type": "string"},
    "finalVerdict": {"type": "string"},
    "topAnomaly": {"type": "string"},
    "failingUnit": {"type": "string"},
    "topEvidence": {"type": "array", "items": {"type": "string"}},
    "causalChain": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["step", "timestamp", "citation"],
        "properties": {
          "step": {"type": "string"},
          "timestamp": {"type": "string"},
          "citation": {"type": "string"}
        }
      }
    },
    "symptomVsCause": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["signal", "classification", "justification"],
        "properties": {
          "signal": {"type": "string"},
          "classification": {"type": "string", "enum": ["symptom", "cause"]},
          "justification": {"type": "string"}
        }
      }
    },
    "falsification": {
      "type": "object",
      "additionalProperties": false,
      "required": ["hypothesis", "ifTrueExpect", "ifFalseExpect", "correlationResult", "outcome"],
      "properties": {
        "hypothesis": {"type": "string"},
        "ifTrueExpect": {"type": "string"},
        "ifFalseExpect": {"type": "string"},
        "correlationResult": {"type": "string"},
        "outcome": {"type": "string", "enum": ["confirmed", "refuted", "inconclusive"]}
      }
    },
    "evidenceGaps": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["missing", "whereItLives", "whyMissing", "howToCapture"],
        "properties": {
          "missing": {"type": "string"},
          "whereItLives": {"type": "string"},
          "whyMissing": {"type": "string"},
          "howToCapture": {"type": "string"}
        }
      }
    },
    "knownUnknowns": {"type": "array", "items": {"type": "string"}},
    "rootCauseSources": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["file", "line", "endLine", "snippet", "explanation"],
        "properties": {
          "file": {"type": "string"},
          "line": {"type": "integer"},
          "endLine": {"type": "integer"},
          "snippet": {"type": "string"},
          "explanation": {"type": "string"}
        }
      }
    },
    "recommendedOwner": {"type": "string"},
    "proposedFix": {"type": "string"},
    "nodeAssessment": {"type": "string"}
  }
}`
	return &Schema{Name: "failure_classification", Definition: json.RawMessage(def)}
}

func systemPrompt() string {
	return investigationPolicy
}

// investigationPolicy is the failure-agent's investigation contract. It enforces
// an evidence-first, verdict-last method: explain the most severe anomaly, cite a
// primary source for every causal hop, falsify the leading hypothesis via
// cross-dimension correlation, reason about expired/absent evidence, and route by
// the actual failing unit. Confidence is calibrated by the model against listed
// known-unknowns (no deterministic override downstream).
//
//go:embed investigation-playbook.md
var investigationPolicy string

func userPrompt(rc model.RunContext, ev model.Evidence, fp model.Fingerprint, matches []model.SignatureMatch, prior PriorContext) string {
	var b strings.Builder

	b.WriteString("## Scenario\n")
	fmt.Fprintf(&b, "Pipeline: %s\n", rc.PipelineName)
	fmt.Fprintf(&b, "Stage/Job: %s / %s\n", rc.StageName, rc.JobName)
	fmt.Fprintf(&b, "Cluster: %s (type=%s, os=%s, cni=%s, region=%s)\n", rc.ClusterName, rc.ClusterType, rc.OS, rc.CNI, rc.Region)
	if rc.IsPR {
		fmt.Fprintf(&b, "Pull request: #%s (source=%s target=%s)\n", rc.PullRequestNumber, rc.SourceBranch, rc.TargetBranch)
	}
	fmt.Fprintf(&b, "Fingerprint: %s\n\n", fp.Hash)

	writeChangeContext(&b, rc)

	if len(ev.Files) > 0 {
		b.WriteString("## Collected artifacts (inventory)\n")
		b.WriteString("Account for the most severe anomaly in EACH of these before concluding; do not stop at the first matching signal.\n")
		for _, f := range ev.Files {
			fmt.Fprintf(&b, "- %s\n", f)
		}
		b.WriteString("\n")
	}

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

	b.WriteString("\n## Evidence retention notes\n")
	b.WriteString("- Kubernetes events have a ~1h TTL; an empty `kubectl get events` captured long after the failure is inconclusive, not proof of absence.\n")
	b.WriteString("- Durable corroborators that survive TTL: node condition lastTransitionTime, container restart counts/ages, Started/Finished timestamps, file mtimes.\n")
	b.WriteString("- Datapath state dumps (azure-cns.json, cnsCache.txt, hns-endpoint.json, extracted endpoint/routes/ports/vfpOutput) and live IP-plane probes (live/nnc, live/clustersubnetstate) are point-in-time snapshots: live probes show CURRENT state that may have drifted since failure, and Windows HNS/VFP state exists only in the collected bundle.\n")
	b.WriteString("- If the decisive log or event has expired or was never collected, name where it lives and the exact command to capture it next run.\n")

	b.WriteString("\n## Evidence excerpts\n")
	writeExcerpts(&b, ev.Excerpts)

	return b.String()
}

// writeChangeContext emits the change under test: the changed-file list and the
// unified diff. This is the primary evidence for the code-correlation half of the
// pr_regression check. Without it the only available signal is how broadly the
// failure reproduces, and a regression in a shared component — which fails every
// stage that exercises it — reads as environmental uniformity.
func writeChangeContext(b *strings.Builder, rc model.RunContext) {
	if len(rc.ChangedFiles) == 0 && rc.Diff == "" {
		return
	}

	b.WriteString("## Change under test\n")
	b.WriteString("Check whether the failing mechanism is reachable from these lines before ruling on pr_regression.\n")
	if len(rc.ChangedFiles) > 0 {
		b.WriteString("Changed files:\n")
		for _, f := range rc.ChangedFiles {
			fmt.Fprintf(b, "- %s\n", f)
		}
	}
	if rc.Diff != "" {
		b.WriteString("\nUnified diff:\n```diff\n")
		b.WriteString(rc.Diff)
		b.WriteString("\n```\n")
	}
	b.WriteString("\n")
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

// datapathEvidenceKeys are exact live-probe excerpt names for the IP
// control-plane. Like node evidence, they are pinned to the front of the excerpt
// budget so the requested-vs-allocated allocation and IP-exhaustion state is
// never starved out of the prompt.
var datapathEvidenceKeys = []string{
	"live/nnc",
	"live/clustersubnetstate",
}

// datapathEvidenceRE matches bundle datapath/IP-plane excerpt paths, which are
// node-name-prefixed and therefore cannot be pinned by exact key. It mirrors the
// collector allowlist so surfaced IP-state dumps (CNS/CNI IPAM view, endpoints,
// routes, VFP) are prioritized into the prompt budget alongside node evidence.
var datapathEvidenceRE = regexp.MustCompile(`(?i)(^|/)(azure-cns|azure-vnet|cnscache|azure-endpoints|hns-endpoint|hns-network|endpoint|routes|ports|vfpoutput|ip|all-pods)(\.[a-z]+)?$`)

// assertionEvidenceRE matches the captured E2E task logs, which carry the test's
// own assertion text. It mirrors the collector allowlist.
var assertionEvidenceRE = regexp.MustCompile(`(?i)(^|/)e2e-task-logs/`)

func writeExcerpts(b *strings.Builder, excerpts map[string]string) {
	names := make([]string, 0, len(excerpts))
	for name := range excerpts {
		names = append(names, name)
	}
	sort.Strings(names)

	tiers := tierEvidence(names)
	written := make(map[string]bool, len(names))
	total := 0

	// Two passes. The first gives every tier its own reserve so a verbose tier
	// cannot starve the others — node evidence alone is two full `kubectl
	// describe nodes` dumps plus every cluster event, which exceeds the whole
	// budget and would leave the assertion and IP-plane state unread. The second
	// spends whatever is left in priority order.
	for _, tier := range tiers {
		spent := 0
		for _, name := range tier {
			if spent >= excerptTierReserve || total >= maxTotalExcerptChars {
				break
			}
			n := writeExcerpt(b, name, excerpts[name])
			written[name] = true
			spent += n
			total += n
		}
	}
	for _, tier := range tiers {
		for _, name := range tier {
			if total >= maxTotalExcerptChars {
				return
			}
			if written[name] {
				continue
			}
			total += writeExcerpt(b, name, excerpts[name])
			written[name] = true
		}
	}
}

// writeExcerpt emits one excerpt, truncated to the per-file cap, and reports how
// much of the budget it consumed.
func writeExcerpt(b *strings.Builder, name, chunk string) int {
	if len(chunk) > maxExcerptChars {
		chunk = chunk[:maxExcerptChars]
	}
	fmt.Fprintf(b, "### %s\n%s\n", name, chunk)
	return len(chunk)
}

// tierEvidence groups names into the assertion, node, datapath, and remaining
// tiers. Assertion evidence leads because it is the failure itself rather than
// state around it; node evidence follows so an infra cause is never missed.
// Exact pinned keys keep their declared order, regex-matched bundle paths follow
// in sorted order, and everything else keeps sorted order.
func tierEvidence(names []string) [][]string {
	claimed := make(map[string]bool, len(names))

	take := func(pred func(string) bool) []string {
		var out []string
		for _, n := range names {
			if !claimed[n] && pred(n) {
				claimed[n] = true
				out = append(out, n)
			}
		}
		return out
	}
	takeKeys := func(keys []string) []string {
		var out []string
		for _, k := range keys {
			if _, ok := indexOf(names, k); ok && !claimed[k] {
				claimed[k] = true
				out = append(out, k)
			}
		}
		return out
	}

	assertion := take(assertionEvidenceRE.MatchString)
	node := takeKeys(nodeEvidenceKeys)
	datapath := append(takeKeys(datapathEvidenceKeys), take(datapathEvidenceRE.MatchString)...)
	rest := take(func(string) bool { return true })

	return [][]string{assertion, node, datapath, rest}
}

func indexOf(names []string, target string) (int, bool) {
	for i, n := range names {
		if n == target {
			return i, true
		}
	}
	return 0, false
}
