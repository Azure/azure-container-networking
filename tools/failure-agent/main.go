// Command failure-agent analyzes a failed ACN pipeline run. It parses the
// collected log bundle, fingerprints the failure, matches known signatures,
// classifies the likely root cause with an Azure OpenAI deployment, writes
// report.md + incident.json, and (for PR builds) posts the analysis back to the
// pull request.
//
// LLM analysis is required: when it is unavailable or fails, the agent still
// emits an analysis_failed incident carrying the raw evidence for human triage.
// --dry-run skips the pull-request write-back.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/classify"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/collect"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/command"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/fingerprint"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/live"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/publish"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/report"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/signatures"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/store"
	"go.uber.org/zap"
)

const (
	defaultSignaturesPath = "signatures/signatures.yaml"
	defaultAOAIAPIVersion = "2024-10-21"
	defaultTimeout        = 90 * time.Second
	publishTimeout        = 30 * time.Second
)

type options struct {
	input          string
	output         string
	signaturesPath string
	dryRun         bool

	aoaiEndpoint   string
	aoaiDeployment string
	aoaiAPIKey     string
	aoaiAPIVersion string
	timeout        time.Duration

	pipeline    string
	clusterName string
	clusterType string
	region      string
	osName      string
	cni         string

	knowledgeDB     string
	live            bool
	privileged      bool
	flakinessOutput string
}

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to init logger:", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	ctx := context.Background()
	opts := parseFlags()

	var ks knowledgeStore = noopStore{}
	var sqlStore *store.Store
	if opts.knowledgeDB != "" {
		s, err := store.Open(ctx, sqliteDriver, opts.knowledgeDB)
		if err != nil {
			logger.Error("failure analysis failed", zap.Error(err))
			os.Exit(1)
		}
		defer func() { _ = s.Close() }()
		sqlStore = s
		ks = s
		logger.Info("knowledge store opened", zap.String("path", opts.knowledgeDB))
	}

	lc := buildCollector(logger, opts)
	pc := buildPrivilegedCollector(logger, opts)

	if err := run(ctx, logger, opts, buildClassifier(opts), ks, lc, pc); err != nil {
		logger.Error("failure analysis failed", zap.Error(err))
		os.Exit(1)
	}

	if sqlStore != nil {
		emitFlakiness(ctx, logger, opts, sqlStore)
	}
}

func parseFlags() options {
	var o options
	flag.StringVar(&o.input, "input", "", "path to the collected evidence/log bundle directory (required)")
	flag.StringVar(&o.output, "output", ".", "directory to write report.md and incident.json")
	flag.StringVar(&o.signaturesPath, "signatures", defaultSignaturesPath, "path to the signatures catalog")
	flag.BoolVar(&o.dryRun, "dry-run", false, "skip pull-request write-back (analysis still runs)")
	flag.StringVar(&o.aoaiEndpoint, "aoai-endpoint", os.Getenv("AZURE_OPENAI_ENDPOINT"), "Azure OpenAI endpoint (or AZURE_OPENAI_ENDPOINT)")
	flag.StringVar(&o.aoaiDeployment, "aoai-deployment", os.Getenv("AZURE_OPENAI_DEPLOYMENT"), "Azure OpenAI deployment name (or AZURE_OPENAI_DEPLOYMENT)")
	flag.StringVar(&o.aoaiAPIKey, "aoai-api-key", os.Getenv("AZURE_OPENAI_API_KEY"), "Azure OpenAI API key (or AZURE_OPENAI_API_KEY)")
	flag.StringVar(&o.aoaiAPIVersion, "aoai-api-version", envOrDefault("AZURE_OPENAI_API_VERSION", defaultAOAIAPIVersion), "Azure OpenAI API version (or AZURE_OPENAI_API_VERSION)")
	flag.DurationVar(&o.timeout, "timeout", defaultTimeout, "overall timeout for LLM classification")
	flag.StringVar(&o.pipeline, "pipeline", "", "override pipeline name")
	flag.StringVar(&o.clusterName, "cluster-name", "", "scenario: cluster name")
	flag.StringVar(&o.clusterType, "cluster-type", "", "scenario: cluster type")
	flag.StringVar(&o.region, "region", "", "scenario: region")
	flag.StringVar(&o.osName, "os", "", "scenario: operating system (linux/windows)")
	flag.StringVar(&o.cni, "cni", "", "scenario: cni (cniv1/cniv2/cilium)")
	flag.StringVar(&o.knowledgeDB, "knowledge-db", os.Getenv("FAILURE_AGENT_DB"), "path to the SQLite knowledge store (or FAILURE_AGENT_DB); enables incident memory")
	flag.BoolVar(&o.live, "live", true, "collect read-only kubectl diagnostics from the retained cluster (requires kubectl + KUBECONFIG)")
	flag.BoolVar(&o.privileged, "privileged", true, "collect host-level logs via kubectl debug node (requires --live; creates ephemeral debug pods)")
	flag.StringVar(&o.flakinessOutput, "flakiness-output", "", "write the knowledge-store flakiness report to this path")
	flag.Parse()
	return o
}

