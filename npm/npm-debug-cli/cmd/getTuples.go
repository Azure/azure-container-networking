/*
Copyright © 2021 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
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

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// getTuplesCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	getTuplesCmd.Flags().StringP("src", "s", "", "set the source")
	getTuplesCmd.Flags().StringP("dst", "d", "", "set the destination")
}
