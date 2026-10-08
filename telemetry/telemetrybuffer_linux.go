// Copyright 2018 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
)

const (
	fdTemplate                  = "/var/run/%s.sock"
	TelemetryServiceProcessName = "azure-vnet-telemetry"
	CniInstallDir               = "/opt/cni/bin"
	metadataFile                = "/tmp/azuremetadata.json"
)

// Dial - try to connect to/create a socket with 'name'
func (tb *TelemetryBuffer) Dial(name string) (err error) {
	conn, err := net.Dial("unix", fdPath(name)) //nolint:noctx // legacy Dial API has no context parameter
	if err == nil {
		tb.client = conn
	}

	return err
}

// Listen - try to create and listen on socket with 'name'
func (tb *TelemetryBuffer) Listen(name string) (err error) {
	conn, err := net.Listen("unix", fdPath(name)) //nolint:noctx // legacy Listen API has no context parameter
	if err == nil {
		tb.listener = conn
	}

	return err
}

// cleanup - manually remove socket
func (tb *TelemetryBuffer) Cleanup(name string) error {
	if err := os.Remove(fdPath(name)); err != nil {
		return fmt.Errorf("removing telemetry socket: %w", err)
	}
	return nil
}

func fdPath(name string) string {
	if filepath.IsAbs(name) {
		return name
	}
	return fmt.Sprintf(fdTemplate, name)
}

func SockExists() bool {
	if _, err := os.Stat(fmt.Sprintf(fdTemplate, FdName)); !os.IsNotExist(err) {
		return true
	}

	return false
}
