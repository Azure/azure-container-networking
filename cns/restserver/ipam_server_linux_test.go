//go:build linux

package restserver

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/logger"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestListenIPAMSocket(t *testing.T) {
	requireRootSocketTests(t)
	t.Run("creates socket with mode 0600", func(t *testing.T) {
		socketPath := filepath.Join(socketTestDirectory(t), "cns.sock")

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
		socketPath := filepath.Join(socketTestDirectory(t), "cns.sock")
		old, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
		require.NoError(t, err)
		old.SetUnlinkOnClose(false)
		require.NoError(t, old.Close())

		listener, err := listenIPAMSocket(socketPath)
		require.NoError(t, err)
		require.NoError(t, listener.Close())
	})

	t.Run("preserves active socket", func(t *testing.T) {
		socketPath := filepath.Join(socketTestDirectory(t), "cns.sock")
		active, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
		require.NoError(t, err)
		t.Cleanup(func() { _ = active.Close() })
		before, err := os.Lstat(socketPath)
		require.NoError(t, err)

		listener, err := listenIPAMSocket(socketPath)
		if listener != nil {
			_ = listener.Close()
		}
		require.ErrorIs(t, err, errIPAMSocketActive)
		after, err := os.Lstat(socketPath)
		require.NoError(t, err)
		require.True(t, os.SameFile(before, after))
	})

	for _, kind := range []string{"file", "symlink", "directory"} {
		t.Run("preserves "+kind, func(t *testing.T) {
			socketPath := filepath.Join(socketTestDirectory(t), "cns.sock")
			switch kind {
			case "file":
				require.NoError(t, os.WriteFile(socketPath, []byte("keep"), 0o600))
			case "symlink":
				require.NoError(t, os.Symlink("missing-target", socketPath))
			case "directory":
				require.NoError(t, os.Mkdir(socketPath, 0o700))
			}
			before, err := os.Lstat(socketPath)
			require.NoError(t, err)
			listener, err := listenIPAMSocket(socketPath)
			if listener != nil {
				_ = listener.Close()
			}
			require.ErrorIs(t, err, errIPAMSocketType)
			after, err := os.Lstat(socketPath)
			require.NoError(t, err)
			require.True(t, os.SameFile(before, after))
		})
	}
}

func TestListenIPAMSocketRejectsUnsafeDirectory(t *testing.T) {
	requireRootSocketTests(t)
	t.Run("symlink", func(t *testing.T) {
		directory := filepath.Join(socketTestDirectory(t), "link")
		require.NoError(t, os.Symlink(socketTestDirectory(t), directory))
		_, err := listenIPAMSocket(filepath.Join(directory, "cns.sock"))
		require.ErrorIs(t, err, errIPAMSocketDirectory)
	})
	t.Run("permissions", func(t *testing.T) {
		directory := socketTestDirectory(t)
		require.NoError(t, os.Chmod(directory, 0o755))
		_, err := listenIPAMSocket(filepath.Join(directory, "cns.sock"))
		require.ErrorIs(t, err, errIPAMSocketDirectory)
		info, err := os.Lstat(directory)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	})
	t.Run("owner", func(t *testing.T) {
		directory := socketTestDirectory(t)
		require.NoError(t, os.Chown(directory, 1, 1))
		_, err := listenIPAMSocket(filepath.Join(directory, "cns.sock"))
		require.ErrorIs(t, err, errIPAMSocketDirectory)
	})
	t.Run("missing directory", func(t *testing.T) {
		_, err := listenIPAMSocket(filepath.Join(socketTestDirectory(t), "missing", "cns.sock"))
		require.ErrorIs(t, err, os.ErrNotExist)
	})
}

func requireRootSocketTests(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("socket directory tests require root")
	}
}

func socketTestDirectory(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	require.NoError(t, os.Chmod(directory, 0o700))
	return directory
}

func TestIPAMServerExposesOnlyIPAMRoutes(t *testing.T) {
	requireRootSocketTests(t)
	initSocketTestLogger(t)
	socketPath := filepath.Join(socketTestDirectory(t), "cns.sock")
	server, err := startIPAMServer(socketPath, &HTTPRestService{}, make(chan error, 1))
	require.NoError(t, err)
	t.Cleanup(server.stop)

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
		counter := ipamRequests.WithLabelValues(route, "unix")
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

func TestTCPIPAMRequestMetrics(t *testing.T) {
	initSocketTestLogger(t)
	config := &common.ServiceConfig{}
	service, err := NewHTTPRestService(config, &fakes.WireserverClientFake{}, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	service.SetOption(acn.OptCnsURL, "tcp://127.0.0.1:0")
	service.SetOption(acn.OptCnsPort, "")
	require.NoError(t, service.Init(config))

	server := httptest.NewServer(service.Listener.GetMux())
	t.Cleanup(server.Close)
	for _, route := range []string{cns.RequestIPConfig, cns.RequestIPConfigs, cns.ReleaseIPConfig, cns.ReleaseIPConfigs} {
		t.Run(route, func(t *testing.T) {
			counter := ipamRequests.WithLabelValues(route, "tcp")
			before := testutil.ToFloat64(counter)
			req, requestErr := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+route, bytes.NewBufferString("{"))
			require.NoError(t, requestErr)
			response, responseErr := server.Client().Do(req)
			require.NoError(t, responseErr)
			require.NoError(t, response.Body.Close())
			require.NotEqual(t, http.StatusNotFound, response.StatusCode)
			require.InDelta(t, before+1, testutil.ToFloat64(counter), 0)
		})
	}
}

func initSocketTestLogger(t *testing.T) {
	t.Helper()
	if logger.Log == nil {
		log, err := logger.New("uds-tests.log", 0, 0, t.TempDir()+"/") //nolint:staticcheck // existing handlers require the global logger
		require.NoError(t, err)
		logger.Log = log
	}
}
