//go:build !linux

package restserver

import (
	"errors"
	"net"
)

var errUnsupportedIPAMSocket = errors.New("Unix sockets are not supported by CNS IPAM on this platform")

func validateIPAMServerConfig(socketPath string) error {
	if socketPath == "" {
		return nil
	}
	return errUnsupportedIPAMSocket
}

func listenIPAMSocket(string) (net.Listener, error) {
	return nil, errUnsupportedIPAMSocket
}
