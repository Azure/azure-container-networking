// This code generates KWOK Nodes for a scale test of Swift controlplane components.
// It creates the Nodes and records metrics to measure the performance.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := rootcmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
