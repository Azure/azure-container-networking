// Package live collects read-only diagnostics from a retained failing cluster.
// Every command is gated by the command policy before execution so the agent can
// never mutate cluster state. Command execution is injected so tests can run the
// collector without a real cluster.
package live

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/command"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

// Runner executes a single command and returns its combined output. The concrete
// kubectl-backed runner lives in main; tests inject a fake.
type Runner interface {
	Run(ctx context.Context, argv []string) (string, error)
}

// diagnostic is one labeled read-only command.
type diagnostic struct {
	name string
	argv []string
}

// diagnostics is the fixed read-only command set the agent runs against a
// retained cluster. ACN components live in kube-system. Both the Linux
// (k8s-app=azure-cns) and Windows (k8s-app=azure-cns-win) CNS label selectors
// are included; the one that does not match the cluster's OS simply returns no
// pods and is recorded as empty, best-effort.
var diagnostics = []diagnostic{
	{"pods", []string{"kubectl", "get", "pods", "-A", "-o", "wide"}},
	{"nodes", []string{"kubectl", "get", "nodes", "-o", "wide"}},
	{"node-conditions", []string{"kubectl", "describe", "nodes"}},
	{"node-events", []string{"kubectl", "get", "events", "-A", "--field-selector", "involvedObject.kind=Node", "--sort-by=.lastTimestamp"}},
	{"events", []string{"kubectl", "get", "events", "-A", "--sort-by=.lastTimestamp"}},
	{"daemonsets", []string{"kubectl", "get", "daemonsets", "-n", "kube-system", "-o", "wide"}},
	{"cns-logs", []string{"kubectl", "logs", "-n", "kube-system", "-l", "k8s-app=azure-cns", "--tail=200", "--prefix"}},
	{"cns-logs-windows", []string{"kubectl", "logs", "-n", "kube-system", "-l", "k8s-app=azure-cns-win", "--tail=200", "--prefix"}},
	{"cilium-logs", []string{"kubectl", "logs", "-n", "kube-system", "-l", "k8s-app=cilium", "--tail=200", "--prefix"}},
}

// Result is the collected live evidence.
type Result struct {
	// Outputs maps a diagnostic name to its captured output.
	Outputs map[string]string
	// Executed is the list of commands that passed policy and were run, in order.
	Executed [][]string
}

// Collector runs the diagnostic command set through a Runner.
type Collector struct {
	runner Runner
}

// NewCollector returns a Collector backed by runner.
func NewCollector(runner Runner) *Collector {
	return &Collector{runner: runner}
}

// Collect runs every diagnostic, skipping any that fail the command policy and
// recording per-command errors as output. It is best-effort: a failing command
// never aborts collection.
func (c *Collector) Collect(ctx context.Context) Result {
	res := Result{Outputs: make(map[string]string, len(diagnostics))}
	for _, d := range diagnostics {
		if err := command.Validate(d.argv); err != nil {
			res.Outputs[d.name] = fmt.Sprintf("[skipped: %v]", err)
			continue
		}
		out, err := c.runner.Run(ctx, d.argv)
		if err != nil {
			res.Outputs[d.name] = fmt.Sprintf("[command failed: %v]\n%s", err, out)
		} else {
			res.Outputs[d.name] = out
		}
		res.Executed = append(res.Executed, d.argv)
	}
	return res
}

// Merge folds live diagnostics into evidence, returning a new Evidence. Live
// outputs become excerpts (so the LLM sees them) and named files, without
// mutating the input.
func Merge(ev model.Evidence, r Result) model.Evidence {
	merged := ev
	merged.Files = append(append([]string(nil), ev.Files...), liveNames(r)...)

	merged.Excerpts = make(map[string]string, len(ev.Excerpts)+len(r.Outputs))
	for k, v := range ev.Excerpts {
		merged.Excerpts[k] = v
	}
	for name, out := range r.Outputs {
		merged.Excerpts["live/"+name] = out
	}
	return merged
}

func liveNames(r Result) []string {
	names := make([]string, 0, len(r.Outputs))
	for _, d := range diagnostics {
		if _, ok := r.Outputs[d.name]; ok {
			names = append(names, "live/"+d.name)
		}
	}
	return names
}

// CommandString renders argv for logging.
func CommandString(argv []string) string {
	return strings.Join(argv, " ")
}