// priorContextLimit caps how many prior incidents of each kind are injected.
const priorContextLimit = 3

func run(ctx context.Context, logger *zap.Logger, opts options, cl classifier, ks knowledgeStore, lc liveCollector, pc liveCollector) error {
	if opts.input == "" {
		return errors.New("--input is required")
	}

	rc := collect.FromEnv(os.Getenv)
	applyOverrides(&rc, opts)

	ev, err := collect.ParseEvidence(opts.input)
	if err != nil {
		return fmt.Errorf("parsing evidence: %w", err)
	}
	logger.Info("evidence collected",
		zap.Int("files", len(ev.Files)),
		zap.Int("errorLines", len(ev.TopErrorLines)),
	)

	if res := lc.Collect(ctx); len(res.Executed) > 0 {
		ev = live.Merge(ev, res)
		logger.Info("live diagnostics collected",
			zap.String("event", "live_evidence_collected"),
			zap.Int("commands", len(res.Executed)),
		)
		for _, argv := range res.Executed {
			name := live.CommandString(argv)
			logger.Info("live command executed", zap.String("command", name))
		}
		for label, output := range res.Outputs {
			// Log full output so pipeline raw logs have complete traceability.
			logger.Info("live diagnostic output",
				zap.String("diagnostic", label),
				zap.Int("bytes", len(output)),
				zap.String("output", output),
			)
		}
	}

	if res := pc.Collect(ctx); len(res.Executed) > 0 {
		ev = live.Merge(ev, res)
		logger.Info("privileged diagnostics collected",
			zap.String("event", "privileged_evidence_collected"),
			zap.Int("commands", len(res.Executed)),
		)
		for _, argv := range res.Executed {
			logger.Info("privileged command executed", zap.String("command", live.CommandString(argv)))
		}
		for label, output := range res.Outputs {
			logger.Info("privileged diagnostic output",
				zap.String("diagnostic", label),
				zap.Int("bytes", len(output)),
				zap.String("output", output),
			)
		}
	}

	fp := fingerprint.Compute(rc, ev)

	sigSet, err := loadSignatures(logger, opts.signaturesPath)
	if err != nil {
		return err
	}
	matches := sigSet.Match(rc, ev)

	// Skip duplicate work when an unresolved incident with the same fingerprint
	// already exists (e.g. a PR is already open for this failure).
	if active, err := ks.ActiveByFingerprint(ctx, fp.Hash); err != nil {
		logger.Warn("knowledge lookup failed; proceeding without dedupe", zap.Error(err))
	} else if active != nil {
		return handleDuplicate(ctx, logger, opts, rc, fp, matches, ev, ks, active)
	}

	prior := priorContext(ctx, logger, ks, fp.Hash)

	classifyCtx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	classification, classifyErr := cl.Classify(classifyCtx, rc, ev, fp, matches, prior)
	status := model.StatusAnalyzed
	if classifyErr != nil {
		logger.Error("llm classification failed",
			zap.String("event", "llm_failed"),
			zap.String("fingerprint", fp.Hash),
			zap.Error(classifyErr),
		)
		classification = analysisFailedClassification(ev, classifyErr)
		status = model.StatusAnalysisFailed
	}

	inc := report.Build(time.Now(), rc, fp, classification, matches, ev)
	inc.AnalysisStatus = status
	if classifyErr != nil {
		inc.AnalysisError = classifyErr.Error()
	}
	if err := report.WriteFiles(opts.output, inc); err != nil {
		return err
	}

	recordIncident(ctx, logger, ks, inc, status)

	logger.Info("incident written",
		zap.String("event", "incident_created"),
		zap.String("fingerprint", inc.Fingerprint),
		zap.String("status", string(status)),
		zap.String("category", string(inc.Category)),
		zap.String("confidenceBand", string(inc.ConfidenceBand)),
		zap.Float64("confidence", inc.Confidence),
		zap.String("source", classification.Source),
		zap.Int("signatureMatches", len(matches)),
		zap.String("report", filepath.Join(opts.output, report.MarkdownFile)),
	)

	if !opts.dryRun {
		if err := publishToPR(ctx, logger, rc, fp, inc); err != nil {
			logger.Warn("failed to publish analysis to pull request", zap.Error(err))
		}
	}
	return nil
}

