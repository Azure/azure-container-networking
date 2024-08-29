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
	"syscall"
	"time"

	"github.com/Azure/azure-container-networking/log"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/sys/windows/registry"
)

const (
	// CNMRuntimePath is the path where CNM state files are stored.
	CNMRuntimePath = ""

	// CNIRuntimePath is the path where CNI state files are stored.
	CNIRuntimePath = ""

	// CNILockPath is the path where CNI lock files are stored.
	CNILockPath = ""

	// CNIStateFilePath is the path to the CNI state file
	CNIStateFilePath = "C:\\k\\azure-vnet.json"

	// CNIIpamStatePath is the name of IPAM state file
	CNIIpamStatePath = "C:\\k\\azure-vnet-ipam.json"

	// CNIBinaryPath is the path to the CNI binary
	CNIBinaryPath = "C:\\k\\azurecni\\bin\\azure-vnet.exe"

	// CNI runtime path on a Kubernetes cluster
	K8SCNIRuntimePath = "C:\\k\\azurecni\\bin"

	// Network configuration file path on a Kubernetes cluster
	K8SNetConfigPath = "C:\\k\\azurecni\\netconf"

	// CNSRuntimePath is the path where CNS state files are stored.
	CNSRuntimePath = ""

	// NPMRuntimePath is the path where NPM state files are stored.
	NPMRuntimePath = ""

	// DNCRuntimePath is the path where DNC state files are stored.
	DNCRuntimePath = ""

	// SDNRemoteArpMacAddress is the registry key for the remote arp mac address.
	// This is set for multitenancy to get arp response from within VM
	// for vlan tagged arp requests
	SDNRemoteArpMacAddress = "12-34-56-78-9a-bc"

	// Command to fetch netadapter and pnp id
	//TODO can we replace this (and things in endpoint_windows) with "golang.org/x/sys/windows"
	//var adapterInfo windows.IpAdapterInfo
	//var bufferSize uint32 = uint32(unsafe.Sizeof(adapterInfo))
	GetMacAddressVFPPnpIDMapping = "Get-NetAdapter | Select-Object MacAddress, PnpDeviceID| Format-Table -HideTableHeaders"

	// Interval between successive checks for mellanox adapter's PriorityVLANTag value
	defaultMellanoxMonitorInterval = 30 * time.Second

	// Value for reg key: PriorityVLANTag for adapter
	// reg key value for PriorityVLANTag = 3  --> Packet priority and VLAN enabled
	// for more details goto https://learn.microsoft.com/en-us/windows-hardware/drivers/network/standardized-inf-keywords-for-ndis-qos
	desiredVLANTagForMellanox = 3
	// Powershell command timeout
	ExecTimeout = 10 * time.Second
)

// Flag to check if sdnRemoteArpMacAddress registry key is set
var sdnRemoteArpMacAddressSet = false

// GetOSInfo returns OS version information.
func GetOSInfo() string {
	return "windows"
}

func GetProcessSupport() error {
	p := NewExecClient(nil)
	cmd := fmt.Sprintf("Get-Process -Id %v", os.Getpid())
	_, err := p.ExecutePowershellCommand(cmd)
	return err
}

var tickCount = syscall.NewLazyDLL("kernel32.dll").NewProc("GetTickCount64")

// GetLastRebootTime returns the last time the system rebooted.
func (p *execClient) GetLastRebootTime() (time.Time, error) {
	currentTime := time.Now()
	output, _, err := tickCount.Call()
	if errno, ok := err.(syscall.Errno); !ok || errno != 0 {
		if p.logger != nil {
			p.logger.Error("Failed to call GetTickCount64", zap.Error(err))
		} else {
			log.Printf("Failed to call GetTickCount64, err: %v", err)
		}
		return time.Time{}.UTC(), err
	}
	rebootTime := currentTime.Add(-time.Duration(output) * time.Millisecond).Truncate(time.Second)
	if p.logger != nil {
		p.logger.Info("Formatted Boot", zap.String("time", rebootTime.Format(time.RFC3339)))
	} else {
		log.Printf("Formatted Boot time: %s", rebootTime.Format(time.RFC3339))
	}
	return rebootTime.UTC(), nil
}

// Deprecated: ExecuteRawCommand is deprecated, it is recommended to use ExecuteCommand when possible
func (p *execClient) ExecuteRawCommand(command string) (string, error) {
	if p.logger != nil {
		p.logger.Info("[Azure-Utils]", zap.String("ExecuteRawCommand", command))
	} else {
		log.Printf("[Azure-Utils] ExecuteRawCommand: %q", command)
	}

	var stderr, stdout bytes.Buffer

	cmd := exec.Command("cmd", "/c", command)
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", errors.Wrapf(err, "ExecuteRawCommand failed. stdout: %q, stderr: %q", stdout.String(), stderr.String())
	}

	return stdout.String(), nil
}

