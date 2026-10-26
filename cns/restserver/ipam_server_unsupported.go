//go:build !linux

package restserver

import (
	"errors"
	"net"
)

var errUnsupportedIPAMSocket = errors.New("cns ipam unix socket is not supported on this platform")

func listenIPAMSocket(string) (net.Listener, error) {
	return nil, errUnsupportedIPAMSocket
}
