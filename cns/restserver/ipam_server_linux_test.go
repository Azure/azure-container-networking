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
	"github.com/stretchr/testify/require"
)

func TestListenIPAMSocket(t *testing.T) {
	t.Run("creates socket with mode 0600", func(t *testing.T) {
		socketPath := filepath.Join(t.TempDir(), "cns.sock")

		listener, err := listenIPAMSocket(socketPath)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = listener.Close()
		})

		socketInfo, err := os.Lstat(socketPath)
		require.NoError(t, err)
		require.NotZero(t, socketInfo.Mode()&os.ModeSocket)
		require.Equal(t, os.FileMode(ipamSocketMode), socketInfo.Mode().Perm())
	})

	t.Run("replaces old socket", func(t *testing.T) {
		socketPath := filepath.Join(t.TempDir(), "cns.sock")
		old, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
		require.NoError(t, err)
		old.SetUnlinkOnClose(false)
		require.NoError(t, old.Close())

		listener, err := listenIPAMSocket(socketPath)
		require.NoError(t, err)
		require.NoError(t, listener.Close())
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
		req, requestErr := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost"+route, bytes.NewBufferString("{"))
		require.NoError(t, requestErr)
		response, responseErr := client.Do(req)
		require.NoError(t, responseErr)
		_, _ = io.Copy(io.Discard, response.Body)
		require.NoError(t, response.Body.Close())
		require.NotEqual(t, http.StatusNotFound, response.StatusCode)
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
