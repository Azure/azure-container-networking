// Package model holds the shared types exchanged between the failure-agent
// stages: evidence collection, fingerprinting, signature matching,
// classification, policy, and reporting.
package model

import "time"

// FailureCategory is the likely origin of a pipeline failure.
type FailureCategory string

const (
	// CategoryPRRegression is a failure caused by the change under test.
	CategoryPRRegression FailureCategory = "pr_regression"
	// CategoryClusterBringupFailure is a failure provisioning or readying the cluster.
	CategoryClusterBringupFailure FailureCategory = "cluster_bringup_failure"
	// CategoryPipelineInfraConfig is a failure in pipeline/infra/config rather than product code.
	CategoryPipelineInfraConfig FailureCategory = "pipeline_infra_config"
	// CategoryKnownFlake is a recognized intermittent failure.
	CategoryKnownFlake FailureCategory = "known_flake"
	// CategoryUnknownNeedsHuman is an unclassified failure needing human triage.
	CategoryUnknownNeedsHuman FailureCategory = "unknown_needs_human"
)

// ConfidenceBand buckets a numeric confidence for human-facing output.
type ConfidenceBand string

const (
	// BandHigh is confidence >= 0.8.
	BandHigh ConfidenceBand = "high"
	// BandMedium is confidence in [0.5, 0.8).
	BandMedium ConfidenceBand = "medium"
	// BandLow is confidence < 0.5.
	BandLow ConfidenceBand = "low"
)

// AnalysisStatus records whether LLM analysis produced a classification.
type AnalysisStatus string

const (
	// StatusAnalyzed means the LLM returned a valid classification.
	StatusAnalyzed AnalysisStatus = "analyzed"
	// StatusAnalysisFailed means the LLM call or its response could not be
	// used; the incident still carries raw evidence for human triage.
	StatusAnalysisFailed AnalysisStatus = "analysis_failed"
)

// RetentionDecision is the agent's recommendation about the failed cluster.
// It is advisory only; the agent never changes teardown behavior.
type RetentionDecision string

const (
	// RetentionDelete recommends the normal teardown proceed.
	RetentionDelete RetentionDecision = "delete"
	// RetentionRetainTTL recommends retaining the cluster for a short TTL for inspection.
	RetentionRetainTTL RetentionDecision = "retain_ttl"
)

// RunContext is the pipeline/scenario metadata for a single failed run,
// sourced from the CI environment.
type RunContext struct {
	PipelineName string `json:"pipelineName"`
	BuildID      string `json:"buildId"`
	BuildNumber  string `json:"buildNumber"`
	Repository   string `json:"repository"`

	StageName string `json:"stageName"`
	JobName   string `json:"jobName"`

	// Pull request context. IsPR is false for scheduled/release runs.
	IsPR              bool     `json:"isPR"`
	PullRequestNumber string   `json:"pullRequestNumber,omitempty"`
	SourceBranch      string   `json:"sourceBranch,omitempty"`
	TargetBranch      string   `json:"targetBranch,omitempty"`
	SourceCommitID    string   `json:"sourceCommitId,omitempty"`
	CommitID          string   `json:"commitId,omitempty"`
	ChangedFiles      []string `json:"changedFiles,omitempty"`
	// Diff is the unified diff of the change under test. It grounds the
	// code-correlation half of the pr_regression check: without it, breadth of
	// failure is the only available signal and a regression in a shared component
	// (which fails every stage that exercises it) is easily misread as infra.
	Diff string `json:"-"`

	// Scenario identity.
	ClusterName string `json:"clusterName,omitempty"`
	ClusterType string `json:"clusterType,omitempty"`
	Region      string `json:"region,omitempty"`
	OS          string `json:"os,omitempty"`
	CNI         string `json:"cni,omitempty"`
}

// Evidence is the parsed failure bundle collected from the log artifact.
type Evidence struct {
	Root          string            `json:"root"`
	Files         []string          `json:"files"`
	TopErrorLines []string          `json:"topErrorLines"`
	ErrorSnippets []ErrorSnippet    `json:"errorSnippets,omitempty"`
	Excerpts      map[string]string `json:"-"`
}

// ErrorSnippet captures context around a matched failure line.
type ErrorSnippet struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

// RootCauseRef pins the confirmed root cause to an exact location in a collected
// artifact: the file, the decisive line (and optional end line for a range), and
// the full snippet copied verbatim from the line-numbered excerpt the model was
// given. Explanation is an optional one-line note on why this location is causal.
// Unlike ErrorSnippet (regex-driven noise), these are curated by the model to
// point at the actual cause.
type RootCauseRef struct {
	File        string `json:"file"`
	Line        int    `json:"line"`
	EndLine     int    `json:"endLine,omitempty"`
	Snippet     string `json:"snippet"`
	Explanation string `json:"explanation,omitempty"`
}

