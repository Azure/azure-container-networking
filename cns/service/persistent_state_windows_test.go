//go:build windows

// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolvePersistentStatePaths(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		paths := resolvePersistentStatePaths("", "")
		require.Equal(t, ".", paths.stateDirectory)
		require.Equal(t, "azure-cns.json", paths.stateFile)
		require.Equal(t, "azure-cns.db", paths.databaseFile)
		require.Equal(t, "/k/azurecns/azure-endpoints.json", paths.endpointFile)
		require.Equal(t, "azure-cns.lock", paths.stateLockFile)
		require.Equal(t, "azure-endpoints.lock", paths.endpointLockFile)
	})

	t.Run("configured directories preserve legacy concatenation", func(t *testing.T) {
		paths := resolvePersistentStatePaths(`C:\cns\`, `C:\endpoints\`)
		require.Equal(t, `C:\cns\azure-cns.json`, paths.stateFile)
		require.Equal(t, `C:\cns\azure-cns.db`, paths.databaseFile)
		require.Equal(t, `C:\endpoints\azure-endpoints.json`, paths.endpointFile)
	})
}
