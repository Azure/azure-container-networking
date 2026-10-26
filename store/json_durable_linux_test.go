package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDurableJSONStoreRoundTripAndFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := NewDurableJSONFileStore(path, nil, zap.NewNop())
	require.NoError(t, err)
	require.True(t, store.(*jsonFileStore).durable)
	require.NoError(t, store.Write("reservation", "container"))
	fresh, err := NewDurableJSONFileStore(path, nil, zap.NewNop())
	require.NoError(t, err)
	var owner string
	require.NoError(t, fresh.Read("reservation", &owner))
	require.Equal(t, "container", owner)
	failed, err := NewDurableJSONFileStore(filepath.Join(t.TempDir(), "missing", "state.json"), nil, zap.NewNop())
	require.NoError(t, err)
	require.Error(t, failed.Write("reservation", "container"))
	require.Empty(t, failed.(*jsonFileStore).data)
}
