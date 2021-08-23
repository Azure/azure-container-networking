package cli

import (
	"fmt"

	"github.com/Azure/azure-container-networking/npm/pkg/debug/dataplane"
	"github.com/Azure/azure-container-networking/npm/util/errors"
	"github.com/spf13/cobra"
)

// convertIptableCmd represents the convertIptable command
var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Debug mode",
}

// getTuplesCmd represents the getTuples command
var getTuplesCmd = &cobra.Command{
	Use:   "gettuples",
	Short: "Get a list of hit rule tuples between specified source and destination",
	RunE: func(cmd *cobra.Command, args []string) error {
		src, _ := cmd.Flags().GetString("src")
		if src == "" {
			return fmt.Errorf("%w", errors.SrcNotSpecified)
		}
		dst, _ := cmd.Flags().GetString("dst")
		if dst == "" {
			return fmt.Errorf("%w", errors.DstNotSpecified)
		}
		npmCacheF, _ := cmd.Flags().GetString("npmF")
		iptableSaveF, _ := cmd.Flags().GetString("iptF")
		srcType := dataplane.GetInputType(src)
		dstType := dataplane.GetInputType(dst)
		srcInput := &dataplane.Input{Content: src, Type: srcType}
		dstInput := &dataplane.Input{Content: dst, Type: dstType}
		if npmCacheF == "" || iptableSaveF == "" {
			_, tuples, err := dataplane.GetNetworkTuple(srcInput, dstInput)
			if err != nil {
				return fmt.Errorf("%w", err)
			}
			for _, tuple := range tuples {
				fmt.Printf("%+v\n", tuple)
			}
		} else {
			_, tuples, err := dataplane.GetNetworkTupleFile(srcInput, dstInput, npmCacheF, iptableSaveF)
			if err != nil {
				return fmt.Errorf("%w", err)
			}
			for _, tuple := range tuples {
				fmt.Printf("%+v\n", tuple)
			}
		}

		return nil
	},
}
