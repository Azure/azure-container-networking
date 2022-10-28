// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package main

import (
	"runtime/debug"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"k8s.io/klog"
)

const (
	flagVersion        = "version"
	flagKubeConfigPath = "kubeconfig"
)

var flagDefaults = map[string]string{
	flagKubeConfigPath: "",
}

// Version is populated by make during build.
var version string

// panicRecoverAndExitWithStackTrace - recovery from panic, print a failure message and stack trace and exit the program
func panicRecoverAndExitWithStackTrace() {
	if r := recover(); r != nil {
		klog.Infoln(r)
		klog.Errorf("Stack trace: %s", string(debug.Stack()))
	}
}

func main() {
	defer panicRecoverAndExitWithStackTrace()

	rootCmd := NewRootCmd()

	if version != "" {
		viper.Set(flagVersion, version)
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
		err := viper.BindPFlags(cmd.Flags())
		cobra.CheckErr(err)

		c := cmd
		c.Flags().VisitAll(func(flag *pflag.Flag) {
			if viper.IsSet(flag.Name) && viper.GetString(flag.Name) != "" {
				err := c.Flags().Set(flag.Name, viper.GetString(flag.Name))
				cobra.CheckErr(err)
			}
		})

		// call recursively on subcommands
		if cmd.HasSubCommands() {
			initCommandFlags(cmd.Commands())
		}
	}
}
