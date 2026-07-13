package live

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/command"
)

const (
	// debugImage is the ephemeral container image used for node-shell access.
	debugImage = "mcr.microsoft.com/cbl-mariner/busybox:2.0"
	// nodeLogTail limits how many bytes are read from each node log file.
	nodeLogTail = "200000"
)

// nodeDiagnostic is a privileged command template run on each node. The
// placeholder %s is replaced with the node name (e.g. "node/aks-nodepool-12345").
type nodeDiagnostic struct {
	name string
	// argv is the full command. The node name is pre-built into the argv by the
	// collector at runtime.
	buildArgv func(node string) []string
}

// privilegedDiagnostics are host-level log collection commands. Each is run
// per-node via kubectl debug.
var privilegedDiagnostics = []nodeDiagnostic{
	{
		name: "azure-vnet",
		buildArgv: func(node string) []string {
			return []string{"kubectl", "debug", node, "--image=" + debugImage, "--quiet", "--",
				"tail", "-c", nodeLogTail, "/host/var/log/azure-vnet.log"}
		},
	},
	{
		name: "azure-vnet-ipam",
		buildArgv: func(node string) []string {
			return []string{"kubectl", "debug", node, "--image=" + debugImage, "--quiet", "--",
				"tail", "-c", nodeLogTail, "/host/var/log/azure-vnet-ipam.log"}
		},
	},
	{
		name: "azure-cns-node",
		buildArgv: func(node string) []string {
			return []string{"kubectl", "debug", node, "--image=" + debugImage, "--quiet", "--",
				"tail", "-c", nodeLogTail, "/host/var/log/azure-cns.log"}
		},
	},
	{
		// NOTE: HNS state is Windows-only. This node-shell path uses a Linux
		// debug image (debugImage), so these commands only succeed where a
		// PowerShell-capable shell is reachable via `kubectl debug node`. On
		// Linux nodes they are recorded as best-effort failures and ignored.
		// The authoritative, reliable HNS capture is the CI pipeline
		// (.pipelines/templates/log.steps.yaml, Windows section), which execs
		// Get-HnsNetwork/Get-HnsEndpoint inside the Windows privileged
		// (hostProcess/SYSTEM) daemonset and publishes them in the evidence
		// bundle the agent consumes.
		name: "hns-networks",
		buildArgv: func(node string) []string {
			return []string{"kubectl", "debug", node, "--image=" + debugImage, "--quiet", "--",
				"powershell", "-Command", "Get-HnsNetwork | ConvertTo-Json -Depth 5"}
		},
	},
	{
		name: "hns-endpoints",
		buildArgv: func(node string) []string {
			return []string{"kubectl", "debug", node, "--image=" + debugImage, "--quiet", "--",
				"powershell", "-Command", "Get-HnsEndpoint | ConvertTo-Json -Depth 5"}
		},
	},
}

// PrivilegedCollector collects host-level logs by running kubectl debug on each
// node. It requires explicit opt-in via --privileged because it creates
// ephemeral debug pods (a mutating operation).
type PrivilegedCollector struct {
	runner Runner
}

// NewPrivilegedCollector returns a PrivilegedCollector backed by runner.
func NewPrivilegedCollector(runner Runner) *PrivilegedCollector {
	return &PrivilegedCollector{runner: runner}
}

// Collect discovers cluster nodes and runs privileged diagnostics on each.
// Results are returned with keys like "privileged/<node>/<diagnostic-name>".
func (c *PrivilegedCollector) Collect(ctx context.Context) Result {
	res := Result{Outputs: make(map[string]string)}

	nodes, err := c.discoverNodes(ctx)
	if err != nil {
		res.Outputs["privileged/error"] = fmt.Sprintf("[node discovery failed: %v]", err)
		return res
	}

	for _, node := range nodes {
		for _, d := range privilegedDiagnostics {
			argv := d.buildArgv(node)
			if err := command.ValidatePrivileged(argv); err != nil {
				key := fmt.Sprintf("privileged/%s/%s", shortNode(node), d.name)
				res.Outputs[key] = fmt.Sprintf("[skipped: %v]", err)
				continue
			}
			out, err := c.runner.Run(ctx, argv)
			key := fmt.Sprintf("privileged/%s/%s", shortNode(node), d.name)
			if err != nil {
				res.Outputs[key] = fmt.Sprintf("[command failed: %v]\n%s", err, out)
			} else {
				res.Outputs[key] = out
			}
			res.Executed = append(res.Executed, argv)
		}
	}
	return res
}

// discoverNodes returns the list of node resource names (e.g. "node/aks-pool-123").
func (c *PrivilegedCollector) discoverNodes(ctx context.Context) ([]string, error) {
	argv := []string{"kubectl", "get", "nodes", "-o", "name"}
	out, err := c.runner.Run(ctx, argv)
	if err != nil {
		return nil, fmt.Errorf("kubectl get nodes: %w", err)
	}
	var nodes []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			nodes = append(nodes, line)
		}
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes found in cluster")
	}
	return nodes, nil
}

// shortNode extracts the node name from "node/<name>".
func shortNode(node string) string {
	if i := strings.IndexByte(node, '/'); i >= 0 {
		return node[i+1:]
	}
	return node
}
