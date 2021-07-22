package cmd

import (
	"fmt"

	processor "github.com/Azure/azure-container-networking/npm/npm-debug-cli/networkTupleProcessor"
	"github.com/spf13/cobra"
)

// getTuplesCmd represents the getTuples command
var getTuplesCmd = &cobra.Command{
	Use:   "getTuples",
	Short: "Get all tuples of hit rules between specified source and destination",
	RunE: func(cmd *cobra.Command, args []string) error {
		src, _ := cmd.Flags().GetString("src")
		if src == "" {
			return fmt.Errorf("no source specified")
		}
		dst, _ := cmd.Flags().GetString("dst")
		if dst == "" {
			return fmt.Errorf("no destination specified")
		}
		npmCacheF, _ := cmd.Flags().GetString("npmF")
		iptableSaveF, _ := cmd.Flags().GetString("iptF")
		p := &processor.Processor{}
		srcType := p.GetInputType(src)
		dstType := p.GetInputType(dst)
		srcInput := &processor.Input{Content: src, Type: srcType}
		dstInput := &processor.Input{Content: dst, Type: dstType}
		if npmCacheF == "" && iptableSaveF == "" {
			_, tuples := p.GetNetworkTuple(srcInput, dstInput)
			for _, tuple := range tuples {
				fmt.Printf("%+v\n", tuple)
			}
		} else {
			_, tuples := p.GetNetworkTuple(srcInput, dstInput, npmCacheF, iptableSaveF)
			for _, tuple := range tuples {
				fmt.Printf("%+v\n", tuple)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(getTuplesCmd)
	getTuplesCmd.Flags().StringP("src", "s", "", "set the source")
	getTuplesCmd.Flags().StringP("dst", "d", "", "set the destination")
}
