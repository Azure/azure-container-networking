package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var errSpecifyBothFiles = fmt.Errorf("must specify either no files or both a cache file and an iptables save file")

// convertIptableCmd represents the convertIptable command
var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Debug mode",
}

func init() {
	rootCmd.AddCommand(debugCmd)
}
