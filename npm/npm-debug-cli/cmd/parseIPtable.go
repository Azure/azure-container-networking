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

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// parseIPtableCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:

}
