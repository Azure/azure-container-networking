package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/classify"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/collect"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/fingerprint"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/live"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/store"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	_ "modernc.org/sqlite"
)

// fakeClassifier returns a canned classification (or error) so the pipeline can
// be exercised end to end without Azure OpenAI credentials. It records the prior
// context it was given so tests can assert knowledge injection.
type fakeClassifier struct {
	result      model.Classification
	err         error
	gotContext  model.RunContext
	gotPrior    classify.PriorContext
	gotEvidence model.Evidence
	callCount   int
}

func (f *fakeClassifier) Classify(_ context.Context, rc model.RunContext, ev model.Evidence, _ model.Fingerprint, _ []model.SignatureMatch, prior classify.PriorContext) (model.Classification, error) {
	f.callCount++
	f.gotContext = rc
	f.gotEvidence = ev
	f.gotPrior = prior
	return f.result, f.err
}

type priorKnowledgeStore struct {
	noopStore
	resolved []store.Incident
}

func (s priorKnowledgeStore) PriorByFingerprint(context.Context, string, string, int) ([]store.Incident, []store.Incident, error) {
	return s.resolved, nil, nil
}

type staticCollector struct {
	result live.Result
}

func (c staticCollector) Collect(context.Context) live.Result {
	return c.result
}

// TestRunEndToEnd exercises collect -> fingerprint -> signatures -> classify ->
// report over a committed evidence bundle using a fake LLM classifier.
func TestRunEndToEnd(t *testing.T) {
	out := t.TempDir()
	cl := &fakeClassifier{result: model.Classification{
		Category:         model.CategoryClusterBringupFailure,
		Confidence:       0.82,
		RootCauseSummary: "Cluster bring-up failed; CNS pods never became ready.",
		TopEvidence:      []string{"ImagePullBackOff azure-cns"},
		RecommendedOwner: "acn-cni",
		ProposedFix:      "Re-check the CNS image reference.",
		Source:           "llm",
	}}
	opts := options{
		input:          writeEvidenceBundle(t),
		output:         out,
		signaturesPath: filepath.Join("signatures", "signatures.yaml"),
		dryRun:         true,
		pipeline:       "ACN PR",
		clusterType:    "overlay-byocni-up",
		osName:         "linux",
		cni:            "cilium",
	}

	if err := run(context.Background(), zap.NewNop(), opts, cl, noopStore{}, noopCollector{}, noopCollector{}); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	inc := readIncident(t, out)
	if inc.Category != model.CategoryClusterBringupFailure {
		t.Errorf("category: got %s, want %s", inc.Category, model.CategoryClusterBringupFailure)
	}
	if inc.AnalysisStatus != model.StatusAnalyzed {
		t.Errorf("status: got %s, want %s", inc.AnalysisStatus, model.StatusAnalyzed)
	}
	if inc.ClassificationSource != "llm" {
		t.Errorf("source: got %s, want llm", inc.ClassificationSource)
	}
	if inc.Fingerprint == "" {
		t.Error("expected non-empty fingerprint")
	}
	if len(inc.SignatureMatches) == 0 {
		t.Error("expected at least one signature match")
	}

	md, err := os.ReadFile(filepath.Join(out, "report.md"))
	if err != nil {
		t.Fatalf("reading report.md: %v", err)
	}
	if !strings.Contains(string(md), "acn-failure-agent:"+inc.Fingerprint) {
		t.Error("expected fingerprint marker in report.md")
	}
}

func TestRunRequiresInput(t *testing.T) {
	if err := run(context.Background(), zap.NewNop(), options{dryRun: true}, &fakeClassifier{}, noopStore{}, noopCollector{}, noopCollector{}); err == nil {
		t.Fatal("expected error when --input missing")
	}
}

