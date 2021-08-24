// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package main

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/Azure/azure-container-networking/log"
	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/klog"
)

// Version is populated by make during build.
var (
	version string
	cfgFile string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "azure-npm",
	Short: "Collection of functions related to Azure NPM's debugging tools",

	RunE: func(cmd *cobra.Command, args []string) error {
		config := &npmconfig.Config{}
		err := viper.Unmarshal(config)
		if err != nil {
			fmt.Printf("unable to decode into config struct, %v", err)
		}

		Start(*config)
		return nil
	},
}

func initLogging() error {
	log.SetName("azure-npm")
	log.SetLevel(log.LevelInfo)
	if err := log.SetTargetLogDirectory(log.TargetStdout, ""); err != nil {
		log.Logf("Failed to configure logging, err:%v.", err)
		return fmt.Errorf("%w", err)
	}

	return nil
}

func main() {
	klog.Infof("Start NPM version: %s", version)
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.Flags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.azure-npm-debug-cli.yaml)")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigFile(npmconfig.GetConfigPath())
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		klog.Error("Using config file:", viper.ConfigFileUsed())
	} else {
		klog.Error(err)
		klog.Info("Using default config")
		b, _ := json.Marshal(npmconfig.DefaultConfig)
		err := viper.ReadConfig(bytes.NewBuffer(b))
		cobra.CheckErr(err)
	}
}
