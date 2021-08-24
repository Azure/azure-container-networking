package main

import (
	"fmt"

	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "azure-npm",
	Short: "Collection of functions related to Azure NPM's debugging tools",

	RunE: func(cmd *cobra.Command, args []string) error {
		config := &npmconfig.Config{}
		err := viper.Unmarshal(config)
		if err != nil {
			return fmt.Errorf("failed to load config with error %w", err)
		}

		return Start(*config)
	},
}