// handleDuplicate is taken when an unresolved incident with the same fingerprint
// already exists. It skips the LLM call and PR write-back to avoid churn, records
// the recurrence on the existing incident, and still writes local artifacts.
func handleDuplicate(ctx context.Context, logger *zap.Logger, opts options, rc model.RunContext, fp model.Fingerprint, matches []model.SignatureMatch, ev model.Evidence, ks knowledgeStore, active *store.Incident) error {
	logger.Info("duplicate failure; skipping new analysis and pr write-back",
		zap.String("event", "duplicate_skipped"),
		zap.String("fingerprint", fp.Hash),
		zap.String("activeIncident", active.ID),
		zap.String("activeStatus", string(active.Status)),
	)
	if err := ks.AppendEvent(ctx, active.ID, "duplicate_skipped", "recurred in "+rc.PipelineName); err != nil {
		logger.Warn("failed to record duplicate recurrence", zap.Error(err))
	}

	classification := model.Classification{
		Category:         model.FailureCategory(active.Category),
		Confidence:       active.Confidence,
		RootCauseSummary: fmt.Sprintf("Duplicate of active incident %s (status=%s). Skipping new analysis and PR write-back.", active.ID, active.Status),
		ProposedFix:      active.ProposedFix,
		Source:           "duplicate",
	}
	inc := report.Build(time.Now(), rc, fp, classification, matches, ev)
	return report.WriteFiles(opts.output, inc)
}

// priorContext loads validated and in-flight prior incidents for a fingerprint
// and shapes them for prompt injection. Lookup failures degrade to empty context.
func priorContext(ctx context.Context, logger *zap.Logger, ks knowledgeStore, fingerprint string) classify.PriorContext {
	resolved, unresolved, err := ks.PriorByFingerprint(ctx, fingerprint, "", priorContextLimit)
	if err != nil {
		logger.Warn("prior knowledge lookup failed; proceeding without prior context", zap.Error(err))
		return classify.PriorContext{}
	}
	return classify.PriorContext{
		Resolved:   toPriorIncidents(resolved),
		Unresolved: toPriorIncidents(unresolved),
	}
}

func toPriorIncidents(rows []store.Incident) []classify.PriorIncident {
	out := make([]classify.PriorIncident, 0, len(rows))
	for _, r := range rows {
		out = append(out, classify.PriorIncident{
			Fingerprint: r.Fingerprint,
			Category:    r.Category,
			Summary:     r.Summary,
			Fix:         r.ProposedFix,
			Status:      string(r.Status),
		})
	}
	return out
}

// recordIncident persists the incident to the knowledge store. Persistence
// failures are logged but never fail the run.
func recordIncident(ctx context.Context, logger *zap.Logger, ks knowledgeStore, inc model.Incident, status model.AnalysisStatus) {
	st := store.StatusNew
	if status == model.StatusAnalysisFailed {
		st = store.StatusAnalysisFailed
	}
	id, err := ks.CreateIncident(ctx, store.Incident{
		Fingerprint: inc.Fingerprint,
		Pipeline:    inc.PipelineName,
		Category:    string(inc.Category),
		Confidence:  inc.Confidence,
		Summary:     inc.RootCauseSummary,
		ProposedFix: inc.ProposedFix,
		Status:      st,
	})
	if err != nil {
		logger.Warn("failed to record incident in knowledge store", zap.Error(err))
		return
	}
	if id != "" {
		logger.Info("incident recorded",
			zap.String("incidentId", id),
			zap.String("storeStatus", string(st)),
		)
	}
}

