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

const (
	defaultIPAMUnixSocketPath = "/var/run/azure-cns/cns.sock"
	ipamSocketDirectoryMode   = 0o700
	ipamSocketMode            = 0o600
	socketProbeTimeout        = 100 * time.Millisecond
)

var errInvalidIPAMSocket = stderrors.New("invalid CNS IPAM Unix socket")

func validateIPAMServerConfig(socketPath string) error {
	if socketPath == "" {
		return nil
	}
	if socketPath != defaultIPAMUnixSocketPath {
		return fmt.Errorf("%w: path must be %q, got %q", errInvalidIPAMSocket, defaultIPAMUnixSocketPath, socketPath)
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("%w: cns must run as UID 0 to create the socket", errInvalidIPAMSocket)
	}
	return nil
}

func listenIPAMSocket(socketPath string) (net.Listener, error) {
	if err := prepareIPAMSocketDirectory(filepath.Dir(socketPath)); err != nil {
		return nil, err
	}
	if err := removeStaleIPAMSocket(socketPath); err != nil {
		return nil, err
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

func prepareIPAMSocketDirectory(directory string) error {
	if err := os.MkdirAll(directory, ipamSocketDirectoryMode); err != nil {
		return fmt.Errorf("creating Unix socket directory: %w", err)
	}

	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("checking Unix socket directory %q: %w", directory, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: parent path %q is not a directory", errInvalidIPAMSocket, directory)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: cannot determine owner of directory %q", errInvalidIPAMSocket, directory)
	}
	if int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("%w: directory %q is owned by UID %d, want UID %d", errInvalidIPAMSocket, directory, stat.Uid, os.Geteuid())
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%w: directory %q must not be writable by group or other users", errInvalidIPAMSocket, directory)
	}
	return nil
}

func removeStaleIPAMSocket(socketPath string) error {
	info, err := os.Lstat(socketPath)
	if stderrors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking Unix socket path %q: %w", socketPath, err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%w: path %q exists and is not a socket", errInvalidIPAMSocket, socketPath)
	}

	conn, dialErr := net.DialTimeout("unix", socketPath, socketProbeTimeout) //nolint:noctx // startup probe has a fixed timeout
	if dialErr == nil {
		_ = conn.Close()
		return fmt.Errorf("%w: path %q already has an active listener", errInvalidIPAMSocket, socketPath)
	}
	staleSocket := stderrors.Is(dialErr, syscall.ECONNREFUSED) || stderrors.Is(dialErr, os.ErrNotExist)
	if !staleSocket {
		return fmt.Errorf("probing Unix socket path %q: %w", socketPath, dialErr)
	}
	if err := os.Remove(socketPath); err != nil && !stderrors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing stale Unix socket %q: %w", socketPath, err)
	}
	return nil
}
