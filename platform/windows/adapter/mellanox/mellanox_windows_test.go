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

func TestAdapterRegistryPath(t *testing.T) {
	classPath := `Software\ACN-MellanoxTest-` + filepath.Base(filepath.Dir(t.TempDir()))
	createKey := func(path string) registry.Key {
		t.Helper()
		key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, key.Close())
			require.NoError(t, registry.DeleteKey(registry.CURRENT_USER, path))
		})
		return key
	}

	createKey(classPath)
	adapterPath := classPath + `\0000`
	adapter := createKey(adapterPath)
	require.NoError(t, adapter.SetStringValue("NetCfgInstanceId", "{A1B2}"))
	other := createKey(classPath + `\0001`)
	require.NoError(t, other.SetStringValue("NetCfgInstanceId", "{C3D4}"))
	createKey(classPath + `\0002`)
	properties := createKey(classPath + `\Properties`)
	require.NoError(t, properties.SetStringValue("NetCfgInstanceId", "{A1B2}"))

	path, err := adapterRegistryPath(registry.CURRENT_USER, classPath, "{a1b2}")
	require.NoError(t, err)
	require.Equal(t, adapterPath, path)
	_, err = adapterRegistryPath(registry.CURRENT_USER, classPath, "{missing}")
	require.Error(t, err)
	require.NoError(t, other.SetStringValue("NetCfgInstanceId", "{a1b2}"))
	_, err = adapterRegistryPath(registry.CURRENT_USER, classPath, "{a1b2}")
	require.Error(t, err)
	require.NoError(t, other.SetStringValue("NetCfgInstanceId", "{C3D4}"))

	require.Error(t, checkVLANTagKeyword(registry.CURRENT_USER, adapterPath))
	createKey(adapterPath + `\Ndi`)
	params := adapterPath + `\Ndi\params`
	createKey(params)
	createKey(params + `\*PriorityVLANTag`)
	require.NoError(t, checkVLANTagKeyword(registry.CURRENT_USER, adapterPath))
	createKey(params + `\OtherPriorityVLANTag`)
	require.Error(t, checkVLANTagKeyword(registry.CURRENT_USER, adapterPath))
}
