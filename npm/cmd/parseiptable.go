package main

import (
	"fmt"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/parse"
	"github.com/spf13/cobra"
)

func newParseIPTableCmd() *cobra.Command {
	parseIPTableCmd := &cobra.Command{
		Use:   "parseiptable",
		Short: "Parse iptable into Go object, dumping it to the console",
		RunE: func(cmd *cobra.Command, args []string) error {
			iptableSaveF, _ := cmd.Flags().GetString("iptables-file")
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

	parseIPTableCmd.Flags().StringP("iptables-file", "i", "", "Set the iptable-save file path (optional)")
	return parseIPTableCmd
}
