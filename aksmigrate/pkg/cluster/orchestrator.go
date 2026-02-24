package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/azure/aksmigrate/pkg/connectivity"
	"github.com/azure/aksmigrate/pkg/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// MigrationOpts holds options for running a full migration.
type MigrationOpts struct {
	OutputDir      string
	SkipSnapshot   bool
	SkipValidation bool
	DryRun         bool
}

// Orchestrator manages the migration of a single AKS cluster from NPM to Cilium.
type Orchestrator struct {
	clusterName   string
	resourceGroup string
	kubeconfig    string
}

// NewOrchestrator creates a new Orchestrator for the specified cluster.
func NewOrchestrator(clusterName, resourceGroup, kubeconfig string) *Orchestrator {
	return &Orchestrator{
		clusterName:   clusterName,
		resourceGroup: resourceGroup,
		kubeconfig:    kubeconfig,
	}
}

// PreflightCheck returns a list of blocking issues that must be resolved before migration.
// Checks include: Windows node pools, Azure CLI version, Kubernetes version, and missing
// PodDisruptionBudgets on stateful workloads.
func (o *Orchestrator) PreflightCheck(ctx context.Context) ([]string, error) {
	var issues []string

	// Check Azure CLI version.
	cliIssue, err := o.checkCLIVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("checking CLI version: %w", err)
	}
	if cliIssue != "" {
		issues = append(issues, cliIssue)
	}

	// Build the Kubernetes client to inspect the cluster.
	clientset, _, err := o.buildKubeClient()
	if err != nil {
		return nil, fmt.Errorf("building kube client: %w", err)
	}

	// Check for Windows node pools.
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1ListOptions())
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}
	for _, node := range nodes.Items {
		if os := node.Labels["kubernetes.io/os"]; os == "windows" {
			issues = append(issues, fmt.Sprintf("Windows node pool detected: node %s has os=windows label; Cilium does not support Windows nodes", node.Name))
			break
		}
	}

	// Check Kubernetes version.
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("getting server version: %w", err)
	}
	k8sVersion := fmt.Sprintf("%s.%s", serverVersion.Major, serverVersion.Minor)
	// Cilium on AKS requires Kubernetes >= 1.28.
	if strings.Compare(k8sVersion, "1.28") < 0 {
		issues = append(issues, fmt.Sprintf("Kubernetes version %s is below the minimum required 1.28 for Cilium dataplane", k8sVersion))
	}

	// Check for stateful workloads without PodDisruptionBudgets.
	pdbIssues, err := o.checkPDBs(ctx, clientset)
	if err != nil {
		return nil, fmt.Errorf("checking PDBs: %w", err)
	}
	issues = append(issues, pdbIssues...)

	if len(issues) == 0 {
		fmt.Println("All preflight checks passed.")
	} else {
		fmt.Printf("Preflight check found %d issue(s).\n", len(issues))
	}

	return issues, nil
}

// Migrate runs the AKS migration command to switch the network dataplane to Cilium.
func (o *Orchestrator) Migrate(ctx context.Context) error {
	fmt.Printf("Starting migration of cluster %s in resource group %s...\n", o.clusterName, o.resourceGroup)

	cmd := exec.CommandContext(ctx, "az", "aks", "update",
		"--resource-group", o.resourceGroup,
		"--name", o.clusterName,
		"--network-dataplane", "cilium",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("az aks update failed: %w", err)
	}

	fmt.Println("Migration command completed successfully.")
	return nil
}

// MonitorProgress polls node status until all nodes are in Ready condition.
func (o *Orchestrator) MonitorProgress(ctx context.Context) error {
	clientset, _, err := o.buildKubeClient()
	if err != nil {
		return fmt.Errorf("building kube client: %w", err)
	}

	fmt.Println("Monitoring node readiness...")

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1ListOptions())
		if err != nil {
			return fmt.Errorf("listing nodes: %w", err)
		}

		totalNodes := len(nodes.Items)
		readyNodes := 0
		for _, node := range nodes.Items {
			for _, cond := range node.Status.Conditions {
				if cond.Type == "Ready" && cond.Status == "True" {
					readyNodes++
					break
				}
			}
		}

		fmt.Printf("  Nodes ready: %d/%d\n", readyNodes, totalNodes)

		if readyNodes == totalNodes && totalNodes > 0 {
			fmt.Println("All nodes are ready.")
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(30 * time.Second):
			// Continue polling.
		}
	}
}

