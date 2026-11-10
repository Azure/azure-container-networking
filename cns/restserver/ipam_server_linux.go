//go:build linux

package restserver

import (
	stderrors "errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const ipamSocketMode = 0o600

var (
	errRelativeIPAMSocket  = stderrors.New("cns Unix socket path must be absolute")
	errIPAMSocketDirectory = stderrors.New("unix socket directory must be root-owned with mode 0700")
	errIPAMSocketType      = stderrors.New("unix socket path is not a socket")
	errIPAMSocketActive    = stderrors.New("unix socket already has a listener")
)

// listenIPAMSocket replaces only stale sockets in a root-owned directory with mode 0700.
func listenIPAMSocket(socketPath string) (net.Listener, error) {
	if !filepath.IsAbs(socketPath) {
		return nil, fmt.Errorf("%w: %q", errRelativeIPAMSocket, socketPath)
	}
	directory := filepath.Dir(socketPath)
	directoryInfo, err := os.Lstat(directory)
	if err != nil {
		return nil, fmt.Errorf("checking Unix socket directory %q: %w", directory, err)
	}
	owner, ok := directoryInfo.Sys().(*syscall.Stat_t)
	if !directoryInfo.IsDir() || directoryInfo.Mode().Perm() != 0o700 || !ok || owner.Uid != 0 {
		return nil, fmt.Errorf("%w: %q", errIPAMSocketDirectory, directory)
	}

	socketInfo, err := os.Lstat(socketPath)
	if err != nil && !stderrors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("checking Unix socket %q: %w", socketPath, err)
	}
	if err == nil {
		if socketInfo.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("%w: %q", errIPAMSocketType, socketPath)
		}
		conn, dialErr := net.DialTimeout("unix", socketPath, time.Second) //nolint:noctx // startup has no context. The probe times out after one second.
		if dialErr == nil {
			_ = conn.Close()
			return nil, fmt.Errorf("%w: %q", errIPAMSocketActive, socketPath)
		}
		if !stderrors.Is(dialErr, syscall.ECONNREFUSED) {
			return nil, fmt.Errorf("checking old Unix socket %q: %w", socketPath, dialErr)
		}
		if err = os.Remove(socketPath); err != nil {
			return nil, fmt.Errorf("removing old Unix socket %q: %w", socketPath, err)
		}
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
