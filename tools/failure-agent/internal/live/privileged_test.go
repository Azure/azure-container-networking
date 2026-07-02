package live

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPrivilegedCollectDiscoversNodesAndRunsDiagnostics(t *testing.T) {
	r := &fakeRunner{
		outputs: map[string]string{
			"kubectl get nodes -o name": "node/aks-pool-1\nnode/aks-pool-2\n",
		},
	}
	res := NewPrivilegedCollector(r).Collect(context.Background())

	if len(res.Executed) == 0 {
		t.Fatal("expected privileged collector to execute commands")
	}

	// Should have outputs for each node × each diagnostic.
	for _, node := range []string{"aks-pool-1", "aks-pool-2"} {
		for _, d := range privilegedDiagnostics {
			key := "privileged/" + node + "/" + d.name
			if _, ok := res.Outputs[key]; !ok {
				t.Errorf("missing output for %s", key)
			}
		}
	}
}

func TestPrivilegedCollectHandlesNodeDiscoveryFailure(t *testing.T) {
	r := &fakeRunner{
		outputs: map[string]string{},
		errFor:  map[string]error{"kubectl get nodes -o name": errors.New("connection refused")},
	}
	res := NewPrivilegedCollector(r).Collect(context.Background())

	if got := res.Outputs["privileged/error"]; !strings.Contains(got, "node discovery failed") {
		t.Errorf("expected node discovery error, got %q", got)
	}
	if len(res.Executed) != 0 {
		t.Error("should not execute diagnostics when node discovery fails")
	}
}

func TestPrivilegedCollectRecordsSingleCommandFailure(t *testing.T) {
	r := &fakeRunner{
		outputs: map[string]string{
			"kubectl get nodes -o name": "node/aks-pool-1\n",
		},
	}
	// Make all debug commands fail for a specific diagnostic.
	argv := privilegedDiagnostics[0].buildArgv("node/aks-pool-1")
	key := CommandString(argv)
	r.errFor = map[string]error{key: errors.New("exec failed")}
	r.outputs[key] = "some partial output"

	res := NewPrivilegedCollector(r).Collect(context.Background())

	outKey := "privileged/aks-pool-1/" + privilegedDiagnostics[0].name
	if got := res.Outputs[outKey]; !strings.Contains(got, "command failed") {
		t.Errorf("expected command failure recorded, got %q", got)
	}
	// Other diagnostics should still run.
	if len(res.Executed) < 2 {
		t.Error("a single command failure must not abort collection")
	}
}

func TestShortNode(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"node/aks-pool-1", "aks-pool-1"},
		{"aks-pool-1", "aks-pool-1"},
	}
	for _, tt := range tests {
		if got := shortNode(tt.input); got != tt.want {
			t.Errorf("shortNode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
