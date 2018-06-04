// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package platform

import (
	"io/ioutil"
	"log"
	"os/exec"
	"time"
)

const (

	// CNMRuntimePath is the path where CNM state files are stored.
	CNMRuntimePath = "/var/lib/azure-network/"

	// CNIRuntimePath is the path where CNM state files are stored.
	CNIRuntimePath = "/var/run/"
)

// GetOSInfo returns OS version information.
func GetOSInfo() string {
	info, err := ioutil.ReadFile("/proc/version")
	if err != nil {
		return "unknown"
	}

	return string(info)
}

// GetLastRebootTime returns the last time the system rebooted.
func GetLastRebootTime() (time.Time, error) {
	// Query last reboot time.
	out, err := exec.Command("uptime", "-s").Output()
	if err != nil {
		log.Printf("Failed to query uptime, err:%v", err)
		return time.Time{}.UTC(), err
	}

	// Parse the output.
	layout := "2006-01-02 15:04:05"
	rebootTime, err := time.ParseInLocation(layout, string(out[:len(out)-1]), time.Local)
	if err != nil {
		log.Printf("Failed to parse uptime, err:%v", err)
		return time.Time{}.UTC(), err
	}

	return rebootTime.UTC(), nil
}

// ExecuteShellCommand executes a shell command.
func ExecuteShellCommand(command string) error {
	//log.Debugf("[shell] %s", command)
	cmd := exec.Command("sh", "-c", command)
	err := cmd.Start()
	if err != nil {
		return err
	}
	return cmd.Wait()
}
