// Copyright 2018 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"fmt"
	"net"
	"os"
)

const (
	fdTemplate = "/tmp/%s.sock"
)

// Dial - try to connect to/create a socket with 'name'
func (tb *TelemetryBuffer) Dial(name string) (err error) {
	conn, err := net.Dial("unix", fmt.Sprintf(fdTemplate, name))
	if err == nil {
		tb.client = conn
	}

	return err
}

// Listen - try to create and listen on socket with 'name'
func (tb *TelemetryBuffer) Listen(name string) (err error) {
	conn, err := net.Listen("unix", fmt.Sprintf(fdTemplate, name))
	if err == nil {
		tb.listener = conn
	}

	return err
}

// cleanup - manually remove socket
func (tb *TelemetryBuffer) cleanup(name string) error {
	return os.Remove(fmt.Sprintf(fdTemplate, name))
}
