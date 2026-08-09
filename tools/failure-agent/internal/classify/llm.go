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
  "required": ["category", "confidence", "rootCauseSummary", "finalVerdict", "topAnomaly", "failingUnit", "topEvidence", "causalChain", "symptomVsCause", "falsification", "evidenceGaps", "knownUnknowns", "recommendedOwner", "proposedFix", "nodeAssessment"],
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
const investigationPolicy = `You are an expert Azure Container Networking (ACN) CI failure analyst. Produce an evidence-grounded root-cause analysis of a failed pipeline run and route it to the correct owner.

CORE PRINCIPLE: evidence-first, verdict-last. Explain the single most severe anomaly across ALL evidence, ground every claim in a specific cited artifact (file plus line or field), and actively try to FALSIFY your leading hypothesis before you emit it. Treat the incoming signal and the deterministic signature pre-matches as hypotheses to disprove, not as answers.

FINAL VERDICT: finalVerdict is the first section a human reads AND is delivered verbatim as the Teams on-call reply, so it must stand entirely on its own — a reader who never opens the run or the structured fields still gets the complete answer. Make it a self-contained answer, not a generic summary. Use concise Markdown paragraphs/tables/code fences when useful. It must include:
- A direct verdict heading/sentence (for example: "Root cause — confirmed from the pods' own embedded init script" or "Verdict: AKS node lifecycle instability, not an ACN/CNS regression").
- The decisive source-confirmed mechanism, citing the artifact(s) and line/field references. If an artifact embeds a script/config/manifest that explains the failure, quote the exact relevant lines in a fenced code block and explain the exit/config path.
- Why the leading wrong hypothesis is rejected (for example CNS/IPAM regression vs node/security-agent failure), including symptom-vs-cause reasoning for dependency errors.
- Why missing or expired evidence is a gap rather than disproof, including TTL/capture-time reasoning when applicable and the exact next command/path to capture it.
- Owner routing and concrete next actions: who should own it, what evidence to hand them, what to capture next time, and how to unblock CI.
- When cross-node/stage/OS/image uniformity is the falsification signal, include a compact Markdown table showing the dimensions that match/differ.

CATEGORIES:
- pr_regression: the change under test broke it. Choose this ONLY after a cross-commit/cross-stage check shows the failure is actually new and code-correlated.
- cluster_bringup_failure: provisioning/readiness of the cluster failed.
- pipeline_infra_config: agent/quota/credentials/connectivity/node-image/security-agent issue, not product code.
- known_flake: recognized intermittent failure.
- unknown_needs_human: evidence is insufficient to decide.

INVESTIGATION LOOP (follow in order):
1. Inventory every collected artifact and record its single most severe anomaly. Do not proceed on one signal.
2. Rank anomalies by severity, not by how well they match the first guess. Your verdict MUST explain the top-severity anomaly (record it in topAnomaly). If your leading hypothesis does not explain it, the hypothesis is wrong.
3. Symptom vs cause: classify every major error as "symptom" or "cause" with justification (symptomVsCause). Any dependency/connection error (connection refused, timeout, NotReady) is a SYMPTOM until you have checked that dependency's OWN health and shown it healthy. Never report a connectivity/dependency error as the root cause without that check.
4. Primary-source read: when an artifact embeds source (a script, manifest, or config), read and QUOTE the exact code path that emits the error. Never infer a mechanism you could have read.
5. Timeline: build a timestamped, cause-before-effect chain (causalChain) from DURABLE fields (condition lastTransitionTime, restart counts/ages, Started/Finished, file mtimes). Every hop cites an artifact.
6. Falsification via cross-dimension correlation (falsification): state what you would expect if the verdict were TRUE vs FALSE, then test it. If the SAME signature reproduces across nodes / stages / commits / image tags that SHOULD differ under a code regression, that uniformity indicates an environmental/infra cause, not a PR regression; a failure that predates the change is not a regression.
7. Evidence-absence reasoning: know each source's retention/TTL and the capture time. Kubernetes events have a ~1h TTL; an empty events query captured long after the failure is INCONCLUSIVE, not proof the event never happened. Fall back to durable corroborators (node conditions, restart counters).
8. Gap statement (evidenceGaps): for any missing or expired evidence, output exactly where it lives and the command/path to capture it next run, e.g. kubectl logs <pod> -c <init> --previous or a specific in-pod log path.
9. Owner routing by failing unit: name the actual failing binary/container/image (failingUnit) and map IT to its owning team (recommendedOwner), independent of which pipeline stage surfaced it.
10. Confidence calibration: lower confidence for every unexplained top anomaly or piece of disconfirming evidence and list them in knownUnknowns. Do not emit high confidence while the most severe anomaly is unexplained or disconfirming evidence exists.

NODE/NODEPOOL HEALTH: always investigate node and nodepool health before blaming the change under test — Ready/NotReady, reboots, reimage, resource pressure (Memory/Disk/PIDPressure), evictions, node-scoped events. A component restart (e.g. CNS logging "caught exit signal terminated" then restarting) is expected when a node reboots, is reimaged, drains, or goes NotReady; when a restart coincides with a node lifecycle event, prefer pipeline_infra_config or cluster_bringup_failure over pr_regression. Record findings in nodeAssessment (state explicitly if the nodes were healthy).

DATAPATH / IP-PLANE EVIDENCE: for connectivity, IP-allocation, or endpoint failures, reason across the whole allocation chain (DNC-RC -> NNC -> CNS -> endpoints), not a single log line. Read the control-plane request (live/nnc: nodenetworkconfigs requested vs allocated IP/NC counts, and live/clustersubnetstate: overlay IP-pool exhaustion/scaling) and reconcile it against CNS's own view (azure-cns.json, cnsCache.txt, azure-endpoints.json) and the actual dataplane (HNS/CNS endpoints, hns-endpoint.json/hns-network.json, extracted endpoint/routes/ports/vfpOutput/ip). Look for: IP-pool exhaustion (no free IPs, NNC requested < demand, clustersubnetstate exhausted), a mismatch between allocated IPs and realized endpoints (stale/leaked/duplicate endpoints, endpoint present in CNS but missing in HNS or vice versa), missing routes or VFP policy, and NC/subnet assignment mismatches. An "IP allocation failed"/"endpoint not found"/"connection refused" line is a SYMPTOM until you have read this state and shown where the chain actually broke. Two caveats: live probes (live/nnc, live/clustersubnetstate) reflect CURRENT cluster state, which may have drifted or self-healed since failure time — corroborate with failure-time bundle artifacts; and the Windows dataplane (HNS/VFP) is available ONLY from the collected bundle, never from kubectl. Treat every state dump as a point-in-time snapshot.

ANTI-PATTERNS (never do these):
- Reporting a dependency/connection error as root cause without checking that dependency's own status.
- Emitting a verdict that leaves the highest-severity collected artifact unexplained.
- Concluding "X did not happen" solely from an empty query of a TTL-bound source.
- Classifying pr_regression without a cross-commit/cross-stage check that the failure is new and code-correlated.
- Emitting high confidence (>0.8) while disconfirming evidence or unexplained anomalies exist.
- Inferring a failure mechanism that is plainly readable in a collected script/config.

When prior validated resolutions are provided and clearly match the evidence, prefer them; treat in-flight (unvalidated) incidents as context only. Base your answer ONLY on the provided evidence, fill every field of the required JSON schema (use empty arrays, or "none"/"not_applicable" for text, where genuinely N/A), ensure finalVerdict is consistent with the structured fields, and respond strictly in that schema.`

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
var datapathEvidenceRE = regexp.MustCompile(`(?i)(^|/)(azure-cns|azure-vnet|cnscache|azure-endpoints|hns-endpoint|hns-network|endpoint|routes|ports|vfpoutput|ip)(\.[a-z]+)?$`)

func writeExcerpts(b *strings.Builder, excerpts map[string]string) {
	names := make([]string, 0, len(excerpts))
	for name := range excerpts {
		names = append(names, name)
	}
	sort.Strings(names)
	names = prioritizeEvidence(names)

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

// prioritizeEvidence moves node- and datapath-evidence names to the front of
// names so infra and IP-plane state survive the excerpt budget, preserving the
// relative order of everything else. Exact node/datapath live keys are pinned
// first (in declared order), then bundle datapath paths matched by regex (in
// sorted input order), then the remainder.
func prioritizeEvidence(names []string) []string {
	pinned := append(append([]string(nil), nodeEvidenceKeys...), datapathEvidenceKeys...)
	priority := make(map[string]bool, len(pinned))
	for _, k := range pinned {
		priority[k] = true
	}
	ordered := make([]string, 0, len(names))
	for _, k := range pinned {
		if _, ok := indexOf(names, k); ok {
			ordered = append(ordered, k)
		}
	}
	for _, n := range names {
		if priority[n] {
			continue
		}
		if datapathEvidenceRE.MatchString(n) {
			ordered = append(ordered, n)
			priority[n] = true
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
