package store

import (
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestDurableJSONStoreRoundTripAndFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := NewDurableJSONFileStore(path, nil, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if !store.(*jsonFileStore).durable {
		t.Error("new durable store has durability disabled")
	}
	if err = store.Write("reservation", "container"); err != nil {
		t.Fatal(err)
	}
	fresh, err := NewDurableJSONFileStore(path, nil, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	var owner string
	if err = fresh.Read("reservation", &owner); err != nil {
		t.Fatal(err)
	}
	if owner != "container" {
		t.Errorf("restored owner = %q, want container", owner)
	}
	failed, err := NewDurableJSONFileStore(filepath.Join(t.TempDir(), "missing", "state.json"), nil, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err := failed.Write("reservation", "container"); err == nil {
		t.Error("write to missing parent directory succeeded")
	}
	if got := len(failed.(*jsonFileStore).data); got != 0 {
		t.Errorf("failed write retained %d store entries, want 0", got)
	}
}
