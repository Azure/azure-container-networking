package subcmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/azure/aksmigrate/pkg/policy"
	"github.com/azure/aksmigrate/pkg/types"
	"github.com/azure/aksmigrate/pkg/utils"
)

// NewAuditCmd returns the "audit" subcommand.
func NewAuditCmd() *cobra.Command {
	var (
		kubeconfig string
		inputDir   string
		output     string
		k8sVersion string
	)

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Analyze NetworkPolicies for Cilium migration incompatibilities",
		Long: `Scans NetworkPolicies and cluster resources to detect breaking changes
that will occur when migrating from Azure NPM to Cilium. Produces a detailed
audit report with findings categorized by severity (FAIL, WARN, INFO).`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var resources *types.ClusterResources
			var err error

			switch {
			case inputDir != "":
				resources, err = utils.LoadFromDirectory(inputDir)
				if err != nil {
					return fmt.Errorf("failed to load from directory %s: %w", inputDir, err)
				}
			case kubeconfig != "":
				client, _, kerr := utils.NewKubeClient(kubeconfig)
				if kerr != nil {
					return fmt.Errorf("failed to create kube client: %w", kerr)
				}
				resources, err = utils.LoadFromCluster(context.Background(), client)
				if err != nil {
					return fmt.Errorf("failed to load from cluster: %w", err)
				}
			default:
				client, _, kerr := utils.NewKubeClient("")
				if kerr != nil {
					return fmt.Errorf("failed to create kube client with default kubeconfig: %w", kerr)
				}
				resources, err = utils.LoadFromCluster(context.Background(), client)
				if err != nil {
					return fmt.Errorf("failed to load from cluster: %w", err)
				}
			}

			analyzer := policy.NewAnalyzer(resources, k8sVersion)
			report := analyzer.Analyze()

			if err := utils.PrintAuditReport(report, output); err != nil {
				return fmt.Errorf("failed to print report: %w", err)
			}

			if report.Summary.FailCount > 0 {
				os.Exit(1)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig file")
	cmd.Flags().StringVar(&inputDir, "input-dir", "", "path to directory containing Kubernetes YAML manifests")
	cmd.Flags().StringVar(&output, "output", "table", "output format: table or json")
	cmd.Flags().StringVar(&k8sVersion, "k8s-version", "1.29", "target Kubernetes version (determines Cilium version)")

	return cmd
}
