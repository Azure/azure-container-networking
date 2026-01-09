package deviceplugin

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

type mockDeviceCounter struct {
	count int
}

func (m *mockDeviceCounter) getDeviceCount() int {
	return m.count
}

func TestServer_Run_CleansUpExistingSocket(t *testing.T) {
	// Create a temporary directory for the socket
	socketPath := filepath.Join("testdata", "test.sock")
	defer os.Remove(socketPath)

	// Create a dummy file at the socket path to simulate a stale socket
	if err := os.WriteFile(socketPath, []byte("stale socket"), 0o600); err != nil {
		t.Fatalf("failed to create dummy socket file: %v", err)
	}

	logger := zap.NewNop()
	counter := &mockDeviceCounter{count: 1}
	server := NewServer(logger, socketPath, counter, time.Second)

	// Create a context that we can cancel to stop the server
	ctx, cancel := context.WithCancel(context.Background())

	// Run the server in a goroutine
	errChan := make(chan error)
	go func() {
		errChan <- server.Run(ctx)
	}()

	// Wait for the server to start up, delete the pre-existing file and recreate it as a socket
	// We verify this by trying to connect to the socket repeatedly until success or timeout
	var conn net.Conn
	var err error
	// Retry for up to 2 seconds
	for start := time.Now(); time.Since(start) < 2*time.Second; time.Sleep(200 * time.Millisecond) {
		conn, err = net.Dial("unix", socketPath)
		if err == nil {
			conn.Close()
			break
		}
	}

	if err != nil {
		t.Errorf("failed to connect to socket: %v", err)
	}

	// Stop the server
	cancel()

	// Wait for Run to return
	if err := <-errChan; err != nil {
		t.Errorf("server.Run returned error: %v", err)
	}
}
