package transport

import "errors"

var (
	// ErrNoPeer is returned when no peer was found in the gRPC context.
	ErrNoPeer = errors.New("no peer found in gRPC context")
	// ErrTLSCerts is returned for any TLS certificate related issue
	ErrTLSCerts = errors.New("tls certificate error")
)
