package main

import (
	"fmt"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/pb"
	"github.com/spf13/cobra"
)

var errSpecifyBothFiles = fmt.Errorf("must specify either no files or both a cache file and an iptables save file")

type IPTablesResponse struct {
	Rules map[*pb.RuleResponse]struct{} `json:"rules,omitempty"`
}

func prettyPrintIPTables(iptableRules map[*pb.RuleResponse]struct{}) error {
	iptresponse := IPTablesResponse{
		Rules: iptableRules,
	}

	fmt.Printf("%+v", iptresponse)
	return nil
}

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
