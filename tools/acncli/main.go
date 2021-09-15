//go:build !ignore_uncovered
// +build !ignore_uncovered

package main

import (
	"github.com/Azure/azure-container-networking/tools/acncli/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	c "github.com/Azure/azure-container-networking/tools/acncli/api"
)

var (
	rootCmd *cobra.Command
	version = ""
)

func main() {
	rootCmd.Execute()
}

func init() {
	rootCmd = cmd.NewRootCmd(version)

	if version != "" {
		viper.Set(c.FlagVersion, version)
	}

	cobra.OnInitialize(func() {
		viper.AutomaticEnv()
		initCommandFlags(rootCmd.Commands())
	})
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
