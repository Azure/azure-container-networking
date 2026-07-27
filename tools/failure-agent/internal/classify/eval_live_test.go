package classify

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/collect"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

// TestGoldenCaseLiveEval runs the golden regression case against a REAL Azure
// OpenAI deployment. It is skipped unless FAA_LIVE_EVAL=1 and the AOAI
// environment is configured, so it never runs in normal CI. This is the true
// behavioral check that the investigation policy produces the infra verdict
// (not a CNS pr_regression) on the golden bundle.
//
// Run it with:
//
//	FAA_LIVE_EVAL=1 \
//	AZURE_OPENAI_ENDPOINT=... AZURE_OPENAI_DEPLOYMENT=... AZURE_OPENAI_API_KEY=... \
//	go test ./internal/classify -run TestGoldenCaseLiveEval -v
//
// Acceptance (mirrors the handoff spec's golden test): the verdict must NOT be
// pr_regression, must name the failing unit (the init installer), must label the
// CNS connection-refused/AllocateIPConfig signal a symptom, and must include a
// falsification step.
func TestGoldenCaseLiveEval(t *testing.T) {
	if os.Getenv("FAA_LIVE_EVAL") != "1" {
		t.Skip("set FAA_LIVE_EVAL=1 and AOAI env to run the live golden eval")
	}
	endpoint := os.Getenv("AZURE_OPENAI_ENDPOINT")
	deployment := os.Getenv("AZURE_OPENAI_DEPLOYMENT")
	apiKey := os.Getenv("AZURE_OPENAI_API_KEY")
	apiVersion := os.Getenv("AZURE_OPENAI_API_VERSION")
	if apiVersion == "" {
		apiVersion = "2024-10-21"
	}
	if endpoint == "" || deployment == "" || apiKey == "" {
		t.Skip("AZURE_OPENAI_ENDPOINT, AZURE_OPENAI_DEPLOYMENT, and AZURE_OPENAI_API_KEY are required")
	}

	client, err := NewAzureClient(endpoint, deployment, apiVersion, apiKey)
	if err != nil {
		t.Fatalf("building azure client: %v", err)
	}

	bundle := filepath.Join("..", "..", "testdata", "golden-defender-crashloop")
	ev, err := collect.ParseEvidence(bundle)
	if err != nil {
		t.Fatalf("parsing golden bundle: %v", err)
	}

	rc := model.RunContext{
		PipelineName: "ACN PR",
		StageName:    "Cilium Podsubnet / DualStack Overlay datapath",
		OS:           "linux",
		CNI:          "cilium",
		IsPR:         true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	got, err := NewLLMClassifier(client).Classify(ctx, rc, ev, model.Fingerprint{Hash: "golden"}, nil, PriorContext{})
	if err != nil {
		t.Fatalf("live classify: %v", err)
	}

	t.Logf("verdict: category=%s confidence=%.2f failingUnit=%q", got.Category, got.Confidence, got.FailingUnit)
	t.Logf("rootCause: %s", got.RootCauseSummary)

	if got.Category == model.CategoryPRRegression {
		t.Errorf("golden case misrouted to pr_regression: %s", got.RootCauseSummary)
	}
	if strings.TrimSpace(got.FailingUnit) == "" {
		t.Error("expected a named failing unit")
	}
	var symptomLabeled bool
	for _, s := range got.SymptomVsCause {
		sig := strings.ToLower(s.Signal)
		if (strings.Contains(sig, "connection refused") || strings.Contains(sig, "allocateipconfig") || strings.Contains(sig, "cns")) && s.Classification == "symptom" {
			symptomLabeled = true
		}
	}
	if !symptomLabeled {
		t.Errorf("expected the CNS connection-refused/AllocateIPConfig signal labeled a symptom, got %+v", got.SymptomVsCause)
	}
	if got.Falsification == nil || strings.TrimSpace(got.Falsification.Outcome) == "" {
		t.Errorf("expected a falsification step with an outcome, got %+v", got.Falsification)
	}
}
