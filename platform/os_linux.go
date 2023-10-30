// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package platform

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/log"
	"go.uber.org/zap"
)

const (
	// CNMRuntimePath is the path where CNM state files are stored.
	CNMRuntimePath = "/var/lib/azure-network/"
	// CNIRuntimePath is the path where CNI state files are stored.
	CNIRuntimePath = "/var/run/"
	// CNILockPath is the path where CNI lock files are stored.
	CNILockPath = "/var/run/azure-vnet/"
	// CNIStateFilePath is the path to the CNI state file
	CNIStateFilePath = "/var/run/azure-vnet.json"
	// CNIIpamStatePath is the name of IPAM state file
	CNIIpamStatePath = "/var/run/azure-vnet-ipam.json"
	// CNIBinaryPath is the path to the CNI binary
	CNIBinaryPath = "/opt/cni/bin/azure-vnet"
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
	info, err := os.ReadFile("/proc/version")
	if err != nil {
		return "unknown"
	}

	return string(info)
}

func GetProcessSupport() error {
	p := NewExecClient(nil)
	cmd := fmt.Sprintf("ps -p %v -o comm=", os.Getpid())
	_, err := p.ExecuteCommand(cmd)
	return err
}

// GetLastRebootTime returns the last time the system rebooted.
func (p *execClient) GetLastRebootTime() (time.Time, error) {
	// Query last reboot time.
	out, err := exec.Command("uptime", "-s").Output()
	if err != nil {
		if p.logger != nil {
			p.logger.Error("Failed to query uptime", zap.Error(err))
		} else {
			log.Printf("Failed to query uptime, err:%v", err)
		}
		return time.Time{}.UTC(), err
	}

	// Parse the output.
	layout := "2006-01-02 15:04:05"
	rebootTime, err := time.ParseInLocation(layout, string(out[:len(out)-1]), time.Local)
	if err != nil {
		if p.logger != nil {
			p.logger.Error("Failed to parse uptime", zap.Error(err))
		} else {
			log.Printf("Failed to parse uptime, err:%v", err)
		}
		return time.Time{}.UTC(), err
	}

	return rebootTime.UTC(), nil
}

func (p *execClient) ExecuteCommand(command string) (string, error) {
	if p.logger != nil {
		p.logger.Info("[Azure-Utils]", zap.String("command", command))
	} else {
		log.Printf("[Azure-Utils] %s", command)
	}

	var stderr bytes.Buffer
	var out bytes.Buffer

	// Create a new context and add a timeout to it
	ctx, cancel := context.WithTimeout(context.Background(), p.Timeout)
	defer cancel() // The cancel should be deferred so resources are cleaned up

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Stderr = &stderr
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%s:%s", err.Error(), stderr.String())
	}

	return out.String(), nil
}

func SetOutboundSNAT(subnet string) error {
	p := NewExecClient(nil)
	cmd := fmt.Sprintf("iptables -t nat -A POSTROUTING -m iprange ! --dst-range 168.63.129.16 -m addrtype ! --dst-type local ! -d %v -j MASQUERADE",
		subnet)
	_, err := p.ExecuteCommand(cmd)
	if err != nil {
		log.Printf("SNAT Iptable rule was not set")
		return err
	}
	return nil
}

// ClearNetworkConfiguration clears the azure-vnet.json contents.
// This will be called only when reboot is detected - This is windows specific
func (p *execClient) ClearNetworkConfiguration() (bool, error) {
	return false, nil
}

func (p *execClient) ExecutePowershellCommand(_ string) (string, error) {
	return "", nil
}

func (p *execClient) KillProcessByName(processName string) error {
	cmd := fmt.Sprintf("pkill -f %v", processName)
	_, err := p.ExecuteCommand(cmd)
	return err
}

// SetSdnRemoteArpMacAddress sets the regkey for SDNRemoteArpMacAddress needed for multitenancy
// This operation is specific to windows OS
func SetSdnRemoteArpMacAddress() error {
	return nil
}

func GetOSDetails() (map[string]string, error) {
	linesArr, err := ReadFileByLines(osReleaseFile)
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

func GetProcessNameByID(pidstr string) (string, error) {
	p := NewExecClient(nil)
	pidstr = strings.Trim(pidstr, "\n")
	cmd := fmt.Sprintf("ps -p %s -o comm=", pidstr)
	out, err := p.ExecuteCommand(cmd)
	if err != nil {
		log.Printf("GetProcessNameByID returned error: %v", err)
		return "", err
	}

	out = strings.Trim(out, "\n")
	out = strings.TrimSpace(out)

	return out, nil
}

func PrintDependencyPackageDetails() {
	p := NewExecClient(nil)
	out, err := p.ExecuteCommand("iptables --version")
	out = strings.TrimSuffix(out, "\n")
	log.Printf("[cni-net] iptable version:%s, err:%v", out, err)
}

func ReplaceFile(source, destination string) error {
	return os.Rename(source, destination)
}

// Mellanox adapter not applicable for linux
func HasMellanoxAdapter() bool {
	return false
}

// Not needed for Linux
func MonitorAndSetMellanoxRegKeyPriorityVLANTag(_ context.Context, _ int) {}
