// Command failure-agent analyzes a failed ACN pipeline run. It parses the
// collected log bundle, fingerprints the failure, matches known signatures,
// classifies the likely root cause, writes report.md + incident.json, and
// (for PR builds) posts the analysis back to the pull request.
//
// --dry-run performs deterministic-only analysis (no LLM, no write-back). The
// live path uses an Azure OpenAI deployment for classification.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/classify"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/collect"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/fingerprint"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/publish"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/report"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/signatures"
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
	aoaiAPIVersion string
	timeout        time.Duration

	pipeline    string
	clusterName string
	clusterType string
	region      string
	osName      string
	cni         string
}

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to init logger:", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	opts := parseFlags()
	if err := run(logger, opts); err != nil {
		logger.Error("failure analysis failed", zap.Error(err))
		os.Exit(1)
	}
}

func parseFlags() options {
	var o options
	flag.StringVar(&o.input, "input", "", "path to the collected evidence/log bundle directory (required)")
	flag.StringVar(&o.output, "output", ".", "directory to write report.md and incident.json")
	flag.StringVar(&o.signaturesPath, "signatures", defaultSignaturesPath, "path to the signatures catalog")
	flag.BoolVar(&o.dryRun, "dry-run", false, "deterministic analysis only (no LLM, no PR write-back)")
	flag.StringVar(&o.aoaiEndpoint, "aoai-endpoint", os.Getenv("AZURE_OPENAI_ENDPOINT"), "Azure OpenAI endpoint (or AZURE_OPENAI_ENDPOINT)")
	flag.StringVar(&o.aoaiDeployment, "aoai-deployment", os.Getenv("AZURE_OPENAI_DEPLOYMENT"), "Azure OpenAI deployment name (or AZURE_OPENAI_DEPLOYMENT)")
	flag.StringVar(&o.aoaiAPIVersion, "aoai-api-version", defaultAOAIAPIVersion, "Azure OpenAI API version")
	flag.DurationVar(&o.timeout, "timeout", defaultTimeout, "overall timeout for LLM classification")
	flag.StringVar(&o.pipeline, "pipeline", "", "override pipeline name")
	flag.StringVar(&o.clusterName, "cluster-name", "", "scenario: cluster name")
	flag.StringVar(&o.clusterType, "cluster-type", "", "scenario: cluster type")
	flag.StringVar(&o.region, "region", "", "scenario: region")
	flag.StringVar(&o.osName, "os", "", "scenario: operating system (linux/windows)")
	flag.StringVar(&o.cni, "cni", "", "scenario: cni (cniv1/cniv2/cilium)")
	flag.Parse()
	return o
}

func run(logger *zap.Logger, opts options) error {
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

	fp := fingerprint.Compute(rc, ev)

	sigSet, err := loadSignatures(logger, opts.signaturesPath)
	if err != nil {
		return err
	}
	matches := sigSet.Match(rc, ev)

	classification, err := classifyFailure(logger, opts, rc, ev, fp, matches)
	if err != nil {
		return err
	}

	inc := report.Build(time.Now(), rc, fp, classification, matches, ev)
	if err := report.WriteFiles(opts.output, inc); err != nil {
		return err
	}

	logger.Info("analysis complete",
		zap.String("fingerprint", inc.Fingerprint),
		zap.String("category", string(inc.Category)),
		zap.String("confidenceBand", string(inc.ConfidenceBand)),
		zap.Float64("confidence", inc.Confidence),
		zap.String("source", classification.Source),
		zap.Int("signatureMatches", len(matches)),
		zap.String("report", filepath.Join(opts.output, report.MarkdownFile)),
	)

	if !opts.dryRun {
		if err := publishToPR(logger, rc, fp, inc); err != nil {
			logger.Warn("failed to publish analysis to pull request", zap.Error(err))
		}
	}
	return nil
}

// classifyFailure runs the deterministic classifier in dry-run mode, otherwise
// the Azure OpenAI-backed classifier. A live-mode LLM failure is fatal.
func classifyFailure(logger *zap.Logger, opts options, rc model.RunContext, ev model.Evidence, fp model.Fingerprint, matches []model.SignatureMatch) (model.Classification, error) {
	if opts.dryRun {
		logger.Info("dry-run: using deterministic classification")
		return classify.Deterministic(rc, ev, matches), nil
	}

	if opts.aoaiEndpoint == "" || opts.aoaiDeployment == "" {
		return model.Classification{}, errors.New("live mode requires --aoai-endpoint and --aoai-deployment (or AZURE_OPENAI_* env); use --dry-run for deterministic-only analysis")
	}

	client, err := classify.NewAzureClient(opts.aoaiEndpoint, opts.aoaiDeployment, opts.aoaiAPIVersion)
	if err != nil {
		return model.Classification{}, fmt.Errorf("creating azure openai client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()

	classification, err := classify.NewLLMClassifier(client).Classify(ctx, rc, ev, fp, matches)
	if err != nil {
		return model.Classification{}, fmt.Errorf("llm classification: %w", err)
	}
	logger.Info("llm classification complete",
		zap.String("category", string(classification.Category)),
		zap.Float64("confidence", classification.Confidence),
	)
	return classification, nil
}

// publishToPR upserts the analysis as a PR comment when the run is a pull
// request build and a write-capable GITHUB_TOKEN is available.
func publishToPR(logger *zap.Logger, rc model.RunContext, fp model.Fingerprint, inc model.Incident) error {
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

	ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
	defer cancel()

	action, err := publish.Upsert(ctx, store, report.CommentMarker(fp.Hash), report.RenderMarkdown(inc))
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
