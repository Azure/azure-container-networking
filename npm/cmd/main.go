// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package main

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	FlagVersion        = "version"
	FlagKubeConfigPath = "kubeconfig"
)

var (
	FlagDefaults = map[string]string{
		FlagKubeConfigPath: "",
	}
)

// Version is populated by make during build.
var version string

func main() {
	rootCmd := NewRootCmd()

	if version != "" {
		viper.Set(FlagVersion, version)
	}

	cobra.OnInitialize(func() {
		viper.AutomaticEnv()
		initCommandFlags(rootCmd.Commands())
	})

	cobra.CheckErr(rootCmd.Execute())
}

func initCommandFlags(commands []*cobra.Command) {
	for _, cmd := range commands {
		// bind vars from env or conf to pflags
		viper.BindPFlags(cmd.Flags())
		cmd.Flags().VisitAll(func(flag *pflag.Flag) {
			if viper.IsSet(flag.Name) && viper.GetString(flag.Name) != "" {
				cmd.Flags().Set(flag.Name, viper.GetString(flag.Name))
			}
		})

		// call recursively on subcommands
		if cmd.HasSubCommands() {
			initCommandFlags(cmd.Commands())
		}
	}
}
