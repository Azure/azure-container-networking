//go:build linux

package client

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/stretchr/testify/require"
)

func TestNewWithUnixSocketPrefersUnixSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "cns.sock")
	var unixRequests atomic.Int32
	startUnixHTTPServer(t, socketPath, ipConfigsHandler(&unixRequests))

	var tcpRequests atomic.Int32
	tcpServer := httptest.NewServer(ipConfigsHandler(&tcpRequests))
	t.Cleanup(tcpServer.Close)

	client, err := NewWithUnixSocket(tcpServer.URL, socketPath, time.Second)
	require.NoError(t, err)

	_, err = client.RequestIPs(t.Context(), cns.IPConfigsRequest{})
	require.NoError(t, err)
	require.EqualValues(t, 1, unixRequests.Load())
	require.Zero(t, tcpRequests.Load())
}

func TestNewWithUnixSocketKeepsNonIPAMRoutesOnTCP(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "cns.sock")
	var unixRequests atomic.Int32
	startUnixHTTPServer(t, socketPath, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		unixRequests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))

	var tcpRequests atomic.Int32
	tcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		tcpRequests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(tcpServer.Close)

	client, err := NewWithUnixSocket(tcpServer.URL, socketPath, time.Second)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, tcpServer.URL+cns.GetNetworkContainerByOrchestratorContext, http.NoBody)
	require.NoError(t, err)
	response, err := client.client.Do(req)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.EqualValues(t, 1, tcpRequests.Load())
	require.Zero(t, unixRequests.Load())
}

func TestNewWithUnixSocketFallsBackBeforeConnecting(t *testing.T) {
	tests := []struct {
		name        string
		preparePath func(*testing.T, string)
	}{
		{name: "missing socket"},
		{
			name: "stale socket",
			preparePath: func(t *testing.T, socketPath string) {
				t.Helper()
				listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
				require.NoError(t, err)
				listener.SetUnlinkOnClose(false)
				require.NoError(t, listener.Close())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			socketPath := filepath.Join(t.TempDir(), "cns.sock")
			if tt.preparePath != nil {
				tt.preparePath(t, socketPath)
			}

			var tcpRequests atomic.Int32
			tcpServer := httptest.NewServer(ipConfigsHandler(&tcpRequests))
			t.Cleanup(tcpServer.Close)

			client, err := NewWithUnixSocket(tcpServer.URL, socketPath, time.Second)
			require.NoError(t, err)

			_, err = client.RequestIPs(t.Context(), cns.IPConfigsRequest{})
			require.NoError(t, err)
			require.EqualValues(t, 1, tcpRequests.Load())
		})
	}
}

func TestNewWithUnixSocketDoesNotReplayAfterUnixConnect(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "cns.sock")
	unixListener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	require.NoError(t, err)
	unixListener.SetUnlinkOnClose(false)
	t.Cleanup(func() {
		_ = unixListener.Close()
		_ = os.Remove(socketPath)
	})

	requestReceived := make(chan struct{})
	go func() {
		conn, acceptErr := unixListener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()

		req, readErr := http.ReadRequest(bufio.NewReader(conn))
		if readErr != nil {
			return
		}
		_, _ = io.Copy(io.Discard, req.Body)
		_ = req.Body.Close()
		close(requestReceived)
		_ = unixListener.Close()
	}()

	var tcpRequests atomic.Int32
	tcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		tcpRequests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(tcpServer.Close)

	client, err := NewWithUnixSocket(tcpServer.URL, socketPath, time.Second)
	require.NoError(t, err)

	// RequestIPs releases after an error. Neither the request nor that release may reach TCP.
	_, err = client.RequestIPs(t.Context(), cns.IPConfigsRequest{})
	require.Error(t, err)
	require.Eventually(t, func() bool {
		select {
		case <-requestReceived:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
	require.Zero(t, tcpRequests.Load())
}

func TestIsSafeUnixFallbackError(t *testing.T) {
	require.True(t, isSafeUnixFallbackError(&net.OpError{Err: syscall.ENOENT}))
	require.True(t, isSafeUnixFallbackError(&net.OpError{Err: syscall.ECONNREFUSED}))
	require.False(t, isSafeUnixFallbackError(&net.OpError{Err: syscall.EACCES}))
}

func startUnixHTTPServer(t *testing.T, socketPath string, handler http.Handler) {
	t.Helper()

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(t.Context(), "unix", socketPath)
	require.NoError(t, err)

	server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		_ = server.Close()
	})
}

func ipConfigsHandler(requests *atomic.Int32) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != cns.RequestIPConfigs {
			http.NotFound(w, req)
			return
		}

		requests.Add(1)
		w.Header().Set("Content-Type", contentTypeJSON)
		_, _ = io.WriteString(w, `{"response":{"ReturnCode":0}}`)
	})
}
