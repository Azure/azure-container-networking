package main

import (
	"fmt"

	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	"github.com/Azure/azure-container-networking/npm/http/api"
	"github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/debug"
	"github.com/Azure/azure-container-networking/npm/util/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
			srcType := common.GetInputType(src)
			dstType := common.GetInputType(dst)
			srcInput := &common.Input{Content: src, Type: srcType}
			dstInput := &common.Input{Content: dst, Type: dstType}

			config := &npmconfig.Config{}
			err := viper.Unmarshal(config)
			if err != nil {
				return fmt.Errorf("failed to load config with err %w", err)
			}

			switch {
			case npmCacheF == "" && iptableSaveF == "":

				c := &debug.Converter{
					NPMDebugEndpointHost: "http://localhost",
					NPMDebugEndpointPort: api.DefaultHttpPort,
					EnableV2NPM:          config.Toggles.EnableV2NPM, // todo: pass this a different way than param to this
				}

				_, tuples, srcList, dstList, err := c.GetNetworkTuple(srcInput, dstInput, config)
				if err != nil {
					return fmt.Errorf("%w", err)
				}

				debug.PrettyPrintTuples(tuples, srcList, dstList)

			case npmCacheF != "" && iptableSaveF != "":

				c := &debug.Converter{
					EnableV2NPM: config.Toggles.EnableV2NPM,
				}

				_, tuples, srcList, dstList, err := c.GetNetworkTupleFile(srcInput, dstInput, npmCacheF, iptableSaveF)
				if err != nil {
					return fmt.Errorf("%w", err)
				}

				debug.PrettyPrintTuples(tuples, srcList, dstList)

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
