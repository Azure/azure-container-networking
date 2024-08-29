package platform

import (
	"context"
	"fmt"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform/windows/adapter"
	"github.com/Azure/azure-container-networking/platform/windows/adapter/mellanox"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type RegistryClient interface {
	OpenKey(k registry.Key, path string, access uint32) (RegistryKey, error)
	restartService(serviceName string) error
}

type RegistryKey interface {
	GetStringValue(name string) (val string, valtype uint32, err error)
	SetStringValue(name, value string) error
	Close() error
}

type registryClient struct {
	Timeout time.Duration
	logger  *zap.Logger
}

type registryKey struct {
	key registry.Key
}

func NewRegistryClient(logger *zap.Logger) RegistryClient {
	return &registryClient{
		Timeout: defaultExecTimeout * time.Second,
		logger:  logger,
	}
}

func (r *registryClient) OpenKey(k registry.Key, path string, access uint32) (RegistryKey, error) {
	key, err := registry.OpenKey(k, path, access)
	if err != nil {
		return nil, err
	}
	return &registryKey{key}, nil
}

func (k *registryKey) GetStringValue(name string) (string, uint32, error) {
	return k.key.GetStringValue(name)
}

func (k *registryKey) SetStringValue(name string, value string) error {
	return k.key.SetStringValue(name, value)
}

func (k *registryKey) Close() error {
	return k.key.Close()
}

// straight out of chat gpt
func (r *registryClient) restartService(serviceName string) error {
	// Connect to the service manager
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("could not connect to service manager: %v", err)
	}
	defer m.Disconnect()

	// Open the service by name
	service, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer service.Close()

	// Stop the service
	_, err = service.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("could not stop service: %v", err)
	}

	// Wait for the service to stop
	status, err := service.Query()
	if err != nil {
		return fmt.Errorf("could not query service status: %v", err)
	}
	for status.State != svc.Stopped {
		time.Sleep(500 * time.Millisecond)
		status, err = service.Query()
		if err != nil {
			return fmt.Errorf("could not query service status: %v", err)
		}
	}

	// Start the service again
	err = service.Start()
	if err != nil {
		return fmt.Errorf("could not start service: %v", err)
	}

	return nil
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
	//TODO not riemovign this because it seems to only be called in test?
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

/*
Output:
6C-A1-00-50-E4-2D PCI\VEN_8086&DEV_2723&SUBSYS_00808086&REV_1A\4&328243d9&0&00E0
80-6D-97-1E-CF-4E USB\VID_17EF&PID_A359\3010019E3
*/
func FetchMacAddressPnpIDMapping(ctx context.Context, execClient ExecClient) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, ExecTimeout)
	defer cancel() // The cancel should be deferred so resources are cleaned up
	output, err := execClient.ExecutePowershellCommandWithContext(ctx, GetMacAddressVFPPnpIDMapping)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch VF mapping")
	}
	result := make(map[string]string)
	if output != "" {
		// Split the output based on new line characters
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			// Split based on " " to fetch the macaddress and pci id
			parts := strings.Split(line, " ")
			// Changing the format of macaddress from xx-xx-xx-xx to xx:xx:xx:xx
			formattedMacaddress, err := net.ParseMAC(parts[0])
			if err != nil {
				return nil, errors.Wrap(err, "failed to fetch MACAddressPnpIDMapping")
			}
			key := formattedMacaddress.String()
			value := parts[1]
			result[key] = value
		}
	}
	return result, nil
}
