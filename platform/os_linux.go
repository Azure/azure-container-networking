// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package platform

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os/exec"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
)

const (
	// CNMRuntimePath is the path where CNM state files are stored.
	CNMRuntimePath = "/var/lib/azure-network/"
	// CNIRuntimePath is the path where CNI state files are stored.
	CNIRuntimePath = "/var/run/"
	// CNSRuntimePath is the path where CNS state files are stored.
	CNSRuntimePath = "/var/run/"
	// CNI runtime path on a Kubernetes cluster
	K8SCNIRuntimePath = "/opt/cni/bin"
	// Network configuration file path on a Kubernetes cluster
	K8SNetConfigPath = "/etc/cni/net.d"
	// NPMRuntimePath is the path where NPM logging files are stored.
	NPMRuntimePath = "/var/run/"
	// DNCRuntimePath is the path where DNC logging files are stored.
	DNCRuntimePath = "/var/run/"
	// This file contains OS details
	osReleaseFile = "/etc/os-release"
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

// ClearNetworkConfiguration clears the azure-vnet.json contents.
// This will be called only when reboot is detected - This is windows specific
func ClearNetworkConfiguration() (bool, error) {
	return false, nil
}

func KillProcessByName(processName string) error {
	cmd := fmt.Sprintf("pkill -f %v", processName)
	_, err := ExecuteCommand(cmd)
	return err
}

// SetSdnRemoteArpMacAddress sets the regkey for SDNRemoteArpMacAddress needed for multitenancy
// This operation is specific to windows OS
func SetSdnRemoteArpMacAddress() error {
	return nil
}

func GetOSDetails() (map[string]string, error) {
	linesArr, err := common.ReadFileByLines(osReleaseFile)
	if err != nil || len(linesArr) <= 0 {
		return nil, err
	}

	osInfoArr := make(map[string]string)

	for i := range linesArr {
		s := strings.Split(linesArr[i], "=")
		if len(s) == 2 {
			osInfoArr[s[0]] = strings.TrimSuffix(s[1], "\n")
		}
	}

	return osInfoArr, nil
}