func TestAOAIAPIKeyIsNotRenderedInFlagDefaults(t *testing.T) {
	const secret = "sentinel-azure-ai-key"
	t.Setenv("AZURE_OPENAI_API_KEY", secret)
	var o options
	fs := flag.NewFlagSet("failure-agent", flag.ContinueOnError)
	var output bytes.Buffer
	fs.SetOutput(&output)
	registerFlags(fs, &o, os.Getenv)

	fs.PrintDefaults()

	if strings.Contains(output.String(), secret) {
		t.Fatal("Azure OpenAI API key was rendered in flag defaults")
	}
}

func TestResolveAOAIAPIKeyPreservesExplicitFlagPrecedence(t *testing.T) {
	const envKey = "environment-key"
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "omitted uses environment", want: envKey},
		{name: "explicit empty wins", args: []string{"--aoai-api-key="}, want: ""},
		{name: "explicit value wins", args: []string{"--aoai-api-key=flag-key"}, want: "flag-key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var o options
			fs := flag.NewFlagSet("failure-agent", flag.ContinueOnError)
			registerFlags(fs, &o, func(name string) string {
				if name == "AZURE_OPENAI_API_KEY" {
					return envKey
				}
				return ""
			})
			if err := fs.Parse(tt.args); err != nil {
				t.Fatalf("parsing flags: %v", err)
			}
			resolveAOAIAPIKey(fs, &o, func(string) string { return envKey })
			if o.aoaiAPIKey != tt.want {
				t.Fatalf("API key: got %q, want %q", o.aoaiAPIKey, tt.want)
			}
		})
	}
}

func TestConfiguredSecretsIncludesOverriddenEnvironmentKey(t *testing.T) {
	const (
		flagKey = "flag-api-key"
		envKey  = "environment-api-key"
	)
	secrets := configuredSecrets(options{aoaiAPIKey: flagKey}, func(name string) string {
		if name == "AZURE_OPENAI_API_KEY" {
			return envKey
		}
		return ""
	})
	got := strings.Join(secrets, " ")
	if !strings.Contains(got, flagKey) || !strings.Contains(got, envKey) {
		t.Fatalf("configured secrets %q do not include both API key sources", got)
	}
}

func TestRunRedactsConfiguredSecretsFromEvidenceAndArtifacts(t *testing.T) {
	tests := []struct {
		name      string
		configure func(t *testing.T, opts *options, secret string)
	}{
		{
			name: "Azure OpenAI API key",
			configure: func(_ *testing.T, opts *options, secret string) {
				opts.aoaiAPIKey = secret
			},
		},
		{
			name: "GitHub token under workload identity",
			configure: func(t *testing.T, _ *options, secret string) {
				t.Setenv("GITHUB_TOKEN", secret)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const secret = "sentinel-pipeline-secret"
			input := t.TempDir()
			if err := os.WriteFile(filepath.Join(input, "task-"+secret+".log"), []byte("Error: request failed with secret "+secret), 0o600); err != nil {
				t.Fatalf("writing evidence: %v", err)
			}
			out := t.TempDir()
			cl := &fakeClassifier{err: errors.New("classification failed for " + secret)}
			opts := options{
				input:          input,
				output:         out,
				signaturesPath: filepath.Join("signatures", "signatures.yaml"),
				dryRun:         true,
			}
			tt.configure(t, &opts, secret)

			if err := run(context.Background(), zap.NewNop(), opts, cl, noopStore{}, noopCollector{}, noopCollector{}); err != nil {
				t.Fatalf("run failed: %v", err)
			}

			if strings.Contains(fmt.Sprintf("%#v", cl.gotEvidence), secret) {
				t.Fatal("classifier received a configured secret in evidence")
			}
			for _, name := range []string{"report.md", "incident.json"} {
				data, err := os.ReadFile(filepath.Join(out, name))
				if err != nil {
					t.Fatalf("reading %s: %v", name, err)
				}
				if strings.Contains(string(data), secret) {
					t.Fatalf("%s contains a configured secret", name)
				}
			}
		})
	}
}

