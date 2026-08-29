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

func hasKey(w *watcher, key string) bool {
	w.lock.Lock()
	defer w.lock.Unlock()
	_, ok := w.pendingDelete[key]
	return ok
}

func waitForKey(t *testing.T, w *watcher, key string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !hasKey(w, key) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for pending key %q", key)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestLiveCreateEventReleasesWithRealKeys guards the async-delete leak.
//
// fsnotify sets event.Name to the full path of the new file, but releaseAll
// rebuilds the path as w.path + "/" + key. A map keyed by the full path made
// every live-enqueued file unreadable, so the release reached CNS with an
// empty PodInterfaceID and the pod IP stayed Assigned until the next CNS
// restart. The map must be keyed by the container ID: the file's base name.
func TestLiveCreateEventReleasesWithRealKeys(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deleteIDs")
	cli := &recordingReleaseClient{}
	w, err := New(cli, dir, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}

	// A pre-seeded file marks the end of the startup scan. watchFS attaches
	// the inotify watch first and lists the directory second, so once this
	// key appears, every later file reaches the map through a create event
	// only. Without this handshake the test file could be picked up by the
	// startup scan, which keys correctly even on the broken code.
	const preseed = "preseed-scan-marker"
	if err := AddFile("preseed-eth0", preseed, dir); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = w.watchFS(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("watchFS did not exit after cancellation")
		}
	})
	waitForKey(t, w, preseed)

	// The CNI enqueues a missed delete exactly as invoker_cns.go does. Only
	// the create event can deliver this key now. On the broken code the key
	// is the full path and this wait times out.
	if err := AddFile(testPodInterfaceID, testContainerID, dir); err != nil {
		t.Fatal(err)
	}
	waitForKey(t, w, testContainerID)

	// Drop the preseed entry so the release assertions see one request.
	w.lock.Lock()
	delete(w.pendingDelete, preseed)
	w.lock.Unlock()
	if err := os.Remove(filepath.Join(dir, preseed)); err != nil {
		t.Fatal(err)
	}

	w.releaseAll(ctx)

	reqs := cli.snapshot()
	if len(reqs) != 1 {
		t.Fatalf("want 1 release request, got %+v", reqs)
	}
	if reqs[0].PodInterfaceID != testPodInterfaceID || reqs[0].InfraContainerID != testContainerID {
		t.Errorf("LEAK: CNS got PodInterfaceID=%q InfraContainerID=%q, want %q and %q",
			reqs[0].PodInterfaceID, reqs[0].InfraContainerID, testPodInterfaceID, testContainerID)
	}
	if _, statErr := os.Stat(filepath.Join(dir, testContainerID)); statErr == nil {
		t.Errorf("LEAK: pending delete file was left on disk")
	}
	if hasKey(w, testContainerID) {
		t.Errorf("released entry must be dropped from pendingDelete")
	}
}

// TestReleaseAllSkipsEntryWithoutFile checks the guard: an entry whose file
// cannot be read must not produce a release, because the request would carry
// an empty pod interface ID and can never match an assignment.
func TestReleaseAllSkipsEntryWithoutFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deleteIDs")
	cli := &recordingReleaseClient{}
	w, err := New(cli, dir, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	w.pendingDelete["no-such-file"] = struct{}{}

	w.releaseAll(context.Background())

	if reqs := cli.snapshot(); len(reqs) != 0 {
		t.Errorf("no release must be sent for an unreadable entry, got %+v", reqs)
	}
}
