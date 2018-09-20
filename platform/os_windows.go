// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package platform

import (
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const (

	// CNMRuntimePath is the path where CNM state files are stored.
	CNMRuntimePath = ""

	// CNIRuntimePath is the path where CNM state files are stored.
	CNIRuntimePath = ""

	// NPMRuntimePath is the path where NPM state files are stored.
	NPMRuntimePath = ""

	// DNCRuntimePath is the path where NPM state files are stored.
	DNCRuntimePath = ""
)

// GetOSInfo returns OS version information.
func GetOSInfo() string {
	return "windows"
}

var (
	modkernel32        = windows.NewLazySystemDLL("kernel32.dll")
	procGetTickCount64 = modkernel32.NewProc("GetTickCount64")
)

// GetLastRebootTime returns the last time the system rebooted in UTC.
func GetLastRebootTime() (time.Time, error) {

	// @jhowardmsft - These following lines are the original non-implementation.
	// I have kept these here, as by introducing a real implementation of this
	// function (which I have below) has side effects which I have no means
	// of being able to verify correct operation, have to leave that to the
	// owners of this repo to verify.
	var rebootTime time.Time
	return rebootTime, nil

	// This is a Windows implementation of returning the UTC time of when the
	// host was booted. See note above - it is no-op'd by the two lines above.
	nowUTC := time.Now().UTC()
	r0, _, _ := syscall.Syscall(procGetTickCount64.Addr(), 0, 0, 0, 0)
	tickCount := uint64(r0)
	upTimeUTC := time.Duration(tickCount) * time.Millisecond
	return nowUTC.Add(-upTimeUTC), nil
}

func ExecuteCommand(command string) (string, error) {
	return "", nil
}

func SetOutboundSNAT(subnet string) error {
	return nil
}