func TestRunRedactsAllClassifierAndLoggingBoundaries(t *testing.T) {
	const secret = "sentinel-boundary-secret"
	t.Setenv("GITHUB_TOKEN", secret)

	input := t.TempDir()
	if err := os.WriteFile(filepath.Join(input, "error-"+secret+".log"), []byte("Error: evidence contains "+secret), 0o600); err != nil {
		t.Fatalf("writing evidence: %v", err)
	}
	diffFile := filepath.Join(t.TempDir(), "change.diff")
	diff := "diff --git a/file b/file\n--- a/file\n+++ b/file\n@@ -1 +1 @@\n-old\n+" + secret + "\n"
	if err := os.WriteFile(diffFile, []byte(diff), 0o600); err != nil {
		t.Fatalf("writing diff: %v", err)
	}

	cl := &fakeClassifier{result: model.Classification{
		Category:         model.CategoryPipelineInfraConfig,
		Confidence:       0.8,
		RootCauseSummary: "model output " + secret,
		TopEvidence:      []string{"model evidence " + secret},
		CausalChain: []model.CausalHop{{
			Step:     "model step " + secret,
			Citation: "model citation " + secret,
		}},
		Source: "llm",
	}}
	ks := priorKnowledgeStore{resolved: []store.Incident{{
		Fingerprint: "prior-" + secret,
		Category:    string(model.CategoryPipelineInfraConfig),
		Summary:     "prior summary " + secret,
		ProposedFix: "prior fix " + secret,
		Status:      store.StatusValidatedResolved,
	}}}
	collector := staticCollector{result: live.Result{
		Executed: [][]string{{"kubectl", "get", "pods"}},
		Outputs:  map[string]string{"diagnostic-" + secret: "live output " + secret},
	}}
	core, observed := observer.New(zap.DebugLevel)
	out := t.TempDir()
	opts := options{
		input:          input,
		output:         out,
		signaturesPath: filepath.Join("signatures", "signatures.yaml"),
		diffFile:       diffFile,
		dryRun:         true,
	}

	if err := run(context.Background(), zap.New(core), opts, cl, ks, collector, noopCollector{}); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	for name, value := range map[string]any{
		"run context": cl.gotContext,
		"evidence":    cl.gotEvidence,
		"prior":       cl.gotPrior,
		"logs":        observed.All(),
	} {
		if strings.Contains(fmt.Sprintf("%#v", value), secret) {
			t.Fatalf("%s contains configured secret", name)
		}
	}
	for _, name := range []string{"report.md", "incident.json"} {
		data, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		if strings.Contains(string(data), secret) {
			t.Fatalf("%s contains configured secret", name)
		}
	}
}

func TestRunWeeklyRedactsPriorIncidentAndOutput(t *testing.T) {
	const secret = "sentinel-weekly-secret"
	t.Setenv("GITHUB_TOKEN", secret)
	input := t.TempDir()
	incidentDir := filepath.Join(input, "artifact")
	if err := os.MkdirAll(incidentDir, 0o755); err != nil {
		t.Fatalf("creating incident directory: %v", err)
	}
	data, err := json.Marshal(model.Incident{
		GeneratedAt:      time.Now(),
		PipelineName:     "pipeline-" + secret,
		Fingerprint:      "fingerprint-" + secret,
		Category:         model.CategoryPipelineInfraConfig,
		RootCauseSummary: "weekly source " + secret,
		AnalysisStatus:   model.StatusAnalyzed,
	})
	if err != nil {
		t.Fatalf("marshaling incident: %v", err)
	}
	if err := os.WriteFile(filepath.Join(incidentDir, "incident.json"), data, 0o600); err != nil {
		t.Fatalf("writing incident: %v", err)
	}
	out := t.TempDir()

	if err := runWeekly(context.Background(), zap.NewNop(), options{
		weeklyReport: input,
		weeklyWindow: 7,
		output:       out,
	}); err != nil {
		t.Fatalf("running weekly aggregation: %v", err)
	}

	for _, name := range []string{"weekly-report.md", "weekly-incident.json"} {
		output, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		if strings.Contains(string(output), secret) {
			t.Fatalf("%s contains configured secret", name)
		}
	}
}

