//go:build !ignore_uncovered
// +build !ignore_uncovered

package cni

import (
	c "github.com/Azure/azure-container-networking/tools/acncli/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewRootCmd returns a root
func CNICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cni",
		Short: "Collection of functions related to Azure CNI",
	}

	viper.New()
	viper.SetEnvPrefix(c.EnvPrefix)
	viper.AutomaticEnv()

	cmd.AddCommand(InstallCmd())
	cmd.AddCommand(LogsCmd())
	cmd.AddCommand(ManagerCmd())
	return cmd
}
