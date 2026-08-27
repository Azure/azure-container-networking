//go:build windows
// +build windows

// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	acn "github.com/Azure/azure-container-networking/common"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName        = "azure-cns"
	serviceDisplayName = "Azure Container Networking Service"
	serviceDescription = "Provides container networking services for Azure"
)

var (
	errServiceAlreadyExists = errors.New("service already exists")
	errServiceStopRequested = errors.New("service stop requested")
	errServiceStopTimeout   = errors.New("timeout waiting for service to stop")
)

// windowsService implements the svc.Handler interface for Windows service control
type windowsService struct {
	runService func()
}

// Execute is called by the Windows service manager and implements the service control loop
func (ws *windowsService) Execute(
	_ []string,
	r <-chan svc.ChangeRequest,
	changes chan<- svc.Status,
) (serviceSpecificExitCode bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	changes <- svc.Status{State: svc.StartPending}

	// Start the service in a goroutine
	go ws.runService()

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			select {
			case rootErrCh <- errServiceStopRequested:
			default:
			}
			return false, 0
		case svc.Pause,
			svc.Continue,
			svc.ParamChange,
			svc.NetBindAdd,
			svc.NetBindRemove,
			svc.NetBindEnable,
			svc.NetBindDisable,
			svc.DeviceEvent,
			svc.HardwareProfileChange,
			svc.PowerEvent,
			svc.SessionChange,
			svc.PreShutdown:
			// These controls are not accepted by this service.
		default:
			// Ignore commands added by newer Windows versions.
		}
	}

	return false, 0
}

func handleServiceAction(action string) (bool, error) {
	switch action {
	case acn.OptServiceInstall:
		return true, installService()
	case acn.OptServiceUninstall:
		return true, uninstallService()
	case acn.OptServiceRun:
		return false, runAsService()
	case "":
		isService, err := isWindowsService()
		if err != nil {
			return false, fmt.Errorf("detecting service mode: %w", err)
		}
		if isService {
			return false, runAsService()
		}
		return false, nil
	default:
		return false, nil
	}
}

func runAsService() error {
	elog, err := eventlog.Open(serviceName)
	if err != nil {
		return fmt.Errorf("failed to open event log: %w", err)
	}
	defer elog.Close()

	_ = elog.Info(1, "Starting "+serviceName+" service") //nolint:errcheck // Event log writes are best-effort.

	ws := &windowsService{
		runService: func() {
			// The main service logic will run in the existing main() function
			// after runAsService() returns
		},
	}

	err = svc.Run(serviceName, ws)
	if err != nil {
		_ = elog.Error(1, fmt.Sprintf("Service failed: %v", err)) //nolint:errcheck // Preserve the service run error.
		return fmt.Errorf("failed to run service: %w", err)
	}

	_ = elog.Info(1, serviceName+" service stopped") //nolint:errcheck // Event log writes are best-effort.
	return nil
}

// installService installs the CNS as a Windows service
func installService() error {
	exepath, err := getExecutablePath()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer func() {
		_ = m.Disconnect() //nolint:errcheck // There is no recovery action for disconnect failure.
	}()

	s, err := m.OpenService(serviceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("%w: %s", errServiceAlreadyExists, serviceName)
	}

	s, err = m.CreateService(serviceName, exepath, mgr.Config{
		DisplayName:      serviceDisplayName,
		Description:      serviceDescription,
		StartType:        mgr.StartAutomatic,
		ServiceStartName: "LocalSystem",
	})
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	defer s.Close()

	// Set recovery options to restart the service on failure
	err = s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
	}, 86400) // Reset failure count after 24 hours
	if err != nil {
		// This is not a fatal error, just log it
		fmt.Printf("Warning: failed to set recovery actions: %v\n", err)
	}

	// Set up event log
	err = eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info)
	if err != nil {
		deleteErr := s.Delete()
		return errors.Join(fmt.Errorf("failed to set up event log: %w", err), deleteErr)
	}

	fmt.Printf("Service %s installed successfully.\n", serviceName)
	fmt.Printf("Run 'net start %s' to start the service.\n", serviceName)
	return nil
}

// uninstallService removes the CNS Windows service
func uninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer func() {
		_ = m.Disconnect() //nolint:errcheck // There is no recovery action for disconnect failure.
	}()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %s is not installed: %w", serviceName, err)
	}
	defer s.Close()

	// Try to stop the service if it's running
	status, err := s.Query()
	if err != nil {
		return fmt.Errorf("failed to query service status: %w", err)
	}

	if status.State != svc.Stopped {
		status, err = s.Control(svc.Stop)
		if err != nil {
			return fmt.Errorf("failed to stop service: %w", err)
		}

		// Wait for the service to stop
		timeout := time.Now().Add(10 * time.Second)
		for status.State != svc.Stopped {
			if time.Now().After(timeout) {
				return errServiceStopTimeout
			}
			time.Sleep(300 * time.Millisecond)
			status, err = s.Query()
			if err != nil {
				return fmt.Errorf("failed to query service status: %w", err)
			}
		}
	}

	err = s.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	// Remove event log
	err = eventlog.Remove(serviceName)
	if err != nil {
		// This is not fatal, just log it
		fmt.Printf("Warning: failed to remove event log: %v\n", err)
	}

	fmt.Printf("Service %s uninstalled successfully.\n", serviceName)
	return nil
}

// getExecutablePath returns the full path to the current executable
func getExecutablePath() (string, error) {
	exepath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("getting executable path: %w", err)
	}
	exepath, err = filepath.Abs(exepath)
	if err != nil {
		return "", fmt.Errorf("getting absolute executable path: %w", err)
	}
	return exepath, nil
}

// isWindowsService checks if the application is running as a Windows service
func isWindowsService() (bool, error) {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false, fmt.Errorf("checking Windows service status: %w", err)
	}
	return isService, nil
}
