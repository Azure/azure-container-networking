//go:build linux

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
		require.Equal(t, "/var/lib/azure-network/azure-cns.json", paths.stateFile)
		require.Equal(t, "/var/lib/azure-network/azure-cns.db", paths.databaseFile)
		require.Equal(t, "/var/run/azure-cns/azure-endpoints.json", paths.endpointFile)
		require.Equal(t, "/var/run/azure-vnet/azure-cns.lock", paths.stateLockFile)
		require.Equal(t, "/var/run/azure-vnet/azure-endpoints.lock", paths.endpointLockFile)
	})

	t.Run("configured directories preserve legacy concatenation", func(t *testing.T) {
		paths := resolvePersistentStatePaths("/custom/cns/", "/custom/endpoints/")
		require.Equal(t, "/custom/cns/azure-cns.json", paths.stateFile)
		require.Equal(t, "/custom/cns/azure-cns.db", paths.databaseFile)
		require.Equal(t, "/custom/endpoints/azure-endpoints.json", paths.endpointFile)
	})
}
