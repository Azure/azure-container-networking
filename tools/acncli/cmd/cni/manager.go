//go:build !ignore_uncovered
// +build !ignore_uncovered

package cni

import (
	"github.com/spf13/cobra"
)

// ManagerCmd starts the manager mode, which installs CNI+Conflists, then watches logs
func ManagerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manager",
		Short: "Starts the ACN CNI manager, which installs CNI, sets up conflists, then starts watching logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := InstallCNICmd().RunE(cmd, args)
			if err != nil {
				return err
			}

			err = LogsCNICmd().RunE(cmd, args)
			return err
		},
	}

	cmd.Flags().AddFlagSet(InstallCNICmd().Flags())
	cmd.Flags().AddFlagSet(LogsCNICmd().Flags())

	return cmd
}