// ApplyPatches applies all YAML files from the given directory using kubectl apply.
func (o *Orchestrator) ApplyPatches(ctx context.Context, patchDir string) error {
	entries, err := os.ReadDir(patchDir)
	if err != nil {
		return fmt.Errorf("reading patch directory %s: %w", patchDir, err)
	}

	applied := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}

		filePath := filepath.Join(patchDir, name)
		fmt.Printf("Applying patch: %s\n", filePath)

		cmd := exec.CommandContext(ctx, "kubectl", "apply",
			"--kubeconfig", o.kubeconfig,
			"-f", filePath,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("applying patch %s: %w", filePath, err)
		}
		applied++
	}

	fmt.Printf("Applied %d patch(es) from %s\n", applied, patchDir)
	return nil
}

// RunFullMigration orchestrates the full migration workflow:
// preflight -> snapshot -> migrate -> patch -> validate.
func (o *Orchestrator) RunFullMigration(ctx context.Context, opts MigrationOpts) (*types.MigrationState, error) {
	state := &types.MigrationState{
		ClusterName:   o.clusterName,
		ResourceGroup: o.resourceGroup,
		StartTime:     time.Now().UTC().Format(time.RFC3339),
		Phase:         "preflight",
	}

	// Ensure output directory exists.
	if opts.OutputDir != "" {
		if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
			return o.failState(state, fmt.Errorf("creating output dir: %w", err))
		}
	}

	// Step 1: Preflight checks.
	fmt.Println("\n=== Phase: Preflight ===")
	issues, err := o.PreflightCheck(ctx)
	if err != nil {
		return o.failState(state, fmt.Errorf("preflight: %w", err))
	}
	if len(issues) > 0 {
		for _, issue := range issues {
			fmt.Printf("  BLOCKING: %s\n", issue)
		}
		return o.failState(state, fmt.Errorf("preflight check found %d blocking issue(s)", len(issues)))
	}

	// Step 2: Pre-migration connectivity snapshot.
	var prober *connectivity.Prober
	if !opts.SkipSnapshot {
		state.Phase = "snapshot"
		fmt.Println("\n=== Phase: Pre-Migration Snapshot ===")

		clientset, restConfig, err := o.buildKubeClient()
		if err != nil {
			return o.failState(state, fmt.Errorf("building kube client for snapshot: %w", err))
		}
		prober = connectivity.NewProber(clientset, restConfig)

		preSnapshot, err := prober.RunSnapshot(ctx, "pre-migration")
		if err != nil {
			return o.failState(state, fmt.Errorf("pre-migration snapshot: %w", err))
		}
		preSnapshot.ClusterName = o.clusterName

		if opts.OutputDir != "" {
			preSnapshotPath := filepath.Join(opts.OutputDir, "pre-migration-snapshot.json")
			if err := connectivity.SaveSnapshot(preSnapshot, preSnapshotPath); err != nil {
				return o.failState(state, fmt.Errorf("saving pre-snapshot: %w", err))
			}
			state.PreSnapshot = preSnapshotPath
		}
	}

	// Step 3: Migrate.
	state.Phase = "migrating"
	fmt.Println("\n=== Phase: Migration ===")
	if opts.DryRun {
		fmt.Println("DRY RUN: Skipping actual migration command.")
	} else {
		if err := o.Migrate(ctx); err != nil {
			return o.failState(state, err)
		}

		fmt.Println("Waiting for nodes to become ready...")
		if err := o.MonitorProgress(ctx); err != nil {
			return o.failState(state, fmt.Errorf("monitoring progress: %w", err))
		}
	}

	// Step 4: Apply patches.
	state.Phase = "patching"
	fmt.Println("\n=== Phase: Patching ===")
	if opts.OutputDir != "" {
		patchDir := filepath.Join(opts.OutputDir, "patches")
		if _, err := os.Stat(patchDir); err == nil {
			if opts.DryRun {
				fmt.Printf("DRY RUN: Would apply patches from %s\n", patchDir)
			} else {
				if err := o.ApplyPatches(ctx, patchDir); err != nil {
					return o.failState(state, fmt.Errorf("applying patches: %w", err))
				}
			}
		} else {
			fmt.Println("No patches directory found; skipping patch application.")
		}
	}

	// Step 5: Post-migration validation.
	if !opts.SkipValidation && !opts.SkipSnapshot && prober != nil {
		state.Phase = "validating"
		fmt.Println("\n=== Phase: Post-Migration Validation ===")

		postSnapshot, err := prober.RunSnapshot(ctx, "post-migration")
		if err != nil {
			return o.failState(state, fmt.Errorf("post-migration snapshot: %w", err))
		}
		postSnapshot.ClusterName = o.clusterName

		if opts.OutputDir != "" {
			postSnapshotPath := filepath.Join(opts.OutputDir, "post-migration-snapshot.json")
			if err := connectivity.SaveSnapshot(postSnapshot, postSnapshotPath); err != nil {
				return o.failState(state, fmt.Errorf("saving post-snapshot: %w", err))
			}
			state.PostSnapshot = postSnapshotPath
		}

		// Load pre-snapshot for diffing.
		if state.PreSnapshot != "" {
			preSnapshot, err := connectivity.LoadSnapshot(state.PreSnapshot)
			if err != nil {
				return o.failState(state, fmt.Errorf("loading pre-snapshot for diff: %w", err))
			}

			diff := connectivity.DiffSnapshots(preSnapshot, postSnapshot)
			connectivity.PrintDiffReport(diff)

			if len(diff.Regressions) > 0 {
				fmt.Printf("\nWARNING: %d connectivity regressions detected!\n", len(diff.Regressions))
			}
		}
	}

	// Complete.
	state.Phase = "complete"
	state.EndTime = time.Now().UTC().Format(time.RFC3339)
	fmt.Printf("\n=== Migration Complete ===\nCluster: %s\nDuration: %s -> %s\n", o.clusterName, state.StartTime, state.EndTime)

	return state, nil
}

