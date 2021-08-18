package cmd

import (
	"fmt"

	"github.com/Azure/azure-container-networking/npm/debugTools/dataplane/parse"
	"github.com/spf13/cobra"
)

// parseIPtableCmd represents the parseIPtable command
var parseIPtableCmd = &cobra.Command{
	Use:   "parseiptable",
	Short: "Parse iptable into Go object, dumping it to the console",
	RunE: func(cmd *cobra.Command, args []string) error {
		iptableSaveF, _ := cmd.Flags().GetString("iptF")
		if iptableSaveF == "" {
			iptable, err := parse.Iptables("filter")
			if err != nil {
				return fmt.Errorf("%w", err)
			}
			fmt.Println(iptable.String())
		} else {
			iptable, err := parse.IptablesFile("filter", iptableSaveF)
			if err != nil {
				return fmt.Errorf("%w", err)
			}
			fmt.Println(iptable.String())
		}

		return nil
	},
}

func init() {
	debugCmd.AddCommand(parseIPtableCmd)
	parseIPtableCmd.Flags().StringP("iptF", "i", "", "Set the iptable-save file path (optional)")
}
