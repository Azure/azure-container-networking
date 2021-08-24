// Copyright 2018 Microsoft. All rights reserved.
// MIT License
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

// Version is populated by make during build.
var (
	version string
	cfgFile string
)

func main() {
	klog.Infof("Start NPM version: %s", version)
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize(func() {
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
	})

	rootCmd.Flags().StringVar(&cfgFile, "config", "", fmt.Sprintf("Manually specify config file (default path is %s)", npmconfig.GetConfigPath()))
}
