package cmd

import (
	"fmt"

	converter "github.com/Azure/azure-container-networking/npm/npm-debug-cli/dataplaneConverter"
	"github.com/spf13/cobra"
)

// convertIptableCmd represents the convertIptable command
var convertIptableCmd = &cobra.Command{
	Use:   "convertIptable",
	Short: "View iptable's rules in JSON format",
	RunE: func(cmd *cobra.Command, args []string) error {
		iptableName, _ := cmd.Flags().GetString("table")
		if iptableName == "" {
			iptableName = "filter"
		}
		npmCacheF, _ := cmd.Flags().GetString("npmF")
		iptableSaveF, _ := cmd.Flags().GetString("iptF")
		c := &converter.Converter{}
		if npmCacheF == "" && iptableSaveF == "" {
			ipTableRulesRes := c.GetJSONRulesFromIptable(iptableName)
			fmt.Printf("%s\n", ipTableRulesRes)
		} else {
			ipTableRulesRes := c.GetJSONRulesFromIptable(iptableName, npmCacheF, iptableSaveF)
			fmt.Printf("%s\n", ipTableRulesRes)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(convertIptableCmd)
}
