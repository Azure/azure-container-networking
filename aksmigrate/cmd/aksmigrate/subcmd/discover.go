package subcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/azure/aksmigrate/pkg/cluster"
	"github.com/azure/aksmigrate/pkg/types"
)

// NewDiscoverCmd returns the "discover" subcommand.
func NewDiscoverCmd() *cobra.Command {
	var (
		subscription string
		output       string
	)

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Find and prioritize AKS clusters running Azure NPM for migration",
		Long: `Scans an Azure subscription for AKS clusters using Azure NPM as their network
policy engine. Evaluates each cluster's configuration and produces a prioritized
migration plan based on risk level, complexity, and readiness.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			discoverer := cluster.NewFleetDiscoverer(subscription)

			fmt.Println("Discovering AKS clusters with NPM...")
			clusters, err := discoverer.DiscoverClusters(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to discover clusters: %w", err)
			}

			if len(clusters) == 0 {
				fmt.Println("No AKS clusters with Azure NPM found in the subscription.")
				return nil
			}

			fmt.Printf("Found %d NPM cluster(s). Prioritizing migration order...\n\n", len(clusters))
			plan := discoverer.PrioritizeMigration(clusters)

			switch output {
			case "json":
				return printDiscoverJSON(plan)
			default:
				printDiscoverTable(plan)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&subscription, "subscription", "", "Azure subscription ID to scan")
	cmd.Flags().StringVar(&output, "output", "table", "output format: table or json")

	return cmd
}

func printDiscoverJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printDiscoverTable(plan *types.MigrationPlan) {
	fmt.Printf("%-30s %-25s %-12s %-10s %-10s %-10s %-10s\n",
		"CLUSTER NAME", "RESOURCE GROUP", "K8S VERSION", "NODES", "WINDOWS", "RISK", "ORDER")
	fmt.Println(strings.Repeat("-", 110))

	for _, c := range plan.Clusters {
		hasWindows := "No"
		if c.HasWindowsPools {
			hasWindows = "Yes"
		}

		fmt.Printf("%-30s %-25s %-12s %-10d %-10s %-10s %-10d\n",
			truncate(c.Name, 30),
			truncate(c.ResourceGroup, 25),
			c.KubernetesVersion,
			c.NodeCount,
			hasWindows,
			c.RiskLevel,
			c.MigrationOrder,
		)
	}

	fmt.Println()
	fmt.Printf("Fleet Summary\n")
	fmt.Printf("-------------\n")
	fmt.Printf("  Total clusters:       %d\n", plan.Summary.TotalClusters)
	fmt.Printf("  Ready to migrate:     %d\n", plan.Summary.ReadyToMigrate)
	fmt.Printf("  Needs remediation:    %d\n", plan.Summary.NeedsRemediation)
	fmt.Printf("  Blocked by Windows:   %d\n", plan.Summary.BlockedByWindows)
	fmt.Println()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
