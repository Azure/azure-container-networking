package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/azure/aksmigrate/cmd/aksmigrate/subcmd"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "aksmigrate",
		Short: "AKS cluster migration toolkit",
		Long: `aksmigrate is a toolkit for planning and executing migrations on Azure
Kubernetes Service (AKS) clusters.

Currently supported migrations:
  - NPM (Azure Network Policy Manager) to Azure CNI powered by Cilium

Commands:
  audit       Analyze NetworkPolicies for migration incompatibilities
  translate   Patch policies and generate Cilium-compatible equivalents
  conntest    Pre/post migration connectivity validation
  discover    Find and prioritize clusters for migration across subscriptions
  migrate     Orchestrate a full end-to-end migration for a single cluster`,
		SilenceUsage: true,
		Version:      "0.1.0",
	}

	rootCmd.AddCommand(subcmd.NewAuditCmd())
	rootCmd.AddCommand(subcmd.NewTranslateCmd())
	rootCmd.AddCommand(subcmd.NewConntestCmd())
	rootCmd.AddCommand(subcmd.NewDiscoverCmd())
	rootCmd.AddCommand(subcmd.NewMigrateCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
