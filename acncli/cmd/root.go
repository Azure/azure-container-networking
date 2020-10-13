package cmd

import (
	"fmt"

	c "github.com/Azure/azure-container-networking/acncli/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	envPrefix = "AZURE_CNI"
)

// NewRootCmd returns a root
func NewRootCmd(version string) *cobra.Command {
	var rootCmd = &cobra.Command{
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if viper.GetString(c.FlagVersion) != "" {
				fmt.Println("TEST")
			}
		},
	}

	viper.New()
	viper.SetEnvPrefix(envPrefix)
	viper.AutomaticEnv()

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version for ACN CLI",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%+s", version)
		},
	}

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(InstallCmd())
	rootCmd.AddCommand(LogsCmd())

	return rootCmd
}
