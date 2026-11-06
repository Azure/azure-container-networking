package fsnotify

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	cnsclient "github.com/Azure/azure-container-networking/cns/client"
	"go.uber.org/zap"
)

const (
	testContainerID    = "56dba5daf3ed3aaab57f13047b0abbebfe2bdb05156896e2531beb6a2a0b15ae"
	testPodInterfaceID = "56dba5da-eth0"
)

type recordingReleaseClient struct {
	mu         sync.Mutex
	reqs       []cns.IPConfigsRequest
	releaseErr error
}

func (c *recordingReleaseClient) ReleaseIPs(_ context.Context, req cns.IPConfigsRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reqs = append(c.reqs, req)
	return c.releaseErr
}

func (c *recordingReleaseClient) snapshot() []cns.IPConfigsRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]cns.IPConfigsRequest(nil), c.reqs...)
}

func waitForFileRemoval(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		_, err := os.Stat(path)
		if os.IsNotExist(err) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s to be removed", path)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestEnqueuePendingDeletesUsesBackpressure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deleteIDs")
	w, err := New(&recordingReleaseClient{}, dir, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= pendingDeleteBatchSize; i++ {
		containerID := fmt.Sprintf("container-%03d", i)
		if err := AddFile(containerID+"-eth0", containerID, dir); err != nil {
			t.Fatal(err)
		}
	}

	done := make(chan error, 1)
	go func() {
		done <- w.enqueuePendingDeletes(context.Background())
	}()
	select {
	case err := <-done:
		t.Fatalf("producer completed before the bounded channel was drained: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	<-w.pendingDelete
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("producer did not resume after the consumer freed channel capacity")
	}
}

func TestEnqueuePendingDeletesFindsFilesCreatedAfterEOF(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deleteIDs")
	w, err := New(&recordingReleaseClient{}, dir, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddFile("container-1-eth0", "container-1", dir); err != nil {
		t.Fatal(err)
	}

	if err := w.enqueuePendingDeletes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := <-w.pendingDelete; got != "container-1" {
		t.Fatalf("want container-1, got %q", got)
	}
	if err := os.Remove(filepath.Join(dir, "container-1")); err != nil {
		t.Fatal(err)
	}

	if err := AddFile("container-2-eth0", "container-2", dir); err != nil {
		t.Fatal(err)
	}
	if err := w.enqueuePendingDeletes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := <-w.pendingDelete; got != "container-2" {
		t.Fatalf("want container-2 after reopening directory, got %q", got)
	}
}

func TestWatchFSEnqueuesWrittenFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deleteIDs")
	w, err := New(&recordingReleaseClient{}, dir, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddFile("preseed-eth0", "preseed", dir); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- w.watchFS(ctx)
	}()
	select {
	case got := <-w.pendingDelete:
		if got != "preseed" {
			t.Fatalf("want initial preseed entry, got %q", got)
		}
	case err := <-done:
		t.Fatalf("watchFS exited before initial scan: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial scan")
	}
	if err := os.Remove(filepath.Join(dir, "preseed")); err != nil {
		t.Fatal(err)
	}

	if err := AddFile(testPodInterfaceID, testContainerID, dir); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-w.pendingDelete:
		if got != testContainerID {
			t.Fatalf("want live container ID %q, got %q", testContainerID, got)
		}
	case err := <-done:
		t.Fatalf("watchFS exited before enqueueing live file: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live file reconciliation")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watchFS did not exit after cancellation")
	}
}

func TestWatchPendingDeleteRemovesReleasedFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deleteIDs")
	cli := &recordingReleaseClient{}
	w, err := New(cli, dir, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddFile(testPodInterfaceID, testContainerID, dir); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- w.watchPendingDelete(ctx)
	}()
	w.pendingDelete <- "already-removed"
	w.pendingDelete <- testContainerID
	waitForFileRemoval(t, filepath.Join(dir, testContainerID))

	reqs := cli.snapshot()
	if len(reqs) != 1 {
		t.Fatalf("want 1 release request, got %+v", reqs)
	}
	if reqs[0].PodInterfaceID != testPodInterfaceID || reqs[0].InfraContainerID != testContainerID {
		t.Errorf("CNS got PodInterfaceID=%q InfraContainerID=%q, want %q and %q",
			reqs[0].PodInterfaceID, reqs[0].InfraContainerID, testPodInterfaceID, testContainerID)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watchPendingDelete did not exit after cancellation")
	}
}

func TestWatchPendingDeleteContinuesAfterNonConnectionError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deleteIDs")
	cli := &recordingReleaseClient{releaseErr: errors.New("rejected by CNS")}
	w, err := New(cli, dir, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	for _, containerID := range []string{"container-1", "container-2"} {
		if err := AddFile(containerID+"-eth0", containerID, dir); err != nil {
			t.Fatal(err)
		}
		w.pendingDelete <- containerID
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- w.watchPendingDelete(ctx)
	}()
	deadline := time.Now().Add(time.Second)
	for len(cli.snapshot()) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("want both deletes attempted, got %d", len(cli.snapshot()))
		}
		time.Sleep(time.Millisecond)
	}
	for _, containerID := range []string{"container-1", "container-2"} {
		if _, err := os.Stat(filepath.Join(dir, containerID)); err != nil {
			t.Fatalf("failed delete file should remain: %v", err)
		}
	}
	cancel()
	<-done
}

func TestStartReturnsConnectionFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deleteIDs")
	releaseErr := cnsclient.NewConnectionFailureErr(errors.New("connection refused"))
	cli := &recordingReleaseClient{releaseErr: releaseErr}
	w, err := New(cli, dir, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddFile(testPodInterfaceID, testContainerID, dir); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = w.Start(ctx)
	if !errors.Is(err, releaseErr) {
		t.Fatalf("want connection error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, testContainerID)); err != nil {
		t.Fatalf("pending delete file should remain for outer retry: %v", err)
	}
	if got := len(cli.snapshot()); got != 1 {
		t.Fatalf("want 1 release request, got %d", got)
	}
}