// classifier produces a Classification from collected evidence. main wires the
// Azure OpenAI-backed implementation; tests inject a fake.
type classifier interface {
	Classify(ctx context.Context, rc model.RunContext, ev model.Evidence, fp model.Fingerprint, matches []model.SignatureMatch, prior classify.PriorContext) (model.Classification, error)
}

// knowledgeStore is the subset of the SQL knowledge base that run depends on. A
// no-op implementation is used when no database is configured.
type knowledgeStore interface {
	ActiveByFingerprint(ctx context.Context, fingerprint string) (*store.Incident, error)
	PriorByFingerprint(ctx context.Context, fingerprint, excludeID string, limit int) (resolved, unresolved []store.Incident, err error)
	CreateIncident(ctx context.Context, inc store.Incident) (string, error)
	AppendEvent(ctx context.Context, incidentID, name, detail string) error
}

// liveCollector gathers read-only diagnostics from a retained cluster.
type liveCollector interface {
	Collect(ctx context.Context) live.Result
}

// noopStore is the knowledgeStore used when no database is configured. It records
// nothing and reports no prior incidents, so the agent runs statelessly.
type noopStore struct{}

func (noopStore) ActiveByFingerprint(context.Context, string) (*store.Incident, error) {
	return nil, nil
}

func (noopStore) PriorByFingerprint(context.Context, string, string, int) ([]store.Incident, []store.Incident, error) {
	return nil, nil, nil
}

func (noopStore) CreateIncident(context.Context, store.Incident) (string, error) { return "", nil }

func (noopStore) AppendEvent(context.Context, string, string, string) error { return nil }

// noopCollector is the liveCollector used when live diagnostics are disabled.
type noopCollector struct{}

func (noopCollector) Collect(context.Context) live.Result { return live.Result{} }

// sqliteDriver is the database/sql driver name for the pure-Go SQLite backend.
const sqliteDriver = "sqlite"

// flakinessReportLimit caps how many entries each flakiness section surfaces.
const flakinessReportLimit = 10

// buildCollector returns a kubectl-backed collector when live diagnostics are
// enabled, otherwise a no-op.
func buildCollector(logger *zap.Logger, opts options) liveCollector {
	if !opts.live {
		return noopCollector{}
	}
	logger.Info("live diagnostics enabled")
	return live.NewCollector(kubectlRunner{})
}

// buildPrivilegedCollector returns a privileged collector when both --live and
// --privileged are set, otherwise a no-op.
func buildPrivilegedCollector(logger *zap.Logger, opts options) liveCollector {
	if !opts.live || !opts.privileged {
		return noopCollector{}
	}
	logger.Info("privileged diagnostics enabled (will create ephemeral debug pods)")
	return live.NewPrivilegedCollector(privilegedRunner{})
}

// kubectlRunner executes read-only kubectl commands. It re-validates argv against
// the command policy as defense in depth before exec.
type kubectlRunner struct{}

func (kubectlRunner) Run(ctx context.Context, argv []string) (string, error) {
	if err := command.Validate(argv); err != nil {
		return "", err
	}
	out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).CombinedOutput()
	return string(out), err
}

// privilegedRunner executes kubectl commands including debug/exec. It validates
// through the privileged policy as defense in depth.
type privilegedRunner struct{}

func (privilegedRunner) Run(ctx context.Context, argv []string) (string, error) {
	if err := command.ValidatePrivileged(argv); err != nil {
		return "", err
	}
	out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).CombinedOutput()
	return string(out), err
}

// emitFlakiness writes the knowledge-store flakiness report when configured.
func emitFlakiness(ctx context.Context, logger *zap.Logger, opts options, s *store.Store) {
	if opts.flakinessOutput == "" {
		return
	}
	rep, err := s.Flakiness(ctx, flakinessReportLimit)
	if err != nil {
		logger.Warn("failed to compute flakiness report", zap.Error(err))
		return
	}
	if err := os.WriteFile(opts.flakinessOutput, []byte(store.RenderFlakiness(rep)), 0o644); err != nil {
		logger.Warn("failed to write flakiness report", zap.Error(err))
		return
	}
	logger.Info("flakiness report written",
		zap.String("path", opts.flakinessOutput),
		zap.Int("incidents", rep.TotalIncidents),
	)
}

