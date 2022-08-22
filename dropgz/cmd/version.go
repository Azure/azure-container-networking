package cmd

import (
	"fmt"

	"github.com/Azure/azure-container-networking/dropgz/internal/buildinfo"
	"github.com/spf13/cobra"
)

// version command.
var version = &cobra.Command{
	Use: "version",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := setLogLevel(); err != nil {
			return err
		}
		fmt.Println(buildinfo.Version)
		return nil
	},
}

func init() {
	root.AddCommand(version)
}
