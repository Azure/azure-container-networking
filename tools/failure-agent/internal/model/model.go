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
	IsPR              bool   `json:"isPR"`
	PullRequestNumber string `json:"pullRequestNumber,omitempty"`
	SourceBranch      string `json:"sourceBranch,omitempty"`
	TargetBranch      string `json:"targetBranch,omitempty"`
	SourceCommitID    string `json:"sourceCommitId,omitempty"`
	CommitID          string `json:"commitId,omitempty"`

	// Scenario identity.
	ClusterName string `json:"clusterName,omitempty"`
	ClusterType string `json:"clusterType,omitempty"`
	Region      string `json:"region,omitempty"`
	OS          string `json:"os,omitempty"`
	CNI         string `json:"cni,omitempty"`

	// CodeContext is the change under test (diff vs base, commits, changed-file
	// source excerpts). It grounds the classifier so a proposed fix can cite the
	// actual lines that changed rather than guessing from logs alone.
	CodeContext CodeContext `json:"codeContext,omitempty"`
	// Versions are the component versions in effect for this run (AKS,
	// Kubernetes, Cilium, CNS/CNI images, etc.), keyed by component name.
	Versions map[string]string `json:"versions,omitempty"`
}

// CodeContext captures "the change under test": the diff against the base
// branch, the commits that produced it, the changed-file inventory, and capped
// source excerpts of the changed files at HEAD. It is best-effort — any field
// may be empty when the git checkout or base ref is unavailable.
type CodeContext struct {
	BaseRef        string            `json:"baseRef,omitempty"`
	HeadRef        string            `json:"headRef,omitempty"`
	ChangedFiles   []string          `json:"changedFiles,omitempty"`
	DiffStat       string            `json:"diffStat,omitempty"`
	Diff           string            `json:"diff,omitempty"`
	Commits        []CommitMeta      `json:"commits,omitempty"`
	SourceExcerpts map[string]string `json:"-"`
}

// isEmpty reports whether no change context was collected.
func (c CodeContext) IsEmpty() bool {
	return len(c.ChangedFiles) == 0 && c.Diff == "" && len(c.Commits) == 0
}

// CommitMeta is a single commit in the change range.
type CommitMeta struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
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
	TopEvidence      []string        `json:"topEvidence"`
	RecommendedOwner string          `json:"recommendedOwner,omitempty"`
	ProposedFix      string          `json:"proposedFix,omitempty"`
	// NodeAssessment records what node/nodepool health showed and whether a node
	// lifecycle event (reboot, reimage, NotReady, eviction) contributed to the
	// failure. It exists so a CNS/agent restart is not misattributed to a PR
	// regression when the node itself went down.
	NodeAssessment string `json:"nodeAssessment,omitempty"`
	Source         string `json:"source"` // "llm" or "none" when analysis failed
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

	// CodeContext, ChangedFiles, and Versions record the change under test and
	// the component versions used, so a recorded incident stays traceable without
	// re-deriving them from the pipeline environment. CodeContext intentionally
	// omits source excerpts from JSON.
	CodeContext  *CodeContext      `json:"codeContext,omitempty"`
	ChangedFiles []string          `json:"changedFiles,omitempty"`
	Versions     map[string]string `json:"versions,omitempty"`

	Fingerprint string `json:"fingerprint"`

	Category         FailureCategory `json:"category"`
	Confidence       float64         `json:"confidence"`
	ConfidenceBand   ConfidenceBand  `json:"confidenceBand"`
	RootCauseSummary string          `json:"rootCauseSummary"`
	RecommendedOwner string          `json:"recommendedOwner,omitempty"`
	NodeAssessment   string          `json:"nodeAssessment,omitempty"`

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
}