// errorClassifier is a classifier that always fails. main returns it when the
// Azure OpenAI client cannot be configured so run still emits an analysis_failed
// incident instead of silently skipping analysis.
type errorClassifier struct{ err error }

func (e errorClassifier) Classify(context.Context, model.RunContext, model.Evidence, model.Fingerprint, []model.SignatureMatch, classify.PriorContext) (model.Classification, error) {
	return model.Classification{}, e.err
}

// buildClassifier returns the LLM classifier, or an errorClassifier carrying the
// configuration error when Azure OpenAI is not set up.
func buildClassifier(opts options) classifier {
	if opts.aoaiEndpoint == "" || opts.aoaiDeployment == "" || opts.aoaiAPIKey == "" {
		return errorClassifier{err: errors.New("azure openai endpoint, deployment, and api key are required for analysis")}
	}
	client, err := classify.NewAzureClient(opts.aoaiEndpoint, opts.aoaiDeployment, opts.aoaiAPIVersion, opts.aoaiAPIKey)
	if err != nil {
		return errorClassifier{err: fmt.Errorf("configuring azure openai client: %w", err)}
	}
	return classify.NewLLMClassifier(client)
}

// maxFailedEvidenceLines caps how many error lines are surfaced when analysis fails.
const maxFailedEvidenceLines = 5

// analysisFailedClassification builds the placeholder classification used when
// the LLM could not analyze the failure. It routes to human triage and carries
// the extracted error lines so the incident is still actionable.
func analysisFailedClassification(ev model.Evidence, err error) model.Classification {
	n := maxFailedEvidenceLines
	if len(ev.TopErrorLines) < n {
		n = len(ev.TopErrorLines)
	}
	top := make([]string, n)
	copy(top, ev.TopErrorLines[:n])

	return model.Classification{
		Category:         model.CategoryUnknownNeedsHuman,
		Confidence:       0,
		RootCauseSummary: fmt.Sprintf("Automated analysis was unavailable (%v). Raw evidence is preserved for human triage.", err),
		TopEvidence:      top,
		Source:           "none",
	}
}

// publishToPR upserts the analysis as a PR comment when the run is a pull
// request build and a write-capable GITHUB_TOKEN is available.
func publishToPR(ctx context.Context, logger *zap.Logger, rc model.RunContext, fp model.Fingerprint, inc model.Incident) error {
	if !rc.IsPR || rc.PullRequestNumber == "" {
		logger.Info("not a pull request build; skipping pr write-back")
		return nil
	}
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		logger.Info("GITHUB_TOKEN not set; skipping pr write-back")
		return nil
	}

	owner, repoName, ok := publish.ParseRepo(rc.Repository)
	if !ok {
		return fmt.Errorf("cannot parse owner/repo from %q", rc.Repository)
	}
	prNum, err := strconv.Atoi(rc.PullRequestNumber)
	if err != nil {
		return fmt.Errorf("invalid pull request number %q: %w", rc.PullRequestNumber, err)
	}

	store, err := publish.NewGitHubCommentStore(publish.GitHubConfig{
		Token:    token,
		Owner:    owner,
		Repo:     repoName,
		PRNumber: prNum,
	})
	if err != nil {
		return err
	}

	pubCtx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()

	action, err := publish.Upsert(pubCtx, store, report.CommentMarker(fp.Hash), report.RenderMarkdown(inc))
	if err != nil {
		return err
	}
	logger.Info("published analysis to pull request", zap.String("action", action), zap.Int("pr", prNum))
	return nil
}

func applyOverrides(rc *model.RunContext, opts options) {
	if opts.pipeline != "" {
		rc.PipelineName = opts.pipeline
	}
	if opts.clusterName != "" {
		rc.ClusterName = opts.clusterName
	}
	if opts.clusterType != "" {
		rc.ClusterType = opts.clusterType
	}
	if opts.region != "" {
		rc.Region = opts.region
	}
	if opts.osName != "" {
		rc.OS = opts.osName
	}
	if opts.cni != "" {
		rc.CNI = opts.cni
	}
}

func loadSignatures(logger *zap.Logger, path string) (*signatures.Set, error) {
	set, err := signatures.LoadFile(path)
	if err == nil {
		return set, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		logger.Warn("signatures file not found; continuing with no signatures", zap.String("path", path))
		return signatures.Load(strings.NewReader(""))
	}
	return nil, err
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
