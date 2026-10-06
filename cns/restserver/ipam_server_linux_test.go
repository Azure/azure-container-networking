//go:build linux

package restserver

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestValidateIPAMServerConfig(t *testing.T) {
	require.Error(t, validateIPAMServerConfig(filepath.Join(t.TempDir(), "cns.sock")))

	err := validateIPAMServerConfig(defaultIPAMUnixSocketPath)
	if os.Geteuid() == 0 {
		require.NoError(t, err)
	} else {
		require.Error(t, err)
	}
}

func TestListenIPAMSocketLifecycle(t *testing.T) {
	t.Run("creates secure directory and socket", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "missing")
		socketPath := filepath.Join(parent, "cns.sock")

		listener, err := listenIPAMSocket(socketPath)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = listener.Close()
		})

		socketInfo, err := os.Lstat(socketPath)
		require.NoError(t, err)
		require.NotZero(t, socketInfo.Mode()&os.ModeSocket)
		require.Equal(t, os.FileMode(ipamSocketMode), socketInfo.Mode().Perm())

		parentInfo, err := os.Stat(parent)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(ipamSocketDirectoryMode), parentInfo.Mode().Perm())
	})

	t.Run("rejects directory writable by group or other users", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "shared")
		require.NoError(t, os.Mkdir(parent, 0o700))
		require.NoError(t, os.Chmod(parent, 0o775))

		listener, err := listenIPAMSocket(filepath.Join(parent, "cns.sock"))
		require.Nil(t, listener)
		require.Error(t, err)
	})

	t.Run("does not change existing directory permissions", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "existing")
		require.NoError(t, os.Mkdir(parent, 0o700))
		require.NoError(t, os.Chmod(parent, 0o755))

		listener, err := listenIPAMSocket(filepath.Join(parent, "cns.sock"))
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = listener.Close()
		})

		info, err := os.Stat(parent)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	})

	t.Run("removes stale socket", func(t *testing.T) {
		socketPath := filepath.Join(t.TempDir(), "cns.sock")
		stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
		require.NoError(t, err)
		stale.SetUnlinkOnClose(false)
		require.NoError(t, stale.Close())

		listener, err := listenIPAMSocket(socketPath)
		require.NoError(t, err)
		require.NoError(t, listener.Close())
	})

	t.Run("rejects active socket", func(t *testing.T) {
		socketPath := filepath.Join(t.TempDir(), "cns.sock")
		var listenConfig net.ListenConfig
		active, err := listenConfig.Listen(t.Context(), "unix", socketPath)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = active.Close()
		})

		listener, err := listenIPAMSocket(socketPath)
		require.Nil(t, listener)
		require.Error(t, err)
	})

	t.Run("rejects non-socket path", func(t *testing.T) {
		socketPath := filepath.Join(t.TempDir(), "cns.sock")
		require.NoError(t, os.WriteFile(socketPath, nil, 0o600))

		listener, err := listenIPAMSocket(socketPath)
		require.Nil(t, listener)
		require.Error(t, err)
	})
}

func TestIPAMServerExposesOnlyIPAMRoutes(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "cns.sock")
	server, err := startIPAMServer(socketPath, &HTTPRestService{}, make(chan error, 1))
	require.NoError(t, err)

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, "unix", socketPath)
			},
		},
		Timeout: time.Second,
	}

	for _, route := range []string{cns.RequestIPConfig, cns.RequestIPConfigs, cns.ReleaseIPConfig, cns.ReleaseIPConfigs} {
		counter := ipamUnixRequests.WithLabelValues(route)
		before := testutil.ToFloat64(counter)
		req, requestErr := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost"+route, bytes.NewBufferString("{"))
		require.NoError(t, requestErr)
		response, responseErr := client.Do(req)
		require.NoError(t, responseErr)
		_, _ = io.Copy(io.Discard, response.Body)
		require.NoError(t, response.Body.Close())
		require.NotEqual(t, http.StatusNotFound, response.StatusCode)
		require.InDelta(t, before+1, testutil.ToFloat64(counter), 0)
	}

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://localhost/debug/pprof/", http.NoBody)
	require.NoError(t, err)
	response, err := client.Do(request)
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, response.Body)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusNotFound, response.StatusCode)

	server.stop()
	_, err = os.Lstat(socketPath)
	require.ErrorIs(t, err, os.ErrNotExist)

	restarted, err := startIPAMServer(socketPath, &HTTPRestService{}, make(chan error, 1))
	require.NoError(t, err)
	restarted.stop()
	_, err = os.Lstat(socketPath)
	require.ErrorIs(t, err, os.ErrNotExist)
}
