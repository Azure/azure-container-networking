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
