package persistence

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/store"
	"github.com/google/go-cmp/cmp"
	"go.uber.org/zap"
)

const testPoolParentOwner = "pod-a"

func updatePoolStateInChild(t *testing.T, dir string) {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	path := filepath.Join(dir, "ipam.db")
	lock, err := store.NewBoltStore(path, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err = root.WriteFile("ipam.db.started", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = lock.Lock(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			t.Errorf("unlocking child IPAM state: %v", unlockErr)
		}
	}()
	if err = root.WriteFile("ipam.db.acquired", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var usage map[string]string
	if err = lock.Read("IPAM", &usage); err != nil {
		t.Fatal(err)
	}
	if usage["parent"] != testPoolParentOwner {
		t.Fatalf("child did not restore parent's reservation: %v", usage)
	}
	usage["child"] = "pod-b"
	if err = lock.Write("IPAM", usage); err != nil {
		t.Fatal(err)
	}
}

func TestPoolBridgeProcessLock(t *testing.T) {
	if dir := os.Getenv("ACN_POOL_LOCK_CHILD"); dir != "" {
		updatePoolStateInChild(t, dir)
		return
	}
	path := filepath.Join(t.TempDir(), "ipam.db")
	lock, err := lockStore(t.Context(), path, time.Millisecond, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer unlockStore(lock, zap.NewNop())
	if err = lock.Write("IPAM", map[string]string{"parent": testPoolParentOwner}); err != nil {
		t.Fatal(err)
	}
	otherLock, err := lockStore(t.Context(), path, time.Millisecond, zap.NewNop())
	if err == nil {
		if unlockErr := otherLock.Unlock(); unlockErr != nil {
			t.Error(unlockErr)
		}
		t.Fatal("single-publisher lifetime lock was acquired twice")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	childCtx, stopChild := context.WithTimeout(t.Context(), 10*time.Second)
	defer stopChild()
	cmd := exec.CommandContext(childCtx, executable, "-test.run=^TestPoolBridgeProcessLock$")
	cmd.Env = append(os.Environ(), "ACN_POOL_LOCK_CHILD="+filepath.Dir(path))
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	for {
		_, statErr := os.Stat(path + ".started")
		if statErr == nil {
			break
		}
		if !os.IsNotExist(statErr) {
			t.Fatal(statErr)
		}
		select {
		case <-poll.C:
		case <-timeout.C:
			t.Fatal("child did not start its lock attempt")
		}
	}
	if _, statErr := os.Stat(path + ".acquired"); !os.IsNotExist(statErr) {
		t.Fatalf("child acquired a held lock or marker could not be inspected: %v", statErr)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	canceledLock, err := lockStore(ctx, path, time.Second, zap.NewNop())
	if canceledLock != nil {
		if unlockErr := canceledLock.Unlock(); unlockErr != nil {
			t.Error(unlockErr)
		}
		t.Error("canceled lock attempt returned an open lock")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("canceled lock attempt: got %v, want %v", err, context.Canceled)
	}
	if err = lock.Unlock(); err != nil {
		t.Fatal(err)
	}
	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if info, statErr := os.Stat(path + ".acquired"); statErr != nil {
		t.Fatal(statErr)
	} else if !info.Mode().IsRegular() {
		t.Error("child lock acquisition marker is not a regular file")
	}
	if err = lock.Lock(time.Second); err != nil {
		t.Fatal(err)
	}
	var usage map[string]string
	if err = lock.Read("IPAM", &usage); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(map[string]string{"parent": testPoolParentOwner, "child": "pod-b"}, usage); diff != "" {
		t.Errorf("reservations after process handoff (-want +got):\n%s", diff)
	}
}
