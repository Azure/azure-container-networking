package cmd

import (
	"github.com/spf13/cobra"
)

// convertIptableCmd represents the convertIptable command
var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Debug mode",
}

func init() {
	rootCmd.AddCommand(debugCmd)
}
