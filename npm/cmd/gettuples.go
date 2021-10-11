package main

import (
	"fmt"

	dataplane "github.com/Azure/azure-container-networking/npm/pkg/dataplane/debug"
	"github.com/Azure/azure-container-networking/npm/util/errors"
	"github.com/spf13/cobra"
)

func newGetTuples() *cobra.Command {
	getTuplesCmd := &cobra.Command{
		Use:   "gettuples",
		Short: "Get a list of hit rule tuples between specified source and destination",
		RunE: func(cmd *cobra.Command, args []string) error {
			src, _ := cmd.Flags().GetString("src")
			if src == "" {
				return fmt.Errorf("%w", errors.ErrSrcNotSpecified)
			}
			dst, _ := cmd.Flags().GetString("dst")
			if dst == "" {
				return fmt.Errorf("%w", errors.ErrDstNotSpecified)
			}
			npmCacheF, _ := cmd.Flags().GetString("cache-file")
			iptableSaveF, _ := cmd.Flags().GetString("iptables-file")
			srcType := dataplane.GetInputType(src)
			dstType := dataplane.GetInputType(dst)
			srcInput := &dataplane.Input{Content: src, Type: srcType}
			dstInput := &dataplane.Input{Content: dst, Type: dstType}

			switch {
			case npmCacheF == "" && iptableSaveF == "":
				_, tuples, err := dataplane.GetNetworkTuple(srcInput, dstInput)
				if err != nil {
					return fmt.Errorf("%w", err)
				}
				for _, tuple := range tuples {
					fmt.Printf("%+v\n", tuple)
				}
			case npmCacheF != "" && iptableSaveF != "":
				_, tuples, err := dataplane.GetNetworkTupleFile(srcInput, dstInput, npmCacheF, iptableSaveF)
				if err != nil {
					return fmt.Errorf("%w", err)
				}
				for _, tuple := range tuples {
					fmt.Printf("%+v\n", tuple)
				}
			default:
				return errSpecifyBothFiles
			}

			return nil
		},
	}

	getTuplesCmd.Flags().StringP("src", "s", "", "set the source")
	getTuplesCmd.Flags().StringP("dst", "d", "", "set the destination")
	getTuplesCmd.Flags().StringP("iptables-file", "i", "", "Set the iptable-save file path (optional, but required when using a cache file)")
	getTuplesCmd.Flags().StringP("cache-file", "c", "", "Set the NPM cache file path (optional, but required when using an iptables save file)")

	return getTuplesCmd
}
