package classify

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

// scriptedCompleter returns a queued response per call and records each call so
// tests can assert on call count and prompt content across the deepen pass.
type scriptedCompleter struct {
	responses []string
	calls     []completerCall
}

type completerCall struct {
	system string
	user   string
	schema *Schema
}

func (s *scriptedCompleter) Complete(_ context.Context, system, user string, schema *Schema) (string, error) {
	i := len(s.calls)
	s.calls = append(s.calls, completerCall{system: system, user: user, schema: schema})
	if i >= len(s.responses) {
		return "", fmt.Errorf("unexpected completer call %d", i)
	}
	return s.responses[i], nil
}

// mapProvider serves a fixed allow-list of evidence ids.
type mapProvider struct{ m map[string]string }

func (p mapProvider) Provide(_ context.Context, id string) (string, bool) {
	v, ok := p.m[id]
	return v, ok
}

const (
	specificFix = "Add a nil check in cns/restserver/ipam.go before dereferencing nc, then run `go test ./cns/restserver/...` to verify."
	genericFix  = "rerun"
)

func weakResponse(fix string, request ...string) string {
	reqs := ""
	for i, r := range request {
		if i > 0 {
			reqs += ","
		}
		reqs += fmt.Sprintf("%q", r)
	}
	return fmt.Sprintf(`{
		"category": "pr_regression",
		"confidence": 0.5,
		"rootCauseSummary": "the change under test looks implicated",
		"topEvidence": ["panic"],
		"recommendedOwner": "acn-cni",
		"proposedFix": %q,
		"evidenceRequest": [%s]
	}`, fix, reqs)
}

func strongResponse(conf float64) string {
	return fmt.Sprintf(`{
		"category": "pr_regression",
		"confidence": %.2f,
		"rootCauseSummary": "the change removed a required nil guard",
		"topEvidence": ["panic: nil pointer"],
		"recommendedOwner": "acn-cni",
		"proposedFix": %q,
		"evidenceRequest": []
	}`, conf, specificFix)
}

func markerRC() model.RunContext {
	return model.RunContext{PipelineName: "PIPE-MARKER"}
}

func TestClassifyNoDeepenWhenConfidentAndSpecific(t *testing.T) {
	sc := &scriptedCompleter{responses: []string{strongResponse(0.9)}}
	prov := mapProvider{m: map[string]string{"source:cns/restserver/ipam.go": "FULL_SOURCE_MARKER"}}

	got, err := NewLLMClassifier(sc).WithEvidence(prov).
		Classify(context.Background(), markerRC(), model.Evidence{}, model.Fingerprint{}, nil, PriorContext{})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if len(sc.calls) != 1 {
		t.Fatalf("expected exactly 1 completer call, got %d", len(sc.calls))
	}
	if got.Confidence != 0.9 {
		t.Errorf("confidence: got %v, want 0.9", got.Confidence)
	}
}

func TestClassifyDeepensOnceWhenWeak(t *testing.T) {
	sc := &scriptedCompleter{responses: []string{
		weakResponse(genericFix, "source:cns/restserver/ipam.go"),
		strongResponse(0.92),
	}}
	prov := mapProvider{m: map[string]string{"source:cns/restserver/ipam.go": "FULL_SOURCE_MARKER"}}

	got, err := NewLLMClassifier(sc).WithEvidence(prov).
		Classify(context.Background(), markerRC(), model.Evidence{}, model.Fingerprint{}, nil, PriorContext{})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if len(sc.calls) != 2 {
		t.Fatalf("expected exactly 2 completer calls (one deepen), got %d", len(sc.calls))
	}
	if got.Confidence != 0.92 {
		t.Errorf("expected final answer from the deepen pass (0.92), got %v", got.Confidence)
	}

	first, second := sc.calls[0].user, sc.calls[1].user
	if !strings.Contains(first, "PIPE-MARKER") || !strings.Contains(second, "PIPE-MARKER") {
		t.Error("expected the full original context to be re-sent on the deepen call")
	}
	if strings.Contains(first, "Additional evidence you requested") {
		t.Error("first call must not contain deepen evidence")
	}
	if !strings.Contains(second, "Additional evidence you requested") {
		t.Error("deepen call must contain the requested-evidence section")
	}
	if !strings.Contains(second, "FULL_SOURCE_MARKER") {
		t.Error("deepen call must include the provider-served source content")
	}
}

func TestClassifyNoDeepenWithoutProvider(t *testing.T) {
	sc := &scriptedCompleter{responses: []string{weakResponse(genericFix, "source:cns/restserver/ipam.go")}}

	got, err := NewLLMClassifier(sc). // no WithEvidence
						Classify(context.Background(), markerRC(), model.Evidence{}, model.Fingerprint{}, nil, PriorContext{})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if len(sc.calls) != 1 {
		t.Fatalf("without a provider there must be no deepen call; got %d calls", len(sc.calls))
	}
	if got.Confidence != 0.5 {
		t.Errorf("expected the single-pass answer (0.5), got %v", got.Confidence)
	}
}

func TestClassifyNoDeepenWhenRequestUnsatisfiable(t *testing.T) {
	sc := &scriptedCompleter{responses: []string{weakResponse(genericFix, "source:not-a-changed-file.go")}}
	prov := mapProvider{m: map[string]string{"source:cns/restserver/ipam.go": "FULL_SOURCE_MARKER"}}

	got, err := NewLLMClassifier(sc).WithEvidence(prov).
		Classify(context.Background(), markerRC(), model.Evidence{}, model.Fingerprint{}, nil, PriorContext{})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if len(sc.calls) != 1 {
		t.Fatalf("provider served nothing, so no deepen call should occur; got %d", len(sc.calls))
	}
	if got.Confidence != 0.5 {
		t.Errorf("expected the single-pass answer (0.5), got %v", got.Confidence)
	}
}

func TestClassifyDeepenIsCappedAtOneRound(t *testing.T) {
	// Both answers are weak and keep requesting evidence; the deepen pass must
	// still fire at most once and never loop.
	sc := &scriptedCompleter{responses: []string{
		weakResponse(genericFix, "source:cns/restserver/ipam.go"),
		weakResponse(genericFix, "source:cns/restserver/ipam.go"),
	}}
	prov := mapProvider{m: map[string]string{"source:cns/restserver/ipam.go": "FULL_SOURCE_MARKER"}}

	got, err := NewLLMClassifier(sc).WithEvidence(prov).
		Classify(context.Background(), markerRC(), model.Evidence{}, model.Fingerprint{}, nil, PriorContext{})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if len(sc.calls) != 2 {
		t.Fatalf("deepen must be capped at one extra call; got %d calls", len(sc.calls))
	}
	if got.Category != model.CategoryPRRegression {
		t.Errorf("unexpected category %s", got.Category)
	}
}
