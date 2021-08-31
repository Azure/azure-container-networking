// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package main

import (
	"bytes"
	"encoding/json"

	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/klog"
)

// Version is populated by make during build.
var version string

func main() {
	klog.Infof("Start NPM version: %s", version)
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize(func() {
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
			klog.Error("Using config file: ", viper.ConfigFileUsed())
		} else {
			klog.Error(err)
			klog.Info("Using default config")
			b, _ := json.Marshal(npmconfig.DefaultConfig)
			err := viper.ReadConfig(bytes.NewBuffer(b))
			cobra.CheckErr(err)
		}
	})
}
