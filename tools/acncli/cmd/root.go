//go:build !ignore_uncovered
// +build !ignore_uncovered

package cmd

import (
	"fmt"

	"github.com/Azure/azure-container-networking/tools/acncli/cmd/npm"

	"github.com/Azure/azure-container-networking/tools/acncli/cmd/cni"

	c "github.com/Azure/azure-container-networking/tools/acncli/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewRootCmd returns a root
func NewRootCmd(version string) *cobra.Command {
	rootCmd := &cobra.Command{
		SilenceUsage: true,
		Version:      version,
	}

	viper.New()
	viper.SetEnvPrefix(c.EnvPrefix)
	viper.AutomaticEnv()

	versionCmd := &cobra.Command{
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
	rootCmd.AddCommand(cni.CNICmd())
	rootCmd.AddCommand(npm.NPMRootCmd())
	rootCmd.SetVersionTemplate(version)
	return rootCmd
}
