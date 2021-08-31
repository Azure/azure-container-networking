package main

import (
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "azure-npm",
	Short: "Collection of functions related to Azure NPM's debugging tools",
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
}
