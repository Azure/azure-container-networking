package cmd

import (
	"github.com/Azure/azure-container-networking/npm/npm-debug-cli/dataplaneParser/parser"
	"github.com/spf13/cobra"
)

// parseIPtableCmd represents the parseIPtable command
var parseIPtableCmd = &cobra.Command{
	Use:   "parseIPtable",
	Short: "Parse iptable into Go object, dumping it to the console",
	RunE: func(cmd *cobra.Command, args []string) error {
		iptableName, _ := cmd.Flags().GetString("table")
		if iptableName == "" {
			iptableName = "filter"
		}
		iptableSaveF, _ := cmd.Flags().GetString("iptF")
		p := &parser.Parser{}
		if iptableSaveF == "" {
			iptable := p.ParseIptablesObject(iptableName)
			iptable.PrintIptable()
		} else {
			iptable := p.ParseIptablesObject(iptableName, iptableSaveF)
			iptable.PrintIptable()
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(parseIPtableCmd)
}
