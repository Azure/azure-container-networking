// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package platform

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os/exec"
	"time"

	"github.com/Azure/azure-container-networking/log"
)

const (

	// CNMRuntimePath is the path where CNM state files are stored.
	CNMRuntimePath = "/var/lib/azure-network/"

	// CNIRuntimePath is the path where CNI state files are stored.
	CNIRuntimePath = "/var/run/"

	// CNSRuntimePath is the path where CNS state files are stored.
	CNSRuntimePath = "/var/run/"

	// NPMRuntimePath is the path where NPM logging files are stored.
	NPMRuntimePath = "/var/run/"

	// DNCRuntimePath is the path where DNC logging files are stored.
	DNCRuntimePath = "/var/run/"
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

func ExecuteCommand(command string) (string, error) {
	log.Printf("[Azure-Utils] %s", command)

	var stderr bytes.Buffer
	var out bytes.Buffer
	cmd := exec.Command("sh", "-c", command)
	cmd.Stderr = &stderr
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%s:%s", err.Error(), stderr.String())
	}

	return out.String(), nil
}

func SetOutboundSNAT(subnet string) error {
	cmd := fmt.Sprintf("iptables -t nat -A POSTROUTING -m iprange ! --dst-range 168.63.129.16 -m addrtype ! --dst-type local ! -d %v -j MASQUERADE",
		subnet)
	_, err := ExecuteCommand(cmd)
	if err != nil {
		log.Printf("SNAT Iptable rule was not set")
		return err
	}
	return nil
}