// Fingerprint is a stable identifier for a class of failure, used for
// recurrence detection and idempotent reporting.
type Fingerprint struct {
	Hash             string `json:"hash"`
	NormalizedSignal string `json:"normalizedSignal"`
}

// SignatureMatch is a known failure pattern matched against the evidence.
type SignatureMatch struct {
	ID             string          `json:"id"`
	Category       FailureCategory `json:"category"`
	Description    string          `json:"description"`
	Owner          string          `json:"owner,omitempty"`
	Recommendation string          `json:"recommendation,omitempty"`
	Confidence     float64         `json:"confidence"`
	MatchedOn      string          `json:"matchedOn"`
}

// Classification is the LLM-produced root-cause assessment.
type Classification struct {
	Category         FailureCategory `json:"category"`
	Confidence       float64         `json:"confidence"`
	RootCauseSummary string          `json:"rootCauseSummary"`
	// FinalVerdict is the self-contained human answer rendered at the top of the
	// report. It summarizes the confirmed mechanism, evidence gaps, owner routing,
	// and immediate next actions in prose.
	FinalVerdict     string   `json:"finalVerdict,omitempty"`
	TopEvidence      []string `json:"topEvidence"`
	RecommendedOwner string   `json:"recommendedOwner,omitempty"`
	ProposedFix      string   `json:"proposedFix,omitempty"`
	// NodeAssessment records what node/nodepool health showed and whether a node
	// lifecycle event (reboot, reimage, NotReady, eviction) contributed to the
	// failure. It exists so a CNS/agent restart is not misattributed to a PR
	// regression when the node itself went down.
	NodeAssessment string `json:"nodeAssessment,omitempty"`

	// TopAnomaly is the single most severe anomaly across all evidence. The
	// verdict must explain it; a verdict that leaves it unexplained is not
	// grounded.
	TopAnomaly string `json:"topAnomaly,omitempty"`
	// FailingUnit is the concrete failing binary/container/image that the owner
	// is derived from (for example "install-packages.sh in the AzSecPack init
	// container"), independent of which pipeline stage surfaced the failure.
	FailingUnit string `json:"failingUnit,omitempty"`
	// CausalChain is the ordered, timestamped, cited cause->effect sequence that
	// connects the root cause to the observed pipeline failure.
	CausalChain []CausalHop `json:"causalChain,omitempty"`
	// SymptomVsCause labels each major error as a symptom or a cause so a
	// downstream casualty (e.g. "connection refused") is not reported as the root.
	SymptomVsCause []SymptomCause `json:"symptomVsCause,omitempty"`
	// Falsification is the disconfirmation test applied to the leading
	// hypothesis before the verdict was emitted.
	Falsification *Falsification `json:"falsification,omitempty"`
	// EvidenceGaps lists evidence that was missing or expired and how to capture
	// it on the next run.
	EvidenceGaps []EvidenceGap `json:"evidenceGaps,omitempty"`
	// KnownUnknowns lists unexplained anomalies or disconfirming evidence that
	// hold the confidence down.
	KnownUnknowns []string `json:"knownUnknowns,omitempty"`

	// RootCauseSources pins the confirmed cause to exact artifact file(s), line(s),
	// and full snippet(s). Empty when the cause is not readable in any collected
	// artifact.
	RootCauseSources []RootCauseRef `json:"rootCauseSources,omitempty"`

	Source string `json:"source"` // "llm" or "none" when analysis failed
}

// CausalHop is one ordered, cited step in the failure's cause->effect chain.
// Timestamp is a durable field (condition lastTransitionTime, restart age,
// Started/Finished, file mtime) when one is available; Citation grounds the hop
// in a specific artifact plus line/field.
type CausalHop struct {
	Step      string `json:"step"`
	Timestamp string `json:"timestamp,omitempty"`
	Citation  string `json:"citation"`
}

// SymptomCause classifies one observed error as a symptom or a cause. Any
// dependency/connection error (connection refused, timeout, not ready) is a
// symptom unless the dependency's own health was checked and shown healthy.
type SymptomCause struct {
	Signal         string `json:"signal"`
	Classification string `json:"classification"` // "symptom" or "cause"
	Justification  string `json:"justification"`
}

// Falsification records the test applied to the leading hypothesis: what would
// be expected if it were true versus false, the cross-dimension correlation
// that was actually observed, and the resulting outcome.
type Falsification struct {
	Hypothesis        string `json:"hypothesis"`
	IfTrueExpect      string `json:"ifTrueExpect"`
	IfFalseExpect     string `json:"ifFalseExpect"`
	CorrelationResult string `json:"correlationResult"`
	Outcome           string `json:"outcome"` // "confirmed", "refuted", or "inconclusive"
}

