package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/zap"
)

func TestBoltStoreCommitAndReopen(t *testing.T) {
	kvs, err := NewBoltStore(filepath.Join(t.TempDir(), "state.db"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err = kvs.Lock(time.Second); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if unlockErr := kvs.Unlock(); unlockErr != nil && !errors.Is(unlockErr, ErrStoreNotLocked) {
			t.Error(unlockErr)
		}
	})
	want := map[string]string{"10.20.16.10": "pod-a"}
	if err = kvs.Write("IPAM", want); err != nil {
		t.Fatal(err)
	}
	if err = kvs.Write("IPAM", make(chan int)); err == nil {
		t.Fatal("writing an unserializable reservation succeeded")
	}
	if err = kvs.Write("Network", "retained"); err != nil {
		t.Fatal(err)
	}
	if err = kvs.Unlock(); err != nil {
		t.Fatal(err)
	}
	if err = kvs.Lock(time.Second); err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err = kvs.Read("IPAM", &got); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("committed reservations after failed write/reopen (-want +got):\n%s", diff)
	}
	var network string
	if err = kvs.Read("Network", &network); err != nil {
		t.Fatal(err)
	}
	if network != "retained" {
		t.Errorf("unrelated state = %q, want retained", network)
	}
	if err = kvs.Read("missing", &network); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("missing key: got %v, want %v", err, ErrKeyNotFound)
	}
}

func TestBoltStorePreservesInvalidDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	invalid := []byte(`{"IPAM":`)
	if err := os.WriteFile(path, invalid, 0o600); err != nil {
		t.Fatal(err)
	}

	kvs, err := NewBoltStore(path, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err = kvs.Lock(time.Second); err == nil {
		if unlockErr := kvs.Unlock(); unlockErr != nil {
			t.Error(unlockErr)
		}
		t.Fatal("invalid database was opened as empty state")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(invalid, after) {
		t.Error("invalid database was overwritten")
	}
}

func TestBoltReadOnlyAndObservation(t *testing.T) {
	const persisted = "persisted"
	path := filepath.Join(t.TempDir(), "state.db")
	read, err := NewReadOnlyBoltStore(path, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err = read.Lock(time.Second); err == nil {
		t.Fatal("read-only open created a database")
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("read-only observation created state: %v", err)
	}
	writer, err := NewBoltStore(path, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	var operations []string
	writer.Observe = func(operation string, duration time.Duration, err error) {
		if duration < 0 {
			t.Error("negative operation duration")
		}
		if err != nil {
			operation += "_error"
		}
		operations = append(operations, operation)
	}
	if err = writer.Lock(time.Second); err != nil {
		t.Fatal(err)
	}
	if err = writer.Write("value", persisted); err != nil {
		t.Fatal(err)
	}
	if err = writer.Write("value", make(chan int)); err == nil {
		t.Fatal("invalid write succeeded")
	}
	if err = writer.Unlock(); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"lock", "write", "write_error"}, operations); diff != "" {
		t.Fatalf("operation observations (-want +got): %s", diff)
	}
	if err = read.Lock(time.Second); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if unlockErr := read.Unlock(); unlockErr != nil {
			t.Error(unlockErr)
		}
	}()
	var value string
	if err = read.Read("value", &value); err != nil || value != persisted {
		t.Fatalf("read-only snapshot = %q, %v", value, err)
	}
	if err = read.Write("value", "overwritten"); err == nil {
		t.Fatal("read-only snapshot allowed a write")
	}
}
