//go:build windows
// +build windows

// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName        = "azure-cns"
	serviceDisplayName = "Azure Container Networking Service"
	serviceDescription = "Provides container networking services for Azure"
)

// windowsService implements the svc.Handler interface for Windows service control
type windowsService struct {
	runService func()
}

// Execute is called by the Windows service manager and implements the service control loop
func (ws *windowsService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	changes <- svc.Status{State: svc.StartPending}

	// Start the service in a goroutine
	go ws.runService()

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	// Service control loop
loop:
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				// Cancel the root context to signal shutdown
				if rootCtx != nil {
					// Send shutdown signal through the error channel
					select {
					case rootErrCh <- fmt.Errorf("service stop requested"):
					default:
					}
				}
				break loop
			default:
				// Log unexpected control request
			}
		}
	}

	return
}

// runAsService runs the application as a Windows service
func runAsService() error {
	elog, err := eventlog.Open(serviceName)
	if err != nil {
		return fmt.Errorf("failed to open event log: %w", err)
	}
	defer elog.Close()

	elog.Info(1, fmt.Sprintf("Starting %s service", serviceName))

	ws := &windowsService{
		runService: func() {
			// The main service logic will run in the existing main() function
			// after runAsService() returns
		},
	}

	err = svc.Run(serviceName, ws)
	if err != nil {
		elog.Error(1, fmt.Sprintf("Service failed: %v", err))
		return fmt.Errorf("failed to run service: %w", err)
	}

	elog.Info(1, fmt.Sprintf("%s service stopped", serviceName))
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
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", serviceName)
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
		// Remove the service if we can't set up event log
		s.Delete()
		return fmt.Errorf("failed to setup event log: %w", err)
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
	defer m.Disconnect()

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
				return fmt.Errorf("timeout waiting for service to stop")
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
		return "", err
	}
	return filepath.Abs(exepath)
}

// isWindowsService checks if the application is running as a Windows service
func isWindowsService() (bool, error) {
	return svc.IsWindowsService()
}
