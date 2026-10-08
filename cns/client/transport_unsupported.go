//go:build !linux

package client

import (
	"errors"
	"net/http"
)

var errUnsupportedUnixSocket = errors.New("CNS Unix sockets are not supported on this platform")

func newUnixIPAMTransport(string, *http.Transport) (http.RoundTripper, error) {
	return nil, errUnsupportedUnixSocket
}
