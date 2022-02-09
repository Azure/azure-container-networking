package main

import (
	"github.com/spf13/cobra"
)

// NewRootCmd returns a root cobra command
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "azure-npm",
		Short: "Collection of functions related to Azure NPM's debugging tools",
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}

	startCmd := newStartNPMCmd()

	startCmd.AddCommand(newStartNPMControlplaneCmd())
	startCmd.AddCommand(newStartNPMDaemonCmd())

	rootCmd.AddCommand(startCmd)

	rootCmd.AddCommand(newDebugCmd())

	return rootCmd
}
