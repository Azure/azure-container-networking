package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	envPrefix = "AZURE_CNI"
)

// NewRootCmd returns a root
func NewRootCmd(version string) *cobra.Command {
	var rootCmd = &cobra.Command{
		SilenceUsage: true,
	}

	viper.New()
	viper.SetEnvPrefix(envPrefix)
	viper.AutomaticEnv()

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version for ACN CLI",
		Run: func(cmd *cobra.Command, args []string) {
			if version != "" {
				fmt.Printf("%+s", version)
			} else {
				fmt.Println("Version not set.")
			}
		},
	}

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(InstallCmd())
	rootCmd.AddCommand(LogsCmd())
	rootCmd.AddCommand(ManagerCmd())

	return rootCmd
}
