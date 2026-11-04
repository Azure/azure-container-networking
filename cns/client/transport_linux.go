//go:build linux

package client

import (
	"context"
	stderrors "errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
)

var errRelativeUnixSocket = stderrors.New("cns Unix socket path must be absolute")

func newUnixIPAMTransport(socketPath string, tcpTransport *http.Transport) (http.RoundTripper, error) {
	if !filepath.IsAbs(socketPath) {
		return nil, fmt.Errorf("%w: %q", errRelativeUnixSocket, socketPath)
	}
	ipamTransport := tcpTransport.Clone()
	var unixDialer net.Dialer
	ipamTransport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return unixDialer.DialContext(ctx, "unix", socketPath)
	}
	return ipamTransport, nil
}
