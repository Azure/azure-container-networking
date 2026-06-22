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
