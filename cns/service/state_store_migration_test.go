// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package main

import (
	"path/filepath"
	"testing"

	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/processlock"
	"github.com/Azure/azure-container-networking/store"
	"github.com/stretchr/testify/require"
)

type testState struct {
	Value string `json:"value"`
}

func TestMigrateStateStoresJSONToBolt(t *testing.T) {
	tempDir := t.TempDir()

	mainJSON := filepath.Join(tempDir, "azure-cns.json")
	mainBolt := filepath.Join(tempDir, "azure-cns.db")
	endpointJSON := filepath.Join(tempDir, "azure-endpoints.json")
	endpointBolt := filepath.Join(tempDir, "azure-endpoints.db")

	mainLock := processlock.NewMockFileLock(false)
	endpointLock := processlock.NewMockFileLock(false)

	jsonMain, err := store.NewJsonFileStore(mainJSON, mainLock, nil)
	require.NoError(t, err)
	jsonEndpoint, err := store.NewJsonFileStore(endpointJSON, endpointLock, nil)
	require.NoError(t, err)

	require.NoError(t, jsonMain.Write(restserver.StoreKey, testState{Value: "main"}))
	endpointState := map[string]*restserver.EndpointInfo{
		"container1": {
			PodName:      "pod1",
			PodNamespace: "default",
		},
	}
	require.NoError(t, jsonEndpoint.Write(restserver.EndpointStoreKey, endpointState))

	boltMain, err := store.NewBoltDBStore(mainBolt, mainLock, nil)
	require.NoError(t, err)
	boltEndpoint, err := store.NewBoltDBStore(endpointBolt, endpointLock, nil)
	require.NoError(t, err)

	require.NoError(t, migrateStateStores(
		configuration.StateStoreMigrationModeJSONToBolt,
		boltMain,
		mainJSON,
		mainLock,
		boltEndpoint,
		endpointJSON,
		endpointLock,
		true,
	))

	var mainResult testState
	require.NoError(t, boltMain.Read(restserver.StoreKey, &mainResult))
	require.Equal(t, "main", mainResult.Value)

	var endpointResult map[string]*restserver.EndpointInfo
	require.NoError(t, boltEndpoint.Read(restserver.EndpointStoreKey, &endpointResult))
	require.Len(t, endpointResult, 1)
	require.Equal(t, "pod1", endpointResult["container1"].PodName)
}

func TestMigrateStateStoresBoltToJSON(t *testing.T) {
	tempDir := t.TempDir()

	mainJSON := filepath.Join(tempDir, "azure-cns.json")
	mainBolt := filepath.Join(tempDir, "azure-cns.db")
	endpointJSON := filepath.Join(tempDir, "azure-endpoints.json")
	endpointBolt := filepath.Join(tempDir, "azure-endpoints.db")

	mainLock := processlock.NewMockFileLock(false)
	endpointLock := processlock.NewMockFileLock(false)

	boltMain, err := store.NewBoltDBStore(mainBolt, mainLock, nil)
	require.NoError(t, err)
	boltEndpoint, err := store.NewBoltDBStore(endpointBolt, endpointLock, nil)
	require.NoError(t, err)

	require.NoError(t, boltMain.Write(restserver.StoreKey, testState{Value: "main"}))
	endpointState := map[string]*restserver.EndpointInfo{
		"container1": {
			PodName:      "pod1",
			PodNamespace: "default",
		},
	}
	require.NoError(t, boltEndpoint.Write(restserver.EndpointStoreKey, endpointState))

	require.NoError(t, migrateStateStores(
		configuration.StateStoreMigrationModeBoltToJSON,
		boltMain,
		mainJSON,
		mainLock,
		boltEndpoint,
		endpointJSON,
		endpointLock,
		true,
	))

	jsonMain, err := store.NewJsonFileStore(mainJSON, mainLock, nil)
	require.NoError(t, err)
	jsonEndpoint, err := store.NewJsonFileStore(endpointJSON, endpointLock, nil)
	require.NoError(t, err)

	var mainResult testState
	require.NoError(t, jsonMain.Read(restserver.StoreKey, &mainResult))
	require.Equal(t, "main", mainResult.Value)

	var endpointResult map[string]*restserver.EndpointInfo
	require.NoError(t, jsonEndpoint.Read(restserver.EndpointStoreKey, &endpointResult))
	require.Len(t, endpointResult, 1)
	require.Equal(t, "pod1", endpointResult["container1"].PodName)
}
