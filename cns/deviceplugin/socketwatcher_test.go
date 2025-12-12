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
	socket := filepath.Join("testdata", "socket.sock")
	f, createErr := os.Create(socket)
	if createErr != nil {
		t.Fatalf("error creating test file %s: %v", socket, createErr)
	}
	f.Close()
	defer os.Remove(socket)

	ctx, cancel := context.WithCancel(context.Background())
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	s := deviceplugin.NewSocketWatcher(logger)
	done := make(chan struct{})
	go func(done chan struct{}) {
		<-s.WatchSocket(ctx, socket)
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
	socket := filepath.Join("testdata", "to-be-deleted.sock")
	f, createErr := os.Create(socket)
	if createErr != nil {
		t.Fatalf("error creating test file %s: %v", socket, createErr)
	}
	f.Close()
	defer os.Remove(socket)

	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
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
	socket := filepath.Join("testdata", "to-be-deleted.sock")
	f, createErr := os.Create(socket)
	if createErr != nil {
		t.Fatalf("error creating test file %s: %v", socket, createErr)
	}
	f.Close()
	defer os.Remove(socket)

	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
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

func TestWatchSocketCleanup(t *testing.T) {
	socket := filepath.Join("testdata", "to-be-deleted.sock")
	f, createErr := os.Create(socket)
	if createErr != nil {
		t.Fatalf("error creating test file %s: %v", socket, createErr)
	}
	f.Close()
	defer os.Remove(socket)

	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	// Use a short interval for faster test execution
	s := deviceplugin.NewSocketWatcher(logger, deviceplugin.SocketWatcherStatInterval(100*time.Millisecond))

	// 1. Watch the socket
	ch1 := s.WatchSocket(context.Background(), socket)

	// Verify it's open
	select {
	case <-ch1:
		t.Fatal("channel should be open initially")
	default:
	}

	// 2. Delete the socket to trigger watcher exit
	if removeErr := os.Remove(socket); removeErr != nil {
		t.Fatalf("failed to remove socket: %v", removeErr)
	}

	// 3. Wait for ch1 to close
	select {
	case <-ch1:
		// Expected
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watcher to detect socket deletion")
	}

	// 4. Recreate the socket
	f, err = os.Create(socket)
	if err != nil {
		t.Fatalf("error recreating test file %s: %v", socket, err)
	}
	f.Close()

	// 5. Watch the socket again
	ch2 := s.WatchSocket(context.Background(), socket)

	// 6. Verify ch2 is open
	select {
	case <-ch2:
		t.Fatal("channel is closed but expected to be open")
	case <-time.After(200 * time.Millisecond):
		// Wait for at least one tick to ensure the watcher has had a chance to run.
	}
}