// ExecuteCommand passes its parameters to an exec.CommandContext, runs the command, and returns its output, or an error if the command fails or times out
func (p *execClient) ExecuteCommand(ctx context.Context, command string, args ...string) (string, error) {
	if p.logger != nil {
		p.logger.Info("[Azure-Utils]", zap.String("ExecuteCommand", command), zap.Strings("args", args))
	} else {
		log.Printf("[Azure-Utils] ExecuteCommand: %q %v", command, args)
	}

	var stderr, stdout bytes.Buffer

	// Create a new context and add a timeout to it
	derivedCtx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel() // The cancel should be deferred so resources are cleaned up

	cmd := exec.CommandContext(derivedCtx, command, args...)
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", errors.Wrapf(err, "ExecuteCommand failed. stdout: %q, stderr: %q", stdout.String(), stderr.String())
	}

	return stdout.String(), nil
}

func SetOutboundSNAT(subnet string) error {
	return nil
}

// ClearNetworkConfiguration clears the azure-vnet.json contents.
// This will be called only when reboot is detected - This is windows specific
func (p *execClient) ClearNetworkConfiguration() (bool, error) {
	jsonStore := CNIRuntimePath + "azure-vnet.json"
	p.logger.Info("Deleting the json", zap.String("store", jsonStore))
	cmd := exec.Command("cmd", "/c", "del", jsonStore)

	if err := cmd.Run(); err != nil {
		p.logger.Info("Error deleting the json", zap.String("store", jsonStore))
		return true, err
	}

	return true, nil
}

func (p *execClient) KillProcessByName(processName string) error {
	cmd := fmt.Sprintf("taskkill /IM %v /F", processName)
	_, err := p.ExecuteRawCommand(cmd)
	return err // nolint
}

// ExecutePowershellCommand executes powershell command
// Deprecated: ExecutePowershellCommand is deprecated, it is recommended to use ExecuteCommand when possible
func (p *execClient) ExecutePowershellCommand(command string) (string, error) {
	ps, err := exec.LookPath("powershell.exe")
	if err != nil {
		return "", fmt.Errorf("Failed to find powershell executable")
	}

	if p.logger != nil {
		p.logger.Info("[Azure-Utils]", zap.String("command", command))
	} else {
		log.Printf("[Azure-Utils] %s", command)
	}

	cmd := exec.Command(ps, command)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%s:%s", err.Error(), stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// ExecutePowershellCommandWithContext executes powershell command wth context
// Deprecated: ExecutePowershellCommandWithContext is deprecated, it is recommended to use ExecuteCommand when possible
func (p *execClient) ExecutePowershellCommandWithContext(ctx context.Context, command string) (string, error) {
	ps, err := exec.LookPath("powershell.exe")
	if err != nil {
		return "", errors.New("failed to find powershell executable")
	}

	if p.logger != nil {
		p.logger.Info("[Azure-Utils]", zap.String("command", command))
	} else {
		log.Printf("[Azure-Utils] %s", command)
	}

	cmd := exec.CommandContext(ctx, ps, command)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		ErrPowershellExecution := errors.New("failed to execute powershell command")
		return "", fmt.Errorf("%w:%s", ErrPowershellExecution, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// SetSdnRemoteArpMacAddress sets the regkey for SDNRemoteArpMacAddress needed for multitenancy if hns is enabled
func SetSdnRemoteArpMacAddress(registerClient RegistryClient) error {
	key, err := registerClient.OpenKey(registry.LOCAL_MACHINE, "SYSTEM\\CurrentControlSet\\Services\\hns\\State", registry.READ|registry.SET_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			log.Printf("hns state path does not exist, skip setting SdnRemoteArpMacAddress")
			return nil
		}
		errMsg := fmt.Sprintf("Failed to check the existent of hns state path due to error %s", err.Error())
		log.Printf(errMsg)
		return errors.Errorf(errMsg)
	}

	if key == nil {
		log.Printf("hns state path does not exist, skip setting SdnRemoteArpMacAddress")
		return nil
	}

	if sdnRemoteArpMacAddressSet == false {

		//Was (Get-ItemProperty -Path HKLM:\\SYSTEM\\CurrentControlSet\\Services\\hns\\State -Name SDNRemoteArpMacAddress).SDNRemoteArpMacAddress"
		result, _, err := key.GetStringValue("SDNRemoteArpMacAddress")
		if err != nil {
			return err
		}

		// Set the reg key if not already set or has incorrect value
		if result != SDNRemoteArpMacAddress {

			//was "Set-ItemProperty -Path HKLM:\\SYSTEM\\CurrentControlSet\\Services\\hns\\State -Name SDNRemoteArpMacAddress -Value \"12-34-56-78-9a-bc\""

			if err := key.SetStringValue("SDNRemoteArpMacAddress", SDNRemoteArpMacAddress); err != nil {
				log.Printf("Failed to set SDNRemoteArpMacAddress due to error %s", err.Error())
				return err
			}
			log.Printf("[Azure CNS] SDNRemoteArpMacAddress regKey set successfully. Restarting hns service.")

			//	was "Restart-Service -Name hns"
			if err := registerClient.restartService("hns"); err != nil {
				log.Printf("Failed to Restart HNS Service due to error %s", err.Error())
				return err
			}
		}

		sdnRemoteArpMacAddressSet = true
	}
	return nil
}
