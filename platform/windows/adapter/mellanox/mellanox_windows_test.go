// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package mellanox

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/registry"
)

func TestRegistryVLANTag(t *testing.T) {
	path := `Software\ACN-MellanoxTest-` + filepath.Base(filepath.Dir(t.TempDir()))
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, key.Close())
		require.NoError(t, registry.DeleteKey(registry.CURRENT_USER, path))
	})

	_, err = readRegistryVLANTag(registry.CURRENT_USER, path)
	require.ErrorIs(t, err, registry.ErrNotExist)

	require.NoError(t, writeRegistryVLANTag(registry.CURRENT_USER, path, 3))
	value, valueType, err := key.GetStringValue(priorityVLANTagIdentifier)
	require.NoError(t, err)
	require.Equal(t, uint32(registry.SZ), valueType)
	require.Equal(t, "3", value)

	got, err := readRegistryVLANTag(registry.CURRENT_USER, path)
	require.NoError(t, err)
	require.Equal(t, 3, got)

	require.NoError(t, key.SetDWordValue(priorityVLANTagIdentifier, 2))
	got, err = readRegistryVLANTag(registry.CURRENT_USER, path)
	require.NoError(t, err)
	require.Equal(t, 2, got)

	require.NoError(t, key.SetQWordValue(priorityVLANTagIdentifier, 1))
	got, err = readRegistryVLANTag(registry.CURRENT_USER, path)
	require.NoError(t, err)
	require.Equal(t, 1, got)

	require.NoError(t, key.SetStringsValue(priorityVLANTagIdentifier, []string{" 3 "}))
	got, err = readRegistryVLANTag(registry.CURRENT_USER, path)
	require.NoError(t, err)
	require.Equal(t, 3, got)

	require.NoError(t, key.SetStringsValue(priorityVLANTagIdentifier, []string{"2", "3"}))
	_, err = readRegistryVLANTag(registry.CURRENT_USER, path)
	require.Error(t, err)

	require.NoError(t, key.SetStringValue(priorityVLANTagIdentifier, "invalid"))
	_, err = readRegistryVLANTag(registry.CURRENT_USER, path)
	require.Error(t, err)

	require.NoError(t, key.SetBinaryValue(priorityVLANTagIdentifier, []byte{3}))
	_, err = readRegistryVLANTag(registry.CURRENT_USER, path)
	require.ErrorIs(t, err, registry.ErrUnexpectedType)

	require.Error(t, writeRegistryVLANTag(registry.CURRENT_USER, path+`\missing`, 3))
}
