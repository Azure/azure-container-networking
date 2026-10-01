//go:build !windows
// +build !windows

// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package main

import (
	"fmt"
)

// installService is not supported on non-Windows platforms
func installService() error {
	return fmt.Errorf("service installation is only supported on Windows")
}

// uninstallService is not supported on non-Windows platforms
func uninstallService() error {
	return fmt.Errorf("service uninstallation is only supported on Windows")
}

// runAsService is not supported on non-Windows platforms
func runAsService() error {
	return fmt.Errorf("running as service is only supported on Windows")
}

// isWindowsService always returns false on non-Windows platforms
func isWindowsService() (bool, error) {
	return false, nil
}