// EvidenceGap names evidence that would have strengthened the verdict but was
// missing, why it was missing (retention/TTL or not collected), and the exact
// command or path to capture it on the next run.
type EvidenceGap struct {
	Missing      string `json:"missing"`
	WhereItLives string `json:"whereItLives"`
	WhyMissing   string `json:"whyMissing"`
	HowToCapture string `json:"howToCapture"`
}

// EscalationSource records how an Escalation was arrived at, so "the model
// declined" is distinguishable from "the gate never ran" and "the gate broke".
type EscalationSource string

const (
	// EscalationLLM means the model was consulted and returned a decision.
	EscalationLLM EscalationSource = "llm"
	// EscalationSkipped means the preconditions were not met, so no model call
	// was made. Needed is false but says nothing about the failure itself.
	EscalationSkipped EscalationSource = "skipped"
	// EscalationError means the gate was consulted but failed. Needed is false
	// because no decision was obtained, not because one was declined.
	EscalationError EscalationSource = "error"
)

// Escalation is the AI gate's decision about whether an incident warrants a
// GitHub issue for a code fix. It is deliberately independent of Category and
// Confidence: a low-confidence but clearly code-shaped failure is worth an issue,
// while a high-confidence quota exhaustion is not. Reason is populated for both
// outcomes so a decision not to escalate is as auditable as a decision to.
type Escalation struct {
	Needed bool   `json:"needed"`
	Reason string `json:"reason"`

	// Title, Labels, FixDirection, SuggestedFiles, and Blockers ground the
	// issue body and are only populated when Needed is true.
	Title          string   `json:"title,omitempty"`
	Labels         []string `json:"labels,omitempty"`
	FixDirection   string   `json:"fixDirection,omitempty"`
	SuggestedFiles []string `json:"suggestedFiles,omitempty"`
	Blockers       []string `json:"blockers,omitempty"`

	Source EscalationSource `json:"source"`
}

// Incident is the full structured result written to incident.json.
type Incident struct {
	GeneratedAt time.Time `json:"generatedAt"`

	PipelineName      string `json:"pipelineName"`
	BuildID           string `json:"buildId"`
	BuildNumber       string `json:"buildNumber"`
	Repository        string `json:"repository"`
	PullRequestNumber string `json:"pullRequestNumber,omitempty"`
	Commit            string `json:"commit,omitempty"`

	Stage string `json:"stage,omitempty"`
	Job   string `json:"job,omitempty"`

	ClusterName string `json:"clusterName,omitempty"`
	ClusterType string `json:"clusterType,omitempty"`
	Region      string `json:"region,omitempty"`
	OS          string `json:"os,omitempty"`
	CNI         string `json:"cni,omitempty"`

	Fingerprint string `json:"fingerprint"`

	Category         FailureCategory `json:"category"`
	Confidence       float64         `json:"confidence"`
	ConfidenceBand   ConfidenceBand  `json:"confidenceBand"`
	RootCauseSummary string          `json:"rootCauseSummary"`
	FinalVerdict     string          `json:"finalVerdict,omitempty"`
	RecommendedOwner string          `json:"recommendedOwner,omitempty"`
	NodeAssessment   string          `json:"nodeAssessment,omitempty"`

	// Structured triage contract (populated on analyzed incidents).
	TopAnomaly     string         `json:"topAnomaly,omitempty"`
	FailingUnit    string         `json:"failingUnit,omitempty"`
	CausalChain    []CausalHop    `json:"causalChain,omitempty"`
	SymptomVsCause []SymptomCause `json:"symptomVsCause,omitempty"`
	Falsification  *Falsification `json:"falsification,omitempty"`
	EvidenceGaps   []EvidenceGap  `json:"evidenceGaps,omitempty"`
	KnownUnknowns  []string       `json:"knownUnknowns,omitempty"`

	// RootCauseSources pins the confirmed cause to exact artifact file(s), line(s),
	// and full snippet(s). Empty when the cause is not readable in any collected
	// artifact.
	RootCauseSources []RootCauseRef `json:"rootCauseSources,omitempty"`

	TopEvidence      []string         `json:"topEvidence"`
	SignatureMatches []SignatureMatch `json:"signatureMatches"`
	EvidenceFiles    []string         `json:"evidenceFiles"`
	ErrorSnippets    []ErrorSnippet   `json:"errorSnippets,omitempty"`

	RetentionDecision RetentionDecision `json:"retentionDecision"`
	RecommendedAction string            `json:"recommendedAction"`
	ProposedFix       string            `json:"proposedFix,omitempty"`

	AnalysisStatus       AnalysisStatus `json:"analysisStatus"`
	AnalysisError        string         `json:"analysisError,omitempty"`
	ClassificationSource string         `json:"classificationSource"`

	// Escalation is the AI gate's decision about raising a GitHub issue for a
	// code fix. Nil when the gate did not run at all.
	Escalation *Escalation `json:"escalation,omitempty"`
}
