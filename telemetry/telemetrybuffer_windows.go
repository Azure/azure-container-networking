// Copyright 2018 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"fmt"

	"github.com/Microsoft/go-winio"
)

const (
	fdTemplate = "\\\\.\\pipe\\%s"
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

// cleanup - stub
func (tb *TelemetryBuffer) cleanup(name string) error {
	return nil
}