// failState marks the migration as failed and returns the state with the error.
func (o *Orchestrator) failState(state *types.MigrationState, err error) (*types.MigrationState, error) {
	state.Phase = "failed"
	state.EndTime = time.Now().UTC().Format(time.RFC3339)
	state.Error = err.Error()
	return state, err
}

// buildKubeClient creates a Kubernetes clientset from the orchestrator's kubeconfig.
func (o *Orchestrator) buildKubeClient() (*kubernetes.Clientset, *rest.Config, error) {
	config, err := clientcmd.BuildConfigFromFlags("", o.kubeconfig)
	if err != nil {
		return nil, nil, fmt.Errorf("building kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("creating kubernetes clientset: %w", err)
	}

	return clientset, config, nil
}

// checkCLIVersion verifies the Azure CLI is installed and meets minimum version.
func (o *Orchestrator) checkCLIVersion(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "az", "version", "--output", "json")
	output, err := cmd.Output()
	if err != nil {
		return "Azure CLI (az) is not installed or not in PATH", nil
	}

	var versionInfo map[string]interface{}
	if err := parseJSON(output, &versionInfo); err != nil {
		return "Could not parse Azure CLI version output", nil
	}

	cliVersion, ok := versionInfo["azure-cli"].(string)
	if !ok {
		return "Could not determine Azure CLI version", nil
	}

	// Minimum required: 2.61.0 for Cilium dataplane support.
	if strings.Compare(cliVersion, "2.61.0") < 0 {
		return fmt.Sprintf("Azure CLI version %s is below minimum 2.61.0 required for Cilium migration", cliVersion), nil
	}

	return "", nil
}

// checkPDBs checks for StatefulSets and Deployments without PodDisruptionBudgets.
func (o *Orchestrator) checkPDBs(ctx context.Context, clientset *kubernetes.Clientset) ([]string, error) {
	var issues []string

	// List all PDBs to build a lookup set.
	pdbList, err := clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1ListOptions())
	if err != nil {
		return nil, fmt.Errorf("listing PDBs: %w", err)
	}

	pdbSelectors := make(map[string]bool)
	for _, pdb := range pdbList.Items {
		// Track PDB by namespace and selector label key-values.
		if pdb.Spec.Selector != nil {
			for k, v := range pdb.Spec.Selector.MatchLabels {
				key := fmt.Sprintf("%s/%s=%s", pdb.Namespace, k, v)
				pdbSelectors[key] = true
			}
		}
	}

	// Check StatefulSets for missing PDBs.
	ssList, err := clientset.AppsV1().StatefulSets("").List(ctx, metav1ListOptions())
	if err != nil {
		return nil, fmt.Errorf("listing StatefulSets: %w", err)
	}

	for _, ss := range ssList.Items {
		if ss.Namespace == "kube-system" {
			continue
		}
		hasPDB := false
		if ss.Spec.Selector != nil {
			for k, v := range ss.Spec.Selector.MatchLabels {
				key := fmt.Sprintf("%s/%s=%s", ss.Namespace, k, v)
				if pdbSelectors[key] {
					hasPDB = true
					break
				}
			}
		}
		if !hasPDB {
			issues = append(issues, fmt.Sprintf(
				"StatefulSet %s/%s has no PodDisruptionBudget; node drains during migration may cause downtime",
				ss.Namespace, ss.Name,
			))
		}
	}

	return issues, nil
}

// metav1ListOptions returns an empty ListOptions (avoids importing metav1 at the top
// since we use the clientset directly).
func metav1ListOptions() metav1.ListOptions {
	return metav1.ListOptions{}
}

// parseJSON is a helper to unmarshal JSON bytes into a target.
func parseJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
