//go:build linux

package restserver

import (
	stderrors "errors"
	"fmt"
	"net"
	"os"
	"syscall"
)

const ipamSocketMode = 0o600

// listenIPAMSocket removes a socket that a previous CNS process left, and listens on a new socket with mode 0600.
// CNS starts its TCP listener first, so a second CNS process fails before it can remove an active socket.
func listenIPAMSocket(socketPath string) (net.Listener, error) {
	if err := os.Remove(socketPath); err != nil && !stderrors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("removing old Unix socket %q: %w", socketPath, err)
	}

	// Set the socket mode before listen so that no client can connect while the socket has the umask mode.
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("creating Unix socket: %w", err)
	}
	socketFile := os.NewFile(uintptr(fd), socketPath)
	defer socketFile.Close()
	if err = syscall.Bind(fd, &syscall.SockaddrUnix{Name: socketPath}); err != nil {
		return nil, fmt.Errorf("binding Unix socket %q: %w", socketPath, err)
	}
	if err = os.Chmod(socketPath, ipamSocketMode); err != nil {
		_ = os.Remove(socketPath)
		return nil, fmt.Errorf("setting Unix socket permissions: %w", err)
	}
	if err = syscall.Listen(fd, syscall.SOMAXCONN); err != nil {
		_ = os.Remove(socketPath)
		return nil, fmt.Errorf("listening on Unix socket %q: %w", socketPath, err)
	}
	listener, err := net.FileListener(socketFile)
	if err != nil {
		_ = os.Remove(socketPath)
		return nil, fmt.Errorf("creating Unix socket listener: %w", err)
	}
	listener.(*net.UnixListener).SetUnlinkOnClose(true)
	return listener, nil
}
