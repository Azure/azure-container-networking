package transport

import "errors"

// ErrNoPeer is returned when no peer was found in the gRPC context.
var ErrNoPeer = errors.New("no peer found in gRPC context")
