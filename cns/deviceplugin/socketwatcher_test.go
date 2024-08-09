package deviceplugin_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns/deviceplugin"
	"go.uber.org/zap"
)

func TestWatchContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	logger, _ := zap.NewDevelopment()
	s := deviceplugin.NewSocketWatcher(logger)
	done := make(chan struct{})
	go func(done chan struct{}) {
		<-s.WatchSocket(ctx, "testdata/socket.sock")
		close(done)
	}(done)

	// done chan should stil be open
	select {
	case <-done:
		t.Fatal("socket watcher isn't watching but the context is still not cancelled")
	default:
	}

	cancel()

	// done chan should be closed since the context was cancelled
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("socket watcher is still watching 5 seconds after context is cancelled")
	}
}

func TestWatchSocketDeleted(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "socket-watcher-test-")
	if err != nil {
		t.Fatalf("error creating temporary directory: %v", err)
	}
	defer os.RemoveAll(tempDir) // Ensure the directory is cleaned up

	socket := filepath.Join(tempDir, "to-be-deleted.sock")
	if _, err := os.Create(socket); err != nil {
		t.Fatalf("error creating test file %s: %v", socket, err)
	}

	logger, _ := zap.NewDevelopment()
	s := deviceplugin.NewSocketWatcher(logger, deviceplugin.SocketWatcherStatInterval(time.Second))
	done := make(chan struct{})
	go func(done chan struct{}) {
		<-s.WatchSocket(context.Background(), socket)
		close(done)
	}(done)

	// done chan should stil be open
	select {
	case <-done:
		t.Fatal("socket watcher isn't watching but the file still exists")
	default:
	}

	if err := os.Remove(socket); err != nil && !os.IsNotExist(err) {
		t.Fatalf("failed to remove socket")
	}

	// done chan should be closed since the socket file was deleted
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("socket watcher is still watching 5 seconds after file is deleted")
	}
}

func TestWatchSocketTwice(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "socket-watcher-test-")
	if err != nil {
		t.Fatalf("error creating temporary directory: %v", err)
	}
	defer os.RemoveAll(tempDir) // Ensure the directory is cleaned up

	socket := filepath.Join(tempDir, "to-be-deleted.sock")
	if _, err := os.Create(socket); err != nil {
		t.Fatalf("error creating test file %s: %v", socket, err)
	}

	logger, _ := zap.NewDevelopment()
	s := deviceplugin.NewSocketWatcher(logger, deviceplugin.SocketWatcherStatInterval(time.Second))
	done1 := make(chan struct{})
	done2 := make(chan struct{})
	go func(done chan struct{}) {
		<-s.WatchSocket(context.Background(), socket)
		close(done)
	}(done1)
	go func(done chan struct{}) {
		<-s.WatchSocket(context.Background(), socket)
		close(done)
	}(done2)

	// done chans should stil be open
	select {
	case <-done1:
		t.Fatal("socket watcher isn't watching but the file still exists")
	default:
	}

	select {
	case <-done2:
		t.Fatal("socket watcher isn't watching but the file still exists")
	default:
	}

	if err := os.Remove(socket); err != nil && !os.IsNotExist(err) {
		t.Fatalf("failed to remove socket")
	}

	// done chans should be closed since the socket file was deleted
	select {
	case <-done1:
	case <-time.After(5 * time.Second):
		t.Fatal("socket watcher is still watching 5 seconds after file is deleted")
	}

	select {
	case <-done2:
	case <-time.After(5 * time.Second):
		t.Fatal("socket watcher is still watching 5 seconds after file is deleted")
	}
}
