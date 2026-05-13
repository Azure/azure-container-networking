// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package main

import "fmt"

// Version is populated by make during build.
var version string

// Prints description and version information.
func printVersion() {
	fmt.Printf("Azure Container Network Service\n")
	fmt.Printf("Version %v\n", version)
}
