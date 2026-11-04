package fsnotify

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"go.uber.org/zap"
)

const (
	testContainerID    = "56dba5daf3ed3aaab57f13047b0abbebfe2bdb05156896e2531beb6a2a0b15ae"
	testPodInterfaceID = "56dba5da-eth0"
)

type recordingReleaseClient struct {
	mu   sync.Mutex
	reqs []cns.IPConfigsRequest
}

func (c *recordingReleaseClient) ReleaseIPs(_ context.Context, req cns.IPConfigsRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reqs = append(c.reqs, req)
	return nil
}

func (c *recordingReleaseClient) snapshot() []cns.IPConfigsRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]cns.IPConfigsRequest(nil), c.reqs...)
}

func TestReadPendingDeletesUsesBoundedChannel(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deleteIDs")
	w, err := New(&recordingReleaseClient{}, dir, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	w.pendingDelete = make(chan string, 2)
	for _, containerID := range []string{"container-1", "container-2", "container-3"} {
		if err := AddFile(containerID+"-eth0", containerID, dir); err != nil {
			t.Fatal(err)
		}
	}

	done := make(chan error, 1)
	go func() {
		done <- w.readPendingDeletes(context.Background())
	}()

	select {
	case err := <-done:
		t.Fatalf("read completed before the bounded channel was drained: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	<-w.pendingDelete
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("read did not resume after channel capacity became available")
	}
}

func TestReleaseAllProcessesPendingDelete(t *testing.T) {
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
	defer cancel()
	go w.releaseAll(ctx)
	w.pendingDelete <- testContainerID

	deadline := time.Now().Add(time.Second)
	for len(cli.snapshot()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	reqs := cli.snapshot()
	if len(reqs) != 1 {
		t.Fatalf("want 1 release request, got %+v", reqs)
	}
	if reqs[0].PodInterfaceID != testPodInterfaceID || reqs[0].InfraContainerID != testContainerID {
		t.Errorf("CNS got PodInterfaceID=%q InfraContainerID=%q, want %q and %q",
			reqs[0].PodInterfaceID, reqs[0].InfraContainerID, testPodInterfaceID, testContainerID)
	}
	if _, statErr := os.Stat(filepath.Join(dir, testContainerID)); !os.IsNotExist(statErr) {
		t.Errorf("pending delete file was left on disk")
	}
}
