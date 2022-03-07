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

// NewRootCmd returns a root cobra command
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "azure-npm",
		Short: "Collection of functions related to Azure NPM's debugging tools",
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			viper.AutomaticEnv() // read in environment variables that match
			viper.SetDefault(npmconfig.ConfigEnvPath, npmconfig.GetConfigPath())
			cfgFile := viper.GetString(npmconfig.ConfigEnvPath)
			viper.SetConfigFile(cfgFile)

			// If a config file is found, read it in.
			// NOTE: there is no config merging with default, if config is loaded, options must be set
			if err := viper.ReadInConfig(); err == nil {
				klog.Infof("Using config file: %+v", viper.ConfigFileUsed())
			} else {
				klog.Infof("Failed to load config from env %s: %v", npmconfig.ConfigEnvPath, err)
				b, _ := json.Marshal(npmconfig.DefaultConfig) //nolint // skip checking error
				err := viper.ReadConfig(bytes.NewBuffer(b))
				if err != nil {
					return fmt.Errorf("failed to read in default with err %w", err)
				}
			}

			return nil
		},
	}

	startCmd := newStartNPMCmd()

	rootCmd.AddCommand(newStartNPMControlplaneCmd())
	rootCmd.AddCommand(newStartNPMDaemonCmd())
	rootCmd.AddCommand(startCmd)

	rootCmd.AddCommand(newDebugCmd())

	return rootCmd
}
