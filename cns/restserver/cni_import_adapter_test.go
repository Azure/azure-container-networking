// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package restserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errCNIImportProjection = errors.New("projection failure")
	errCNIImportStatus     = errors.New("status failure")
	errCNIQuery            = errors.New("CNI query failure")
)

const (
	cniImportContainerA  = "container-a"
	cniImportPodA        = "pod-a"
	cniImportNamespaceA  = "namespace-a"
	cniImportSecondaryIF = "eth1"
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
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, fmt.Errorf("reading CNI endpoint state: %w", contextErr)
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
	assert.Equal(t, adapterTestPodKeyA, snapshot.IPOwners[adapterTestPrimaryIPID])
	assert.Equal(t, "if-b", snapshot.IPOwners["33333333-3333-3333-3333-333333333333"])
	assert.Equal(t, []string{
		adapterTestPrimaryIPID,
		adapterTestIPv6ID,
	}, snapshot.Assignments[adapterTestPodKeyA].IPIDs)
	assert.Equal(t, adapterTestIPv4, service.EndpointState[cniImportContainerA].IfnameToIPMap[InfraInterfaceName].IPv4[0].IP.String())
	assert.Equal(t, "2001:db8::4", service.EndpointState[cniImportContainerA].IfnameToIPMap[InfraInterfaceName].IPv6[0].IP.String())
	assert.Equal(t, []string{
		adapterTestPrimaryIPID,
		adapterTestIPv6ID,
	}, service.PodIPIDByPodInterfaceKey[adapterTestPodKeyA])
	assert.Equal(t, adapterTestNCID, snapshot.IPs[adapterTestPrimaryIPID].NCID)
	assert.Contains(t, snapshot.Networks, "network-1")

	_, err = provider(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, providerCalls)
}

func TestDurableStateLifecycleImportsCNIBeforeSelection(t *testing.T) {
	t.Run("success selects unified state and leaves provider callable", func(t *testing.T) {
		db := openAdapterImportDB(t)
		seedAdapterImportInventory(t, db)
		service := newAdapterTestService()
		providerCalls := 0
		provider := cns.CNIEndpointStateProvider(func(ctx context.Context) ([]cns.CNIEndpointState, error) {
			providerCalls++
			if contextErr := ctx.Err(); contextErr != nil {
				return nil, fmt.Errorf("reading CNI endpoint state: %w", contextErr)
			}
			return adapterImportRecords(t), nil
		})
		restore, closeState, err := NewDurableStateLifecycleWithCNIImport(service, db, true, provider)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, closeState()) })

		require.Nil(t, service.selectedUnifiedStateAdapter())
		require.NoError(t, restore(context.Background()))
		require.NotNil(t, service.selectedUnifiedStateAdapter())
		assert.Equal(t, 1, providerCalls)
		assert.Contains(t, service.EndpointState, "container-a")

		_, err = provider(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 2, providerCalls)
	})

	t.Run("provider failure blocks unified selection", func(t *testing.T) {
		db := openAdapterImportDB(t)
		seedAdapterImportInventory(t, db)
		service := newAdapterTestService()
		before, err := cloneJSON(service.EndpointState)
		require.NoError(t, err)
		restore, closeState, err := NewDurableStateLifecycleWithCNIImport(
			service,
			db,
			true,
			func(context.Context) ([]cns.CNIEndpointState, error) {
				return nil, errCNIQuery
			},
		)
		require.NoError(t, err)
		err = restore(context.Background())
		require.ErrorIs(t, err, errCNIQuery)
		assert.Nil(t, service.selectedUnifiedStateAdapter())
		assert.Equal(t, before, service.EndpointState)
		require.NoError(t, closeState())
	})

	t.Run("preflight failure blocks unified selection without cache projection", func(t *testing.T) {
		db := openAdapterImportDB(t)
		seedAdapterImportInventory(t, db)
		service := newAdapterTestService()
		before, err := cloneJSON(service.EndpointState)
		require.NoError(t, err)
		records := adapterImportRecords(t)
		records[1].InterfaceKey = records[0].InterfaceKey
		restore, closeState, err := NewDurableStateLifecycleWithCNIImport(
			service,
			db,
			true,
			func(context.Context) ([]cns.CNIEndpointState, error) {
				return records, nil
			},
		)
		require.NoError(t, err)
		require.Error(t, restore(context.Background()))
		assert.Nil(t, service.selectedUnifiedStateAdapter())
		assert.Equal(t, before, service.EndpointState)
		require.NoError(t, closeState())
	})
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
		adapter.buildProjection = func(state.Snapshot) (durableCacheProjection, error) {
			return durableCacheProjection{}, errCNIImportProjection
		}

		err = adapter.importCNIEndpointState(context.Background(), records, plan)
		require.ErrorIs(t, err, errCNIImportProjection)
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
		adapter.store.status = func(context.Context) (state.Status, error) {
			return state.Status{}, errCNIImportStatus
		}

		err = adapter.importCNIEndpointState(context.Background(), records, plan)
		require.ErrorIs(t, err, errCNIImportStatus)
		assert.Contains(t, service.EndpointState, cniImportContainerA)
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
	for _, ncID := range []string{adapterTestNCID, adapterTestNCID2} {
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
			InfraContainerID: cniImportContainerA,
			PodEndpointID:    adapterTestPodKeyA,
			PodName:          cniImportPodA,
			PodNamespace:     cniImportNamespaceA,
			InterfaceKey:     "container-a-eth0",
			InterfaceName:    InfraInterfaceName,
			IPAddresses: []net.IPNet{
				testIPNet("10.0.0.4/24"),
				testIPNet("2001:db8::4/64"),
			},
		},
		{
			InfraContainerID: cniImportContainerA,
			PodEndpointID:    "if-b",
			PodName:          cniImportPodA,
			PodNamespace:     cniImportNamespaceA,
			InterfaceKey:     "container-a-eth1",
			InterfaceName:    cniImportSecondaryIF,
			IPAddresses:      []net.IPNet{testIPNet("10.0.1.4/32")},
		},
	}
}
