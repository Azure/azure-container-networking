// this package is not used for e2e tests, but rather
// leverage the e2e framework to deploy components for
// quick access

package main

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func main() {
	rootCmd := NewRootCmd()

	cobra.OnInitialize(func() {
		viper.AutomaticEnv()
		initCommandFlags(rootCmd.Commands())
	})

	cobra.CheckErr(rootCmd.Execute())
}

func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "acndev",
		Short: "Manual CLI for deploying specific ACN components, leveraging the e2e framework",
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			viper.AutomaticEnv() // read in environment variables that match
			return nil
		},
	}

	clusterCmd := newClusterCmd()

	rootCmd.AddCommand(clusterCmd)

	return rootCmd
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
