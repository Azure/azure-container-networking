package main

import (
	"fmt"

	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	"github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
	dataplane "github.com/Azure/azure-container-networking/npm/pkg/dataplane/debug"
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
			srcType := dataplane.GetInputType(src)
			dstType := dataplane.GetInputType(dst)
			srcInput := &common.Input{Content: src, Type: srcType}
			dstInput := &common.Input{Content: dst, Type: dstType}

			switch {
			case npmCacheF == "" && iptableSaveF == "":
				config := &npmconfig.Config{}
				err := viper.Unmarshal(config)
				if err != nil {
					return fmt.Errorf("failed to load config with err %w", err)
				}

				_, tuples, srcList, dstList, err := dataplane.GetNetworkTuple(srcInput, dstInput, config)
				if err != nil {
					return fmt.Errorf("%w", err)
				}

				fmt.Printf("Source IPSets:\n")
				for i := range srcList {
					fmt.Printf("\tName: %s, HashedName: %s,\n", srcList[i].Name, srcList[i].HashedSetName)
				}

				fmt.Printf("Destination IPSets:\n")
				for i := range dstList {
					fmt.Printf("\tName: %s, HashedName: %s,\n", dstList[i].Name, dstList[i].HashedSetName)
				}

				fmt.Printf("Rules:\n")
				for _, tuple := range tuples {
					fmt.Printf("%s for %s\n", tuple.Tuple.RuleType, tuple.Tuple.Direction)
					fmt.Printf("\tSource IP: %s, Port %s\n", tuple.Tuple.SrcIP, tuple.Tuple.SrcPort)
					fmt.Printf("\tDestination IP: %s, Port %s\n", tuple.Tuple.DstIP, tuple.Tuple.DstPort)
					fmt.Printf("\tProtocol: %s\n", tuple.Rule.Protocol)
					fmt.Printf("\tChain: %+v\n", tuple.Rule.Chain)
					fmt.Printf("\tSource Sets:\n")
					for _, src := range tuple.Rule.SrcList {
						fmt.Printf("\t\tName: %s\n", src.Name)
						fmt.Printf("\t\t\tHashedName: %s\n", src.HashedSetName)
						fmt.Printf("\t\t\tType: %s\n", src.Type)
						fmt.Printf("\t\t\tIncluded: %v\n", src.Included)
					}
					fmt.Printf("\tDestination Sets:\n")
					for _, dst := range tuple.Rule.DstList {
						fmt.Printf("\t\tName: %s\n", dst.Name)
						fmt.Printf("\t\t\tHashedName: %s\n", dst.HashedSetName)
						fmt.Printf("\t\t\tType: %s\n", dst.Type)
						fmt.Printf("\t\t\tIncluded: %v\n", dst.Included)
					}
				}

			case npmCacheF != "" && iptableSaveF != "":
				_, tuples, _, _, err := dataplane.GetNetworkTupleFile(srcInput, dstInput, npmCacheF, iptableSaveF)
				if err != nil {
					return fmt.Errorf("%w", err)
				}
				for _, tuple := range tuples {
					fmt.Printf("%s for %s\n", tuple.Tuple.RuleType, tuple.Tuple.Direction)
					fmt.Printf("\t Source IP: %s, Port %s", tuple.Tuple.SrcIP, tuple.Tuple.SrcPort)
					fmt.Printf("\t Destination IP: %s, Port %s", tuple.Tuple.DstIP, tuple.Tuple.DstIP)
					fmt.Printf("\t Rule: %+v", tuple.Rule)
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
