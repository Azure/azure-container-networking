//go:build !windows
// +build !windows

// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package main

import (
	"errors"

	acn "github.com/Azure/azure-container-networking/common"
)

var (
	errServiceInstallationUnsupported   = errors.New("service installation is only supported on Windows")
	errServiceUninstallationUnsupported = errors.New("service uninstallation is only supported on Windows")
	errServiceRunUnsupported            = errors.New("running as a service is only supported on Windows")
)

func handleServiceAction(action string) (bool, error) {
	switch action {
	case acn.OptServiceInstall:
		return false, errServiceInstallationUnsupported
	case acn.OptServiceUninstall:
		return false, errServiceUninstallationUnsupported
	case acn.OptServiceRun:
		return false, errServiceRunUnsupported
	default:
		return false, nil
	}
}
