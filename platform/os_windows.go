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
	"github.com/Azure/azure-container-networking/platform/windows/adapter"
	"github.com/Azure/azure-container-networking/platform/windows/adapter/mellanox"
	"github.com/avast/retry-go/v4"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
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

	// Interval between successive checks for mellanox adapter's PriorityVLANTag value
	defaultMellanoxMonitorInterval = 30 * time.Second

	// Value for reg key: PriorityVLANTag for adapter
	// reg key value for PriorityVLANTag = 3  --> Packet priority and VLAN enabled
	// for more details goto https://learn.microsoft.com/en-us/windows-hardware/drivers/network/standardized-inf-keywords-for-ndis-qos
	desiredVLANTagForMellanox = 3
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

func (p *execClient) ExecuteCommand(command string) (string, error) {
	if p.logger != nil {
		p.logger.Info("[Azure-Utils]", zap.String("ExecuteCommand", command))
	} else {
		log.Printf("[Azure-Utils] ExecuteCommand: %q", command)
	}

	var stderr, stdout bytes.Buffer

	cmd := exec.Command("cmd", "/c", command)
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
	_, err := p.ExecuteCommand(cmd)
	return err // nolint
}

// ExecutePowershellCommand executes powershell command
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

// SetSdnRemoteArpMacAddress sets the regkey for SDNRemoteArpMacAddress needed for multitenancy if hns is enabled
func SetSdnRemoteArpMacAddress(ctx context.Context) error {
	if err := setSDNRemoteARPRegKey(); err != nil {
		return err
	}
	log.Printf("SDNRemoteArpMacAddress regKey set successfully")
	if err := restartHNS(ctx); err != nil {
		return err
	}
	log.Printf("HNS service restarted successfully")
	return nil
}

