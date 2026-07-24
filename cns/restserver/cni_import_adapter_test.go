// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package restserver

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCNIEndpointImportLifecycleKeepsStatefulProviderAvailable(t *testing.T) {
	db := openAdapterImportDB(t)
	seedAdapterImportInventory(t, db)
	service := newAdapterTestService()
	restore, preflight, importState, closeState, err := NewCNIEndpointImportLifecycle(service, db)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, closeState()) })
	require.NoError(t, restore(context.Background()))

	providerCalls := 0
	var provider cns.CNIEndpointStateProvider = func(ctx context.Context) ([]cns.CNIEndpointState, error) {
		providerCalls++
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return adapterImportRecords(t), nil
	}
	records, err := provider(context.Background())
	require.NoError(t, err)
	plan, err := preflight(context.Background(), records)
	require.NoError(t, err)
	require.NoError(t, importState(context.Background(), records, plan))

	snapshot, err := db.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "if-a", snapshot.IPOwners["11111111-1111-1111-1111-111111111111"])
	assert.Equal(t, "if-b", snapshot.IPOwners["33333333-3333-3333-3333-333333333333"])
	assert.Equal(t, []string{
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
	}, snapshot.Assignments["if-a"].IPIDs)
	assert.Equal(t, "10.0.0.4", service.EndpointState["container-a"].IfnameToIPMap["eth0"].IPv4[0].IP.String())
	assert.Equal(t, "2001:db8::4", service.EndpointState["container-a"].IfnameToIPMap["eth0"].IPv6[0].IP.String())
	assert.Equal(t, []string{
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
	}, service.PodIPIDByPodInterfaceKey["if-a"])
	assert.Equal(t, "nc-1", snapshot.IPs["11111111-1111-1111-1111-111111111111"].NCID)
	assert.Contains(t, snapshot.Networks, "network-1")

	_, err = provider(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, providerCalls)
}

func TestCNIEndpointImportAdapterProjectionFailures(t *testing.T) {
	t.Run("prebuild failure does not commit", func(t *testing.T) {
		db := openAdapterImportDB(t)
		seedAdapterImportInventory(t, db)
		service := newAdapterTestService()
		adapter, err := newDurableStateAdapter(service, db, true)
		require.NoError(t, err)
		require.NoError(t, adapter.restore(context.Background()))
		records := adapterImportRecords(t)
		plan, err := adapter.preflightCNIEndpointImport(context.Background(), records)
		require.NoError(t, err)
		beforeSnapshot, err := db.Snapshot(context.Background())
		require.NoError(t, err)
		beforeCache := durableCacheFingerprint(service, adapter)
		injected := errors.New("projection failure")
		adapter.buildProjection = func(state.Snapshot) (durableCacheProjection, error) {
			return durableCacheProjection{}, injected
		}

		err = adapter.importCNIEndpointState(context.Background(), records, plan)
		require.ErrorIs(t, err, injected)
		afterSnapshot, snapshotErr := db.Snapshot(context.Background())
		require.NoError(t, snapshotErr)
		assert.Equal(t, beforeSnapshot, afterSnapshot)
		assert.Equal(t, beforeCache, durableCacheFingerprint(service, adapter))
		require.NoError(t, adapter.Close())
	})

	t.Run("status failure restores committed projection", func(t *testing.T) {
		db := openAdapterImportDB(t)
		seedAdapterImportInventory(t, db)
		service := newAdapterTestService()
		adapter, err := newDurableStateAdapter(service, db, true)
		require.NoError(t, err)
		require.NoError(t, adapter.restore(context.Background()))
		records := adapterImportRecords(t)
		plan, err := adapter.preflightCNIEndpointImport(context.Background(), records)
		require.NoError(t, err)
		originalStatus := adapter.store.status
		injected := errors.New("status failure")
		adapter.store.status = func(context.Context) (state.Status, error) {
			return state.Status{}, injected
		}

		err = adapter.importCNIEndpointState(context.Background(), records, plan)
		require.ErrorIs(t, err, injected)
		assert.Contains(t, service.EndpointState, "container-a")
		snapshot, snapshotErr := db.Snapshot(context.Background())
		require.NoError(t, snapshotErr)
		generation, projected := adapter.cacheGeneration()
		assert.True(t, projected)
		assert.Equal(t, snapshot.Metadata.Generation, generation)
		adapter.store.status = originalStatus
		require.NoError(t, adapter.Close())
	})
}

func openAdapterImportDB(t *testing.T) *state.DB {
	t.Helper()
	db, err := state.Open(filepath.Join(t.TempDir(), "state.db"), state.Options{})
	require.NoError(t, err)
	return db
}

func seedAdapterImportInventory(t *testing.T, db *state.DB) {
	t.Helper()
	snapshot := completeAdapterSnapshot(0)
	for _, ncID := range []string{"nc-1", "nc-2"} {
		ips := make([]state.IPRecord, 0)
		for _, ip := range snapshot.IPs {
			if ip.NCID == ncID {
				ips = append(ips, ip)
			}
		}
		_, err := db.ApplyNetworkContainer(context.Background(), snapshot.NetworkContainers[ncID], ips)
		require.NoError(t, err)
	}
	_, err := db.ReplaceDurableState(context.Background(), 2, state.DurableState{
		NetworkContainers:    snapshot.NetworkContainers,
		IPs:                  snapshot.IPs,
		Networks:             snapshot.Networks,
		OrchestratorContexts: snapshot.OrchestratorContexts,
		PnPIDByMAC:           snapshot.PnPIDByMAC,
	})
	require.NoError(t, err)
}

func adapterImportRecords(t *testing.T) []cns.CNIEndpointState {
	t.Helper()
	return []cns.CNIEndpointState{
		{
			InfraContainerID: "container-a",
			PodEndpointID:    "if-a",
			PodName:          "pod-a",
			PodNamespace:     "namespace-a",
			InterfaceKey:     "container-a-eth0",
			InterfaceName:    "eth0",
			IPAddresses: []net.IPNet{
				testIPNet("10.0.0.4/24"),
				testIPNet("2001:db8::4/64"),
			},
		},
		{
			InfraContainerID: "container-a",
			PodEndpointID:    "if-b",
			PodName:          "pod-a",
			PodNamespace:     "namespace-a",
			InterfaceKey:     "container-a-eth1",
			InterfaceName:    "eth1",
			IPAddresses:      []net.IPNet{testIPNet("10.0.1.4/32")},
		},
	}
}
