// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package main

import (
	"github.com/spf13/cobra"
)

// Version is populated by make during build.
var version string

func main() {
	rootCmd := NewRootCmd()
	cobra.CheckErr(rootCmd.Execute())
}