// TestRunAnalysisFailedWhenLLMFails verifies that an LLM failure still produces
// an analysis_failed incident carrying the raw evidence, rather than aborting.
func TestRunAnalysisFailedWhenLLMFails(t *testing.T) {
	out := t.TempDir()
	cl := &fakeClassifier{err: errors.New("azure openai unauthorized")}
	opts := options{
		input:          writeEvidenceBundle(t),
		output:         out,
		signaturesPath: filepath.Join("signatures", "signatures.yaml"),
		dryRun:         true,
	}
	if err := run(context.Background(), zap.NewNop(), opts, cl, noopStore{}, noopCollector{}, noopCollector{}); err != nil {
		t.Fatalf("run should not fail on LLM error: %v", err)
	}

	inc := readIncident(t, out)
	if inc.AnalysisStatus != model.StatusAnalysisFailed {
		t.Errorf("status: got %s, want %s", inc.AnalysisStatus, model.StatusAnalysisFailed)
	}
	if inc.Category != model.CategoryUnknownNeedsHuman {
		t.Errorf("category: got %s, want %s", inc.Category, model.CategoryUnknownNeedsHuman)
	}
	if inc.ClassificationSource != "none" {
		t.Errorf("source: got %s, want none", inc.ClassificationSource)
	}
	if inc.AnalysisError == "" {
		t.Error("expected analysisError to be populated")
	}
	if len(inc.EvidenceFiles) == 0 {
		t.Error("expected evidence files to be preserved on analysis failure")
	}
}

// TestBuildClassifierWithoutCredentials returns an errorClassifier that surfaces
// the misconfiguration as a Classify error.
func TestBuildClassifierWithoutCredentials(t *testing.T) {
	cl := buildClassifier(options{})
	if _, err := cl.Classify(context.Background(), model.RunContext{}, model.Evidence{}, model.Fingerprint{}, nil, classify.PriorContext{}); err == nil {
		t.Fatal("expected error from classifier built without credentials")
	}
}

func bundleOptions(t *testing.T, out string) options {
	t.Helper()
	return options{
		input:          writeEvidenceBundle(t),
		output:         out,
		signaturesPath: filepath.Join("signatures", "signatures.yaml"),
		dryRun:         true,
	}
}

func writeEvidenceBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"nodes.txt": `Name:               aks-nodepool1-12345678-vmss000000
Conditions:
  Type             Status
  Ready            True
  MemoryPressure   False
  DiskPressure     False
System Info:
  Kernel Version:  5.15.0-1071-azure
  Container Runtime Version: containerd://1.7.20
`,
		"pods.txt": `NAMESPACE     NAME                          READY   STATUS             RESTARTS   AGE
kube-system   azure-cns-abcd12              0/1     ImagePullBackOff   0          3m
kube-system   coredns-7f9c8b6d4-xz12        1/1     Running            0          10m
kube-system   azure-cni-node-qq88           1/1     Running            0          10m

Warning  Failed     kubelet  Failed to pull image "acnpublic.azurecr.io/azure-cns:bad-tag": ErrImagePull
Warning  Failed     kubelet  Error: ImagePullBackOff
`,
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
			t.Fatalf("writing evidence file %s: %v", name, err)
		}
	}
	return dir
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(context.Background(), "sqlite", filepath.Join(t.TempDir(), "knowledge.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func bundleFingerprint(t *testing.T, opts options) string {
	t.Helper()
	rc := collect.FromEnv(os.Getenv)
	applyOverrides(&rc, opts)
	ev, err := collect.ParseEvidence(opts.input)
	if err != nil {
		t.Fatalf("parsing evidence: %v", err)
	}
	return fingerprint.Compute(rc, ev).Hash
}

// TestRunSkipsDuplicateFailure verifies that a second run of the same failure,
// while an incident is still active, skips the LLM call and PR write-back and
// records a duplicate event instead of creating a new incident.
func TestRunSkipsDuplicateFailure(t *testing.T) {
	st := openTestStore(t)
	cl := &fakeClassifier{result: model.Classification{
		Category:         model.CategoryClusterBringupFailure,
		Confidence:       0.8,
		RootCauseSummary: "first analysis",
		Source:           "llm",
	}}

	opts := bundleOptions(t, t.TempDir())
	if err := run(context.Background(), zap.NewNop(), opts, cl, st, noopCollector{}, noopCollector{}); err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	if cl.callCount != 1 {
		t.Fatalf("first run should classify once, got %d", cl.callCount)
	}

	opts.output = t.TempDir()
	if err := run(context.Background(), zap.NewNop(), opts, cl, st, noopCollector{}, noopCollector{}); err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if cl.callCount != 1 {
		t.Errorf("duplicate run should not re-classify, got %d calls", cl.callCount)
	}

	inc := readIncident(t, opts.output)
	if inc.ClassificationSource != "duplicate" {
		t.Errorf("source: got %s, want duplicate", inc.ClassificationSource)
	}
}

// TestRunInjectsPriorResolvedContext verifies that a validated prior resolution
// for the same fingerprint is injected into the classifier prompt context.
func TestRunInjectsPriorResolvedContext(t *testing.T) {
	st := openTestStore(t)
	opts := bundleOptions(t, t.TempDir())
	fp := bundleFingerprint(t, opts)

	id, err := st.CreateIncident(context.Background(), store.Incident{
		Fingerprint: fp,
		Category:    string(model.CategoryClusterBringupFailure),
		Summary:     "earlier validated resolution",
		ProposedFix: "bump the CNS image tag",
		Status:      store.StatusNew,
	})
	if err != nil {
		t.Fatalf("seeding incident: %v", err)
	}
	if err := st.UpdateStatus(context.Background(), id, store.StatusValidatedResolved); err != nil {
		t.Fatalf("resolving incident: %v", err)
	}

	cl := &fakeClassifier{result: model.Classification{
		Category: model.CategoryClusterBringupFailure, Confidence: 0.7, Source: "llm",
	}}
	if err := run(context.Background(), zap.NewNop(), opts, cl, st, noopCollector{}, noopCollector{}); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if len(cl.gotPrior.Resolved) != 1 {
		t.Fatalf("expected 1 resolved prior, got %d", len(cl.gotPrior.Resolved))
	}
	if cl.gotPrior.Resolved[0].Fix != "bump the CNS image tag" {
		t.Errorf("prior fix: got %q", cl.gotPrior.Resolved[0].Fix)
	}
}

// fakeRunner returns canned output so the live collector can run without a cluster.
type fakeRunner struct{}

func (fakeRunner) Run(_ context.Context, argv []string) (string, error) {
	return "fake output for " + strings.Join(argv, " "), nil
}

// TestRunMergesLiveDiagnostics verifies that live diagnostics are folded into the
// evidence surfaced in the incident.
func TestRunMergesLiveDiagnostics(t *testing.T) {
	cl := &fakeClassifier{result: model.Classification{
		Category: model.CategoryClusterBringupFailure, Confidence: 0.7, Source: "llm",
	}}
	opts := bundleOptions(t, t.TempDir())
	collector := live.NewCollector(fakeRunner{})

	if err := run(context.Background(), zap.NewNop(), opts, cl, noopStore{}, collector, noopCollector{}); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	inc := readIncident(t, opts.output)
	var sawLive bool
	for _, f := range inc.EvidenceFiles {
		if strings.HasPrefix(f, "live/") {
			sawLive = true
			break
		}
	}
	if !sawLive {
		t.Errorf("expected a live/ evidence file, got %v", inc.EvidenceFiles)
	}
}

func readIncident(t *testing.T, dir string) model.Incident {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "incident.json"))
	if err != nil {
		t.Fatalf("reading incident.json: %v", err)
	}
	var inc model.Incident
	if err := json.Unmarshal(data, &inc); err != nil {
		t.Fatalf("unmarshaling incident: %v", err)
	}
	return inc
}
