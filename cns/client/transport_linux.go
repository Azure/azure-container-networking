//go:build linux

package client

import (
	"context"
	stderrors "errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"syscall"
)

var errRelativeUnixSocket = stderrors.New("cns Unix socket path must be absolute")

func newUnixIPAMTransport(socketPath string, tcpTransport *http.Transport) (http.RoundTripper, error) {
	if !filepath.IsAbs(socketPath) {
		return nil, fmt.Errorf("%w: %q", errRelativeUnixSocket, socketPath)
	}
	ipamTransport := tcpTransport.Clone()
	unixDialer := &net.Dialer{}
	tcpDialContext := tcpTransport.DialContext
	if tcpDialContext == nil {
		tcpDialContext = (&net.Dialer{}).DialContext
	}
	var unixConnected atomic.Bool
	ipamTransport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := unixDialer.DialContext(ctx, "unix", socketPath)
		if err == nil {
			unixConnected.Store(true)
			return conn, nil
		}
		// After a Unix connection succeeds, a later request can follow one with an unknown result, so do not switch to TCP.
		if unixConnected.Load() || !isSafeUnixFallbackError(err) {
			return nil, fmt.Errorf("dialing CNS Unix socket %q: %w", socketPath, err)
		}
		return tcpDialContext(ctx, network, address)
	}
	return ipamTransport, nil
}

// isSafeUnixFallbackError reports whether the Unix dial failed before CNS could receive the request.
func isSafeUnixFallbackError(err error) bool {
	return stderrors.Is(err, syscall.ENOENT) || stderrors.Is(err, syscall.ECONNREFUSED)
}
