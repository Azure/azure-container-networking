package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var errSpecifyBothFiles = fmt.Errorf("must specify either no files or both a cache file and an iptables save file")

func newDebugCmd() *cobra.Command {
	debugCmd := &cobra.Command{
		Use:   "debug",
		Short: "Debug mode",
	}

	debugCmd.AddCommand(newParseIPTableCmd())
	debugCmd.AddCommand(newConvertIPTableCmd())
	debugCmd.AddCommand(newGetTuples())

	return debugCmd
}
