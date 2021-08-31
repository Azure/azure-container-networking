package main

import (
	"bytes"
	"encoding/json"
	"fmt"

	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/klog"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "azure-npm",
	Short: "Collection of functions related to Azure NPM's debugging tools",
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
	PreRunE: func(cmd *cobra.Command, args []string) error {
		viper.AutomaticEnv() // read in environment variables that match
		cfgFile := viper.GetString(npmconfig.ConfigEnvPath)
		if cfgFile != "" {
			// Use config file from the flag.
			viper.SetConfigFile(cfgFile)
		} else {
			viper.SetConfigFile(npmconfig.GetConfigPath())
		}

		// If a config file is found, read it in.
		if err := viper.ReadInConfig(); err == nil {
			klog.Info("Using config file: ", viper.ConfigFileUsed())
		} else {
			klog.Info("Using default config")
			b, _ := json.Marshal(npmconfig.DefaultConfig)
			err := viper.ReadConfig(bytes.NewBuffer(b))
			if err != nil {
				return fmt.Errorf("failed to read in default with err %w", err)
			}
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		config := &npmconfig.Config{}
		err := viper.Unmarshal(config)
		if err != nil {
			return fmt.Errorf("failed to load config with error %w", err)
		}

		return Start(*config)
	},
}
