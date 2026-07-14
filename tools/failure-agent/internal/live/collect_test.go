package live

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/command"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

// fakeRunner records every command it is asked to run and returns canned output.
type fakeRunner struct {
	calls   [][]string
	outputs map[string]string
	errFor  map[string]error
}

func (f *fakeRunner) Run(_ context.Context, argv []string) (string, error) {
	f.calls = append(f.calls, argv)
	key := CommandString(argv)
	if f.errFor != nil {
		if err, ok := f.errFor[key]; ok {
			return f.outputs[key], err
		}
	}
	return f.outputs[key], nil
}

func TestCollectRunsNodeEventsDiagnostic(t *testing.T) {
	r := &fakeRunner{outputs: map[string]string{}}
	res := NewCollector(r).Collect(context.Background())

	if _, ok := res.Outputs["node-events"]; !ok {
		t.Fatal("expected a node-events diagnostic to be collected")
	}
	var found bool
	for _, argv := range r.calls {
		if CommandString(argv) == "kubectl get events -A --field-selector involvedObject.kind=Node --sort-by=.lastTimestamp" {
			found = true
			if err := command.Validate(argv); err != nil {
				t.Errorf("node-events command rejected by policy: %v", err)
			}
		}
	}
	if !found {
		t.Error("node-events diagnostic did not run the expected node-scoped events query")
	}
}

func TestCollectOnlyRunsAllowedCommands(t *testing.T) {
	r := &fakeRunner{outputs: map[string]string{}}
	res := NewCollector(r).Collect(context.Background())

	if len(r.calls) == 0 {
		t.Fatal("expected the collector to run at least one diagnostic")
	}
	for _, argv := range r.calls {
		if err := command.Validate(argv); err != nil {
			t.Errorf("collector ran a non-allowed command %v: %v", argv, err)
		}
	}
	if len(res.Outputs) != len(diagnostics) {
		t.Errorf("expected %d outputs, got %d", len(diagnostics), len(res.Outputs))
	}
}

func TestCollectRecordsCommandErrors(t *testing.T) {
	failing := CommandString([]string{"kubectl", "get", "nodes", "-o", "wide"})
	r := &fakeRunner{
		outputs: map[string]string{failing: ""},
		errFor:  map[string]error{failing: errors.New("connection refused")},
	}
	res := NewCollector(r).Collect(context.Background())

	if got := res.Outputs["nodes"]; got == "" || !strings.Contains(got, "command failed") {
		t.Errorf("expected failure note for nodes diagnostic, got %q", got)
	}
	if _, ok := res.Outputs["pods"]; !ok {
		t.Error("a single command failure must not abort collection")
	}
}

func TestMergeFoldsLiveEvidenceWithoutMutating(t *testing.T) {
	ev := model.Evidence{
		Files:    []string{"artifact.log"},
		Excerpts: map[string]string{"artifact.log": "boom"},
	}
	res := Result{Outputs: map[string]string{"pods": "pod output"}}

	merged := Merge(ev, res)

	if len(ev.Files) != 1 || len(ev.Excerpts) != 1 {
		t.Error("Merge must not mutate the input evidence")
	}
	if merged.Excerpts["live/pods"] != "pod output" {
		t.Errorf("expected live/pods excerpt, got %q", merged.Excerpts["live/pods"])
	}
	if merged.Excerpts["artifact.log"] != "boom" {
		t.Error("expected original excerpts preserved")
	}
}

func TestLiveNamesOrderIsDeterministic(t *testing.T) {
	// Build a Result with outputs keyed in arbitrary (non-diagnostics) order.
	res := Result{Outputs: map[string]string{
		"events":          "ev",
		"pods":            "po",
		"nodes":           "no",
		"daemonsets":      "ds",
		"cns-logs":        "cl",
		"cilium-logs":     "ci",
		"node-conditions": "nc",
	}}

	// Run liveNames multiple times and confirm order is always the same.
	first := liveNames(res)
	for i := 0; i < 20; i++ {
		got := liveNames(res)
		if len(got) != len(first) {
			t.Fatalf("iteration %d: length mismatch %d vs %d", i, len(got), len(first))
		}
		for j := range first {
			if got[j] != first[j] {
				t.Fatalf("iteration %d: index %d differs: %q vs %q", i, j, got[j], first[j])
			}
		}
	}

	// Confirm the order follows the diagnostics slice.
	expected := []string{
		"live/pods",
		"live/nodes",
		"live/node-conditions",
		"live/events",
		"live/daemonsets",
		"live/cns-logs",
		"live/cilium-logs",
	}
	if len(first) != len(expected) {
		t.Fatalf("expected %d names, got %d", len(expected), len(first))
	}
	for i, want := range expected {
		if first[i] != want {
			t.Errorf("index %d: got %q, want %q", i, first[i], want)
		}
	}
}

func TestLiveNamesExcludesMissingDiagnostics(t *testing.T) {
	// Only a subset of diagnostics has output.
	res := Result{Outputs: map[string]string{
		"pods":  "po",
		"nodes": "no",
	}}
	names := liveNames(res)
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d: %v", len(names), names)
	}
	if names[0] != "live/pods" || names[1] != "live/nodes" {
		t.Errorf("unexpected order: %v", names)
	}
}
