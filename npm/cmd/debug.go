package main

import (
	"encoding/json"
	"fmt"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/pb"
	"github.com/spf13/cobra"
)

var errSpecifyBothFiles = fmt.Errorf("must specify either no files or both a cache file and an iptables save file")

type IPTablesResponse struct {
	Rules []*pb.RuleResponse `json:"rules,omitempty"`
}

func prettyPrintIPTables(iptableRules []*pb.RuleResponse) error {
	iptresponse := IPTablesResponse{
		Rules: iptableRules,
	}
	s, err := json.MarshalIndent(iptresponse, "", "  ")
	if err != nil {
		return fmt.Errorf("err pretty printing iptables")
	}
	fmt.Printf("%v", string(s))
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
