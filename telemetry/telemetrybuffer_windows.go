// Copyright 2018 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"fmt"
	"os"

	"github.com/Microsoft/go-winio"
)

const (
	fdTemplate                  = "\\\\.\\pipe\\%s"
	TelemetryServiceProcessName = "azure-vnet-telemetry.exe"
	CniInstallDir               = "c:\\k\\azurecni\\bin"
	metadataFile                = "azuremetadata.json"
)

// Dial - try to connect to a named pipe with 'name'
func (tb *TelemetryBuffer) Dial(name string) (err error) {
	conn, err := winio.DialPipe(fmt.Sprintf(fdTemplate, name), nil)
	if err == nil {
		tb.client = conn
	}

	return err
}

// Listen - try to create and listen on named pipe with 'name'
func (tb *TelemetryBuffer) Listen(name string) (err error) {
	listener, err := winio.ListenPipe(fmt.Sprintf(fdTemplate, name), nil)
	if err == nil {
		tb.listener = listener
	}

	return err
}

// Cleanup - cleanup named pipe
func (tb *TelemetryBuffer) Cleanup(name string) error {
	return nil
}

// Check if telemetry unix domain socket exists
func SockExists() bool {
	if _, err := os.Stat(fmt.Sprintf(fdTemplate, FdName)); !os.IsNotExist(err) {
		return true
	}

	return false
}
