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
	mu            sync.Mutex
	reqs          []cns.IPConfigsRequest
	connectionErr bool
}

func (c *recordingReleaseClient) ReleaseIPs(_ context.Context, req cns.IPConfigsRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reqs = append(c.reqs, req)
	if c.connectionErr {
		return cnsclient.NewConnectionFailureErr(errors.New("connection refused"))
	}
	return nil
}

func (c *recordingReleaseClient) snapshot() []cns.IPConfigsRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]cns.IPConfigsRequest(nil), c.reqs...)
}

func TestReleaseBatchProcessesFilesCreatedAfterStartup(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deleteIDs")
	cli := &recordingReleaseClient{}
	w, err := New(cli, dir, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}

	if err := AddFile(testPodInterfaceID, testContainerID, dir); err != nil {
		t.Fatal(err)
	}

	if _, err := w.releaseBatch(context.Background()); err != nil {
		t.Fatal(err)
	}

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
}

func TestReleaseBatchIsBounded(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deleteIDs")
	cli := &recordingReleaseClient{}
	w, err := New(cli, dir, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	w.batchSize = 2
	for _, containerID := range []string{"container-1", "container-2", "container-3"} {
		if err := AddFile(containerID+"-eth0", containerID, dir); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := w.releaseBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := len(cli.snapshot()); got != 2 {
		t.Fatalf("want 2 release requests in first batch, got %d", got)
	}

	if _, err := w.releaseBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := len(cli.snapshot()); got != 3 {
		t.Fatalf("want 3 total release requests, got %d", got)
	}
}

func TestReleaseBatchStopsOnConnectionFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deleteIDs")
	cli := &recordingReleaseClient{connectionErr: true}
	w, err := New(cli, dir, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	for _, containerID := range []string{"container-1", "container-2"} {
		if err := AddFile(containerID+"-eth0", containerID, dir); err != nil {
			t.Fatal(err)
		}
	}

	connectionFailure, err := w.releaseBatch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !connectionFailure {
		t.Fatal("expected connection failure")
	}
	if got := len(cli.snapshot()); got != 1 {
		t.Fatalf("want release processing to stop after 1 request, got %d", got)
	}

	cli.mu.Lock()
	cli.connectionErr = false
	cli.mu.Unlock()
	if _, err := w.releaseBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := len(cli.snapshot()); got != 3 {
		t.Fatalf("want failed entry and remaining entry retried, got %d total requests", got)
	}
}

func TestNextReleaseInterval(t *testing.T) {
	interval := releaseInterval
	for _, want := range []time.Duration{30 * time.Second, time.Minute, maxReleaseInterval, maxReleaseInterval} {
		interval = nextReleaseInterval(interval, true)
		if interval != want {
			t.Fatalf("want interval %s, got %s", want, interval)
		}
	}
	if got := nextReleaseInterval(interval, false); got != releaseInterval {
		t.Fatalf("want successful release interval %s, got %s", releaseInterval, got)
	}
}
