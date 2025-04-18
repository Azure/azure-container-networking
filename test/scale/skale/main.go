// This code generates KWOK Nodes for a scale test of Swift controlplane components.
// It creates the Nodes and records metrics to measure the performance.
package main

import (
	"fmt"
	"os"

	"github.com/Azure/azure-container-networking/test/scale/skale/cmd"
)

func main() {
	if err := cmd.Skale.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
