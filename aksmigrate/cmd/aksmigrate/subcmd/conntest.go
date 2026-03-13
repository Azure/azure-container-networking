package subcmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/azure/aksmigrate/pkg/connectivity"
	"github.com/azure/aksmigrate/pkg/utils"
)

// NewConntestCmd returns the "conntest" subcommand with snapshot/validate/diff children.
func NewConntestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "conntest",
		Short: "Pre/post migration connectivity validation",
		Long: `Captures connectivity snapshots before and after migration from Azure NPM
to Cilium, and validates that no regressions have been introduced.

Use "snapshot" to capture the current connectivity state, "validate" to run
a post-migration snapshot and compare against a pre-migration baseline, or
"diff" to compare two previously saved snapshots offline.`,
		SilenceUsage: true,
	}

	cmd.AddCommand(newSnapshotCmd())
	cmd.AddCommand(newValidateCmd())
	cmd.AddCommand(newDiffCmd())

	return cmd
}

func newSnapshotCmd() *cobra.Command {
	var (
		kubeconfig string
		output     string
		phase      string
	)

	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Capture a connectivity snapshot from the current cluster state",
		Long: `Runs connectivity probes across the cluster and saves the results to a JSON file.
Use --phase to tag the snapshot as "pre-migration" or "post-migration".`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if phase != "pre-migration" && phase != "post-migration" {
				return fmt.Errorf("--phase must be either \"pre-migration\" or \"post-migration\", got %q", phase)
			}

			client, restConfig, err := utils.NewKubeClient(kubeconfig)
			if err != nil {
				return fmt.Errorf("failed to create kube client: %w", err)
			}

			ctx := context.Background()
			prober := connectivity.NewProber(client, restConfig)

			fmt.Printf("Running %s connectivity snapshot...\n", phase)
			snapshot, err := prober.RunSnapshot(ctx, phase)
			if err != nil {
				return fmt.Errorf("failed to run connectivity snapshot: %w", err)
			}

			fmt.Printf("Captured %d connectivity results.\n", len(snapshot.Results))

			if output != "" {
				if err := connectivity.SaveSnapshot(snapshot, output); err != nil {
					return fmt.Errorf("failed to save snapshot to %s: %w", output, err)
				}
				fmt.Printf("Snapshot saved to %s\n", output)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig file")
	cmd.Flags().StringVar(&output, "output", "", "file path to save the snapshot JSON")
	cmd.Flags().StringVar(&phase, "phase", "", "snapshot phase: pre-migration or post-migration")
	_ = cmd.MarkFlagRequired("output")
	_ = cmd.MarkFlagRequired("phase")

	return cmd
}

func newValidateCmd() *cobra.Command {
	var (
		kubeconfig  string
		preSnapshot string
		output      string
	)

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Run a post-migration snapshot and validate against a pre-migration baseline",
		Long: `Runs a new post-migration connectivity snapshot on the current cluster, loads
a previously saved pre-migration snapshot, and compares the two. Prints a diff
report and exits with code 1 if any regressions are found.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, restConfig, err := utils.NewKubeClient(kubeconfig)
			if err != nil {
				return fmt.Errorf("failed to create kube client: %w", err)
			}

			ctx := context.Background()
			prober := connectivity.NewProber(client, restConfig)

			fmt.Println("Running post-migration connectivity snapshot...")
			postSnap, err := prober.RunSnapshot(ctx, "post-migration")
			if err != nil {
				return fmt.Errorf("failed to run post-migration snapshot: %w", err)
			}
			fmt.Printf("Captured %d connectivity results.\n", len(postSnap.Results))

			if output != "" {
				if err := connectivity.SaveSnapshot(postSnap, output); err != nil {
					return fmt.Errorf("failed to save post-migration snapshot to %s: %w", output, err)
				}
				fmt.Printf("Post-migration snapshot saved to %s\n", output)
			}

			fmt.Printf("Loading pre-migration snapshot from %s...\n", preSnapshot)
			preSnap, err := connectivity.LoadSnapshot(preSnapshot)
			if err != nil {
				return fmt.Errorf("failed to load pre-migration snapshot from %s: %w", preSnapshot, err)
			}

			fmt.Println("Comparing snapshots...")
			diff := connectivity.DiffSnapshots(preSnap, postSnap)
			connectivity.PrintDiffReport(diff)

			if len(diff.Regressions) > 0 {
				fmt.Printf("\nVALIDATION FAILED: %d regressions detected.\n", len(diff.Regressions))
				os.Exit(1)
			}

			fmt.Println("\nVALIDATION PASSED: No connectivity regressions detected.")
			return nil
		},
	}

	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig file")
	cmd.Flags().StringVar(&preSnapshot, "pre-snapshot", "", "file path to pre-migration snapshot JSON")
	cmd.Flags().StringVar(&output, "output", "", "file path to save the post-migration snapshot JSON")
	_ = cmd.MarkFlagRequired("pre-snapshot")

	return cmd
}

func newDiffCmd() *cobra.Command {
	var (
		preFile  string
		postFile string
	)

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compare two previously saved connectivity snapshots offline",
		Long: `Loads two snapshot files and produces a diff report showing regressions,
new allows, and unchanged connectivity results. This is an offline operation
that does not require cluster access.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Loading pre-migration snapshot from %s...\n", preFile)
			preSnap, err := connectivity.LoadSnapshot(preFile)
			if err != nil {
				return fmt.Errorf("failed to load pre-migration snapshot from %s: %w", preFile, err)
			}

			fmt.Printf("Loading post-migration snapshot from %s...\n", postFile)
			postSnap, err := connectivity.LoadSnapshot(postFile)
			if err != nil {
				return fmt.Errorf("failed to load post-migration snapshot from %s: %w", postFile, err)
			}

			fmt.Println("Comparing snapshots...")
			diff := connectivity.DiffSnapshots(preSnap, postSnap)
			connectivity.PrintDiffReport(diff)

			if len(diff.Regressions) > 0 {
				fmt.Printf("\n%d regressions found.\n", len(diff.Regressions))
				os.Exit(1)
			}

			fmt.Println("\nNo connectivity regressions detected.")
			return nil
		},
	}

	cmd.Flags().StringVar(&preFile, "pre", "", "file path to pre-migration snapshot JSON")
	cmd.Flags().StringVar(&postFile, "post", "", "file path to post-migration snapshot JSON")
	_ = cmd.MarkFlagRequired("pre")
	_ = cmd.MarkFlagRequired("post")

	return cmd
}