func setSDNRemoteARPRegKey() error {
	log.Printf("Setting SDNRemoteArpMacAddress regKey")
	// open the registry key
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\hns\State`, registry.READ|registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return errors.Wrap(err, "could not open registry key")
	}
	defer k.Close()
	// check the key value
	if v, _, _ := k.GetStringValue("SDNRemoteArpMacAddress"); v == SDNRemoteArpMacAddress {
		log.Printf("SDNRemoteArpMacAddress regKey already set")
		return nil // already set
	}
	if err = k.SetStringValue("SDNRemoteArpMacAddress", SDNRemoteArpMacAddress); err != nil {
		return errors.Wrap(err, "could not set registry key")
	}
	return nil
}

func restartHNS(ctx context.Context) error {
	log.Printf("Restarting HNS service")
	// connect to the service manager
	m, err := mgr.Connect()
	if err != nil {
		return errors.Wrap(err, "could not connect to service manager")
	}
	defer m.Disconnect() //nolint:errcheck // ignore error
	// open the HNS service
	service, err := m.OpenService("hns")
	if err != nil {
		return errors.Wrap(err, "could not access service")
	}
	defer service.Close()
	// Stop the service
	log.Printf("Stopping HNS service")
	_ = retry.Do(
		tryStopServiceFn(ctx, service),
		retry.UntilSucceeded(),
		retry.Context(ctx),
		retry.DelayType(retry.BackOffDelay),
	)
	// Start the service again
	log.Printf("Starting HNS service")
	_ = retry.Do(
		tryStartServiceFn(ctx, service),
		retry.UntilSucceeded(),
		retry.Context(ctx),
		retry.DelayType(retry.BackOffDelay),
	)
	log.Printf("HNS service started")
	return nil
}

type managedService interface {
	Control(control svc.Cmd) (svc.Status, error)
	Query() (svc.Status, error)
	Start(args ...string) error
}

func tryStartServiceFn(ctx context.Context, service managedService) func() error {
	shouldStart := func(state svc.State) bool {
		return !(state == svc.Running || state == svc.StartPending)
	}
	return func() error {
		status, err := service.Query()
		if err != nil {
			return errors.Wrap(err, "could not query service status")
		}
		if shouldStart(status.State) {
			err = service.Start()
			if err != nil {
				return errors.Wrap(err, "could not start service")
			}
		}
		// Wait for the service to start
		deadline, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		ticker := time.NewTicker(500 * time.Millisecond) //nolint:gomnd // 500ms
		defer ticker.Stop()
		for {
			status, err := service.Query()
			if err != nil {
				return errors.Wrap(err, "could not query service status")
			}
			if status.State == svc.Running {
				log.Printf("service started")
				break
			}
			select {
			case <-deadline.Done():
				return deadline.Err() //nolint:wrapcheck // error has sufficient context
			case <-ticker.C:
			}
		}
		return nil
	}
}

func tryStopServiceFn(ctx context.Context, service managedService) func() error {
	shouldStop := func(state svc.State) bool {
		return !(state == svc.Stopped || state == svc.StopPending)
	}
	return func() error {
		status, err := service.Query()
		if err != nil {
			return errors.Wrap(err, "could not query service status")
		}
		if shouldStop(status.State) {
			_, err = service.Control(svc.Stop)
			if err != nil {
				return errors.Wrap(err, "could not stop service")
			}
		}
		// Wait for the service to stop
		deadline, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		ticker := time.NewTicker(500 * time.Millisecond) //nolint:gomnd // 500ms
		defer ticker.Stop()
		for {
			status, err := service.Query()
			if err != nil {
				return errors.Wrap(err, "could not query service status")
			}
			if status.State == svc.Stopped {
				log.Printf("service stopped")
				break
			}
			select {
			case <-deadline.Done():
				return deadline.Err() //nolint:wrapcheck // error has sufficient context
			case <-ticker.C:
			}
		}
		return nil
	}
}

func HasMellanoxAdapter() bool {
	m := &mellanox.Mellanox{}
	return hasNetworkAdapter(m)
}

func hasNetworkAdapter(na adapter.NetworkAdapter) bool {
	adapterName, err := na.GetAdapterName()
	if err != nil {
		log.Errorf("Error while getting network adapter name: %v", err)
		return false
	}
	log.Printf("Name of the network adapter : %v", adapterName)
	return true
}

// Regularly monitors the Mellanox PriorityVLANGTag registry value and sets it to desired value if needed
func MonitorAndSetMellanoxRegKeyPriorityVLANTag(ctx context.Context, intervalSecs int) {
	m := &mellanox.Mellanox{}
	interval := defaultMellanoxMonitorInterval
	if intervalSecs > 0 {
		interval = time.Duration(intervalSecs) * time.Second
	}
	err := updatePriorityVLANTagIfRequired(m, desiredVLANTagForMellanox)
	if err != nil {
		log.Errorf("Error while monitoring mellanox, continuing: %v", err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Printf("context cancelled, stopping Mellanox Monitoring: %v", ctx.Err())
			return
		case <-ticker.C:
			err := updatePriorityVLANTagIfRequired(m, desiredVLANTagForMellanox)
			if err != nil {
				log.Errorf("Error while monitoring mellanox, continuing: %v", err)
			}
		}
	}
}

// Updates the priority VLAN Tag of mellanox adapter if not already set to the desired value
func updatePriorityVLANTagIfRequired(na adapter.NetworkAdapter, desiredValue int) error {
	currentVal, err := na.GetPriorityVLANTag()
	if err != nil {
		return fmt.Errorf("error while getting Priority VLAN Tag value: %w", err)
	}

	if currentVal == desiredValue {
		log.Printf("Adapter's PriorityVLANTag is already set to %v, skipping reset", desiredValue)
		return nil
	}

	err = na.SetPriorityVLANTag(desiredValue)
	if err != nil {
		return fmt.Errorf("error while setting Priority VLAN Tag value: %w", err)
	}

	return nil
}

func GetOSDetails() (map[string]string, error) {
	return nil, nil
}

func GetProcessNameByID(pidstr string) (string, error) {
	pidstr = strings.Trim(pidstr, "\r\n")
	cmd := fmt.Sprintf("Get-Process -Id %s|Format-List", pidstr)
	p := NewExecClient(nil)
	out, err := p.ExecutePowershellCommand(cmd)
	if err != nil {
		log.Printf("Process is not running. Output:%v, Error %v", out, err)
		return "", err
	}

	if len(out) <= 0 {
		log.Printf("Output length is 0")
		return "", fmt.Errorf("get-process output length is 0")
	}

	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Name") {
			pName := strings.Split(line, ":")
			if len(pName) > 1 {
				return strings.TrimSpace(pName[1]), nil
			}
		}
	}

	return "", fmt.Errorf("Process not found")
}

func PrintDependencyPackageDetails() {
}

// https://docs.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexw
func ReplaceFile(source, destination string) error {
	src, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return err
	}

	dest, err := syscall.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}

	return windows.MoveFileEx(src, dest, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
