// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package routes

import (
	"os"
	"testing"
)

const (
	testDest    = "159.254.169.254"
	testMask    = "255.255.255.255"
	testGateway = "0.0.0.0"
	testMetric  = "12"
)

// Wraps the test run.
func TestMain(m *testing.M) {
	// Run tests.
	exitCode := m.Run()
	os.Exit(exitCode)
}
