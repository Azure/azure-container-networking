package fsnotify

import (
	"context"
	"errors"
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
	mu        sync.Mutex
	reqs      []cns.IPConfigsRequest
	errs      []error
	requestCh chan string
}

func (c *recordingReleaseClient) ReleaseIPs(_ context.Context, req cns.IPConfigsRequest) error {
	c.mu.Lock()
	c.reqs = append(c.reqs, req)
	var err error
	if len(c.errs) > 0 {
		err = c.errs[0]
		c.errs = c.errs[1:]
	}
	requestCh := c.requestCh
	c.mu.Unlock()
	if requestCh != nil {
		requestCh <- req.InfraContainerID
	}
	return err
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
	w.batchSize = 2
	w.pendingDelete = make(chan string, w.batchSize)
	for _, containerID := range []string{"container-1", "container-2", "container-3"} {
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
	w.batchSize = 1
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
	w.batchSize = 1
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

func TestWatchPendingDeleteContinuesAfterLocalFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deleteIDs")
	cli := &recordingReleaseClient{}
	w, err := New(cli, dir, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	w.retryInterval = 2 * time.Second

	if err := os.WriteFile(filepath.Join(dir, "empty-container"), nil, 0o600); err != nil {
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
	w.pendingDelete <- "empty-container"
	w.pendingDelete <- testContainerID
	waitForFileRemoval(t, filepath.Join(dir, testContainerID))

	if got := len(cli.snapshot()); got != 1 {
		t.Fatalf("want valid delete processed without waiting for local retry, got %d requests", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watchPendingDelete did not exit after cancellation")
	}
}

func TestConnectionFailureBackoff(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deleteIDs")
	connectionErr := cnsclient.NewConnectionFailureErr(errors.New("connection refused"))
	cli := &recordingReleaseClient{
		errs:      []error{connectionErr, connectionErr, connectionErr, connectionErr, nil, connectionErr},
		requestCh: make(chan string, 6),
	}
	w, err := New(cli, dir, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}

	waitIntervals := make(chan time.Duration)
	continueWait := make(chan struct{})
	w.waitForRetry = func(ctx context.Context, interval time.Duration) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case waitIntervals <- interval:
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-continueWait:
			return nil
		}
	}

	containerIDs := []string{"container-1", "container-2", "container-3", "container-4", "container-5", "container-6"}
	for _, containerID := range containerIDs {
		if err := AddFile(containerID+"-eth0", containerID, dir); err != nil {
			t.Fatal(err)
		}
		w.pendingDelete <- containerID
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- w.watchPendingDelete(ctx)
	}()

	nextRequest := func() string {
		t.Helper()
		select {
		case request := <-cli.requestCh:
			return request
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for release request")
			return ""
		}
	}
	nextWait := func() time.Duration {
		t.Helper()
		select {
		case interval := <-waitIntervals:
			return interval
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for backoff")
			return 0
		}
	}

	for i, wantInterval := range []time.Duration{30 * time.Second, time.Minute, maxReleaseInterval, maxReleaseInterval} {
		if got := nextRequest(); got != containerIDs[i] {
			t.Fatalf("want request for %q, got %q", containerIDs[i], got)
		}
		if got := nextWait(); got != wantInterval {
			t.Fatalf("want backoff %s, got %s", wantInterval, got)
		}
		select {
		case got := <-cli.requestCh:
			t.Fatalf("consumer processed %q while waiting after a connection failure", got)
		default:
		}
		continueWait <- struct{}{}
	}

	for _, want := range containerIDs[4:] {
		if got := nextRequest(); got != want {
			t.Fatalf("want request for %q, got %q", want, got)
		}
	}
	waitForFileRemoval(t, filepath.Join(dir, containerIDs[4]))
	if got := nextWait(); got != 30*time.Second {
		t.Fatalf("want reset backoff 30s, got %s", got)
	}

	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("want cancellation error")
		}
	case <-time.After(time.Second):
		t.Fatal("watcher did not interrupt backoff after cancellation")
	}
	if got := len(cli.snapshot()); got != len(containerIDs) {
		t.Fatalf("want %d release requests, got %d", len(containerIDs), got)
	}
}
