package cmd

import (
	"fmt"

	c "github.com/Azure/azure-container-networking/acncli/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewRootCmd returns a root
func NewRootCmd(version string) *cobra.Command {
	var rootCmd = &cobra.Command{
		SilenceUsage: true,
		Version:      version,
	}

	viper.New()
	viper.SetEnvPrefix(c.EnvPrefix)
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
	rootCmd.SetVersionTemplate(version)
	return rootCmd
}
