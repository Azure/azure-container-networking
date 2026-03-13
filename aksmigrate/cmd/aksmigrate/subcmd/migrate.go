package subcmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/azure/aksmigrate/pkg/cluster"
	"github.com/azure/aksmigrate/pkg/connectivity"
	"github.com/azure/aksmigrate/pkg/policy"
	"github.com/azure/aksmigrate/pkg/types"
	"github.com/azure/aksmigrate/pkg/utils"
)

// NewMigrateCmd returns the "migrate" subcommand.
func NewMigrateCmd() *cobra.Command {
	var (
		clusterName    string
		resourceGroup  string
		kubeconfig     string
		outputDir      string
		skipSnapshot   bool
		skipValidation bool
		dryRun         bool
		k8sVersion     string
	)

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Orchestrate a full NPM-to-Cilium migration for an AKS cluster",
		Long: `Runs the end-to-end migration workflow for a single AKS cluster:
preflight checks, connectivity snapshots, policy audit and translation,
migration execution, patch application, and post-migration validation.

Use --dry-run to preview the migration plan without making any changes.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			fmt.Println("=== NPM-to-Cilium Migration ===")
			fmt.Printf("Cluster:        %s\n", clusterName)
			fmt.Printf("Resource Group: %s\n", resourceGroup)
			fmt.Printf("K8s Version:    %s\n", k8sVersion)
			fmt.Printf("Output Dir:     %s\n", outputDir)
			fmt.Printf("Dry Run:        %v\n", dryRun)
			fmt.Println()

			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
			}

			client, restConfig, err := utils.NewKubeClient(kubeconfig)
			if err != nil {
				return fmt.Errorf("failed to create kube client: %w", err)
			}

			orch := cluster.NewOrchestrator(clusterName, resourceGroup, kubeconfig)

			// Step 1: Preflight checks
			fmt.Println("[1/7] Running preflight checks...")
			issues, err := orch.PreflightCheck(ctx)
			if err != nil {
				return fmt.Errorf("preflight checks failed: %w", err)
			}
			if len(issues) > 0 {
				for _, issue := range issues {
					fmt.Printf("  BLOCKING: %s\n", issue)
				}
				if !dryRun {
					return fmt.Errorf("preflight check found %d blocking issue(s)", len(issues))
				}
			}
			fmt.Println("  Preflight checks passed.")
			fmt.Println()

			// Step 2: Pre-migration connectivity snapshot
			var preSnapshotPath string
			if !skipSnapshot {
				fmt.Println("[2/7] Capturing pre-migration connectivity snapshot...")
				prober := connectivity.NewProber(client, restConfig)
				preSnap, err := prober.RunSnapshot(ctx, "pre-migration")
				if err != nil {
					return fmt.Errorf("failed to capture pre-migration snapshot: %w", err)
				}

				preSnapshotPath = filepath.Join(outputDir, "pre-migration-snapshot.json")
				if err := connectivity.SaveSnapshot(preSnap, preSnapshotPath); err != nil {
					return fmt.Errorf("failed to save pre-migration snapshot: %w", err)
				}
				fmt.Printf("  Snapshot saved: %s (%d probes)\n\n", preSnapshotPath, len(preSnap.Results))
			} else {
				fmt.Println("[2/7] Skipping pre-migration snapshot (--skip-snapshot)")
				fmt.Println()
			}

			// Step 3: Policy audit
			fmt.Println("[3/7] Running policy audit...")
			resources, err := utils.LoadFromCluster(ctx, client)
			if err != nil {
				return fmt.Errorf("failed to load cluster resources: %w", err)
			}

			analyzer := policy.NewAnalyzer(resources, k8sVersion)
			auditReport := analyzer.Analyze()
			auditReport.ClusterName = clusterName

			if err := utils.PrintAuditReport(auditReport, "table"); err != nil {
				return fmt.Errorf("failed to print audit report: %w", err)
			}

			if auditReport.Summary.FailCount > 0 && !dryRun {
				return fmt.Errorf("migration blocked: %d FAIL findings must be resolved before migration (use --dry-run to preview)", auditReport.Summary.FailCount)
			}
			fmt.Println()

			// Step 4: Policy translation
			fmt.Println("[4/7] Translating policies...")
			translator := policy.NewTranslator(resources, k8sVersion)
			translation := translator.Translate()
			utils.PrintTranslationSummary(translation)

			patchDir := filepath.Join(outputDir, "patches")
			if err := os.MkdirAll(patchDir, 0755); err != nil {
				return fmt.Errorf("failed to create patches directory: %w", err)
			}

			if err := writePatchFiles(translation, patchDir); err != nil {
				return fmt.Errorf("failed to write patch files: %w", err)
			}
			fmt.Printf("  Patches written to %s\n\n", patchDir)

			// Step 5: Execute migration
			if !dryRun {
				fmt.Println("[5/7] Executing migration...")
				if err := orch.Migrate(ctx); err != nil {
					return fmt.Errorf("migration execution failed: %w", err)
				}
				fmt.Println("  Waiting for nodes to become ready...")
				if err := orch.MonitorProgress(ctx); err != nil {
					return fmt.Errorf("monitoring migration progress: %w", err)
				}
				fmt.Println("  Migration complete.")
				fmt.Println()
			} else {
				fmt.Println("[5/7] Skipping migration execution (--dry-run)")
				fmt.Println()
			}

			// Step 6: Apply patches
			if !dryRun {
				fmt.Println("[6/7] Applying policy patches...")
				if err := orch.ApplyPatches(ctx, filepath.Join(outputDir, "patches")); err != nil {
					return fmt.Errorf("patch application failed: %w", err)
				}
				fmt.Println()
			} else {
				fmt.Println("[6/7] Skipping patch application (--dry-run)")
				fmt.Println()
			}

			// Step 7: Post-migration validation
			if !skipValidation && !dryRun {
				fmt.Println("[7/7] Running post-migration validation...")
				prober := connectivity.NewProber(client, restConfig)
				postSnap, err := prober.RunSnapshot(ctx, "post-migration")
				if err != nil {
					return fmt.Errorf("failed to capture post-migration snapshot: %w", err)
				}

				postSnapshotPath := filepath.Join(outputDir, "post-migration-snapshot.json")
				if err := connectivity.SaveSnapshot(postSnap, postSnapshotPath); err != nil {
					return fmt.Errorf("failed to save post-migration snapshot: %w", err)
				}
				fmt.Printf("  Post-migration snapshot saved: %s (%d probes)\n", postSnapshotPath, len(postSnap.Results))

				if preSnapshotPath != "" {
					preSnap, err := connectivity.LoadSnapshot(preSnapshotPath)
					if err != nil {
						return fmt.Errorf("failed to reload pre-migration snapshot: %w", err)
					}

					diff := connectivity.DiffSnapshots(preSnap, postSnap)
					connectivity.PrintDiffReport(diff)

					if len(diff.Regressions) > 0 {
						fmt.Printf("\n  WARNING: %d connectivity regressions detected!\n", len(diff.Regressions))
					} else {
						fmt.Println("\n  Validation passed: no connectivity regressions.")
					}
				}
				fmt.Println()
			} else {
				reason := "--dry-run"
				if skipValidation {
					reason = "--skip-validation"
				}
				fmt.Printf("[7/7] Skipping post-migration validation (%s)\n\n", reason)
			}

			// Final report
			printFinalReport(clusterName, resourceGroup, dryRun, auditReport, translation, preSnapshotPath, outputDir)

			return nil
		},
	}

	cmd.Flags().StringVar(&clusterName, "cluster-name", "", "name of the AKS cluster")
	cmd.Flags().StringVar(&resourceGroup, "resource-group", "", "Azure resource group containing the cluster")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig file")
	cmd.Flags().StringVar(&outputDir, "output-dir", "./migration-output", "directory to write migration artifacts")
	cmd.Flags().BoolVar(&skipSnapshot, "skip-snapshot", false, "skip pre-migration connectivity snapshot")
	cmd.Flags().BoolVar(&skipValidation, "skip-validation", false, "skip post-migration connectivity validation")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the migration plan without executing changes")
	cmd.Flags().StringVar(&k8sVersion, "k8s-version", "1.29", "target Kubernetes version (determines Cilium version)")

	_ = cmd.MarkFlagRequired("cluster-name")
	_ = cmd.MarkFlagRequired("resource-group")

	return cmd
}

func writePatchFiles(translation *types.TranslationOutput, patchDir string) error {
	for _, pp := range translation.PatchedPolicies {
		filename := filepath.Join(patchDir, fmt.Sprintf("patched-netpol-%s-%s.yaml", pp.Patched.Namespace, pp.Patched.Name))
		content := renderNetworkPolicyYAML(pp)
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", filename, err)
		}
	}

	for _, cp := range translation.CiliumPolicies {
		filename := filepath.Join(patchDir, fmt.Sprintf("cilium-netpol-%s-%s.yaml", cp.Namespace, cp.Name))
		content := policy.RenderCiliumPolicyYAML(cp)
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", filename, err)
		}
	}

	return nil
}

func renderNetworkPolicyYAML(pp types.PatchedPolicy) string {
	header := fmt.Sprintf("# Patched NetworkPolicy: %s/%s\n", pp.Patched.Namespace, pp.Patched.Name)
	header += "# Changes:\n"
	for _, c := range pp.Changes {
		header += fmt.Sprintf("#   - %s\n", c)
	}
	header += "---\n"
	header += fmt.Sprintf("apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\nmetadata:\n  name: %s\n  namespace: %s\n",
		pp.Patched.Name, pp.Patched.Namespace)
	return header
}

func printFinalReport(clusterName, resourceGroup string, dryRun bool, audit *types.AuditReport, translation *types.TranslationOutput, preSnapshotPath, outputDir string) {
	fmt.Println("=== Migration Report ===")
	fmt.Printf("Cluster:            %s\n", clusterName)
	fmt.Printf("Resource Group:     %s\n", resourceGroup)

	if dryRun {
		fmt.Printf("Mode:               DRY RUN (no changes applied)\n")
	} else {
		fmt.Printf("Mode:               LIVE MIGRATION\n")
	}

	fmt.Printf("Policies Analyzed:  %d\n", audit.TotalPolicies)
	fmt.Printf("Audit Findings:     FAIL=%d WARN=%d INFO=%d\n",
		audit.Summary.FailCount, audit.Summary.WarnCount, audit.Summary.InfoCount)
	fmt.Printf("Policies Patched:   %d\n", len(translation.PatchedPolicies))
	fmt.Printf("Cilium Policies:    %d\n", len(translation.CiliumPolicies))
	fmt.Printf("Named Ports Fixed:  %d\n", len(translation.RemovedNamedPorts))

	if preSnapshotPath != "" {
		fmt.Printf("Pre-snapshot:       %s\n", preSnapshotPath)
	}

	fmt.Printf("Output Directory:   %s\n", outputDir)
	fmt.Println()

	if dryRun {
		fmt.Println("Dry run complete. Review the output directory and re-run without --dry-run to execute.")
	} else if audit.Summary.FailCount > 0 {
		fmt.Println("Migration blocked due to critical audit findings. Resolve FAIL issues and retry.")
	} else {
		fmt.Println("Migration complete. Review post-migration validation results above.")
	}
}
