// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/wireserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
	bolterrors "go.etcd.io/bbolt/errors"
)

const (
	testMACAddress       = "00:11:22:33:44:55"
	testInvalidIPAddress = "not-an-ip"
	testAuthToken        = "secret"
	testIPID2            = "ip-2"
	testPnPID            = "pnp-1"
)

func TestDurableTransactions(t *testing.T) {
	db, _ := openTestDB(t)
	nc := testNetworkContainer(testNCID)
	ip := IPRecord{ID: testIPID1, IPAddress: testIPv4Address, NCID: testNCID, NCVersion: 1}
	network := NetworkRecord{NetworkName: testNetworkName}

	t.Run("put get and list", func(t *testing.T) {
		require.NoError(t, db.Update(context.Background(), func(tx *WriteTx) error {
			if err := tx.PutNetworkContainer(nc); err != nil {
				return err
			}
			if err := tx.PutIP(ip); err != nil {
				return err
			}
			if err := tx.PutNetwork(network); err != nil {
				return err
			}
			if err := tx.PutOrchestratorContext("context-1", []string{testNCID}); err != nil {
				return err
			}
			return tx.PutPnPID("00-11-22-33-44-55", testPnPID)
		}))

		require.NoError(t, db.View(context.Background(), func(tx *ReadTx) error {
			gotNC, err := tx.GetNetworkContainer(testNCID)
			require.NoError(t, err)
			assert.Equal(t, nc, gotNC)
			gotIP, err := tx.GetIP(testIPID1)
			require.NoError(t, err)
			assert.Equal(t, ip, gotIP)
			gotNetwork, err := tx.GetNetwork(testNetworkName)
			require.NoError(t, err)
			assert.Equal(t, network, gotNetwork)
			contextNCs, err := tx.GetOrchestratorContext("context-1")
			require.NoError(t, err)
			assert.Equal(t, []string{testNCID}, contextNCs)
			contextNCs[0] = "mutated"
			pnpID, err := tx.GetPnPID(testMACAddress)
			require.NoError(t, err)
			assert.Equal(t, testPnPID, pnpID)

			ncs, err := tx.ListNetworkContainers()
			require.NoError(t, err)
			ips, err := tx.ListIPs()
			require.NoError(t, err)
			networks, err := tx.ListNetworks()
			require.NoError(t, err)
			contexts, err := tx.ListOrchestratorContexts()
			require.NoError(t, err)
			pnpIDs, err := tx.ListPnPIDs()
			require.NoError(t, err)
			assert.Equal(t, map[string]NetworkContainerRecord{testNCID: nc}, ncs)
			assert.Equal(t, map[string]IPRecord{testIPID1: ip}, ips)
			assert.Equal(t, map[string]NetworkRecord{testNetworkName: network}, networks)
			assert.Equal(t, map[string][]string{"context-1": {testNCID}}, contexts)
			assert.Equal(t, map[string]string{testMACAddress: testPnPID}, pnpIDs)
			return nil
		}))
	})

	t.Run("delete and not found", func(t *testing.T) {
		require.NoError(t, db.Update(context.Background(), func(tx *WriteTx) error {
			if err := tx.DeleteOrchestratorContext("context-1"); err != nil {
				return err
			}
			if err := tx.DeletePnPID(testMACAddress); err != nil {
				return err
			}
			if err := tx.DeleteNetwork(testNetworkName); err != nil {
				return err
			}
			if err := tx.DeleteIP(testIPID1); err != nil {
				return err
			}
			return tx.DeleteNetworkContainer(testNCID)
		}))

		require.NoError(t, db.View(context.Background(), func(tx *ReadTx) error {
			_, err := tx.GetNetworkContainer(testNCID)
			require.ErrorIs(t, err, ErrNotFound)
			_, err = tx.GetIP(testIPID1)
			require.ErrorIs(t, err, ErrNotFound)
			_, err = tx.GetNetwork(testNetworkName)
			require.ErrorIs(t, err, ErrNotFound)
			_, err = tx.GetOrchestratorContext("context-1")
			require.ErrorIs(t, err, ErrNotFound)
			_, err = tx.GetPnPID(testMACAddress)
			require.ErrorIs(t, err, ErrNotFound)
			return nil
		}))

		before := readMetadata(t, db).Generation
		err := db.Update(context.Background(), func(tx *WriteTx) error {
			return tx.DeleteNetworkContainer(testNCID)
		})
		require.ErrorIs(t, err, ErrNotFound)
		assert.Equal(t, before, readMetadata(t, db).Generation)
	})
}

func TestClearBucketAfterMaterializingWritableNode(t *testing.T) {
	db, _ := openTestDB(t)
	require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketDeleteIntents)
		for index := range 200 {
			key := []byte(fmt.Sprintf("intent-%03d", index))
			if err := bucket.Put(key, []byte(`{"createdAt":"2026-08-28T00:00:00Z"}`)); err != nil {
				return fmt.Errorf("putting key %q: %w", key, err)
			}
		}
		if err := bucket.Put([]byte("intent-000"), []byte(`{"createdAt":"2026-08-28T01:00:00Z"}`)); err != nil {
			return fmt.Errorf("putting key %q: %w", "intent-000", err)
		}
		return clearBucket(bucket)
	}))

	require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketDeleteIntents)
		key, _ := bucket.Cursor().First()
		assert.Nil(t, key)
		return nil
	}))
}

func TestDurableTransactionValidationRollsBack(t *testing.T) {
	db, _ := openTestDB(t)
	before, err := db.Snapshot(context.Background())
	require.NoError(t, err)

	err = db.Update(context.Background(), func(tx *WriteTx) error {
		if perr := tx.PutNetworkContainer(testNetworkContainer(testNCID)); perr != nil {
			return perr
		}
		if perr := tx.PutIP(IPRecord{
			ID:        testIPID1,
			IPAddress: testInvalidIPAddress,
			NCID:      testNCID,
		}); perr != nil {
			return perr
		}
		return tx.PutNetwork(NetworkRecord{
			NetworkName: testNetworkName,
			Options:     map[string]any{"unsupported": make(chan struct{})},
		})
	})

	require.ErrorIs(t, err, ErrInvalidInput)
	after, snapshotErr := db.Snapshot(context.Background())
	require.NoError(t, snapshotErr)
	assert.Equal(t, before, after)
}

func TestApplyNetworkContainer(t *testing.T) {
	t.Run("exact replacement and normalization", func(t *testing.T) {
		db, _ := openTestDB(t)
		record := testNetworkContainer(testNCID)
		record.Request.AuthorizationToken = testAuthToken
		record.Request.SecondaryIPConfigs = map[string]cns.SecondaryIPConfig{"embedded": {}}
		first := []IPRecord{
			{ID: testIPID1, IPAddress: testIPv4Address, NCID: testNCID, NCVersion: 1},
			{ID: testIPID2, IPAddress: "fd00::4", NCID: testNCID, NCVersion: 1},
		}

		changed, err := db.ApplyNetworkContainer(context.Background(), record, first)
		require.NoError(t, err)
		assert.True(t, changed)
		replacement := []IPRecord{
			{ID: testIPID2, IPAddress: "fd00::5", NCID: testNCID, NCVersion: 2},
		}
		changed, err = db.ApplyNetworkContainer(context.Background(), record, replacement)
		require.NoError(t, err)
		assert.True(t, changed)

		snapshot, err := db.Snapshot(context.Background())
		require.NoError(t, err)
		assert.Equal(t, map[string]IPRecord{testIPID2: replacement[0]}, snapshot.IPs)
		stored := snapshot.NetworkContainers[testNCID]
		assert.Empty(t, stored.Request.AuthorizationToken)
		assert.Nil(t, stored.Request.SecondaryIPConfigs)
		assert.Equal(t, uint64(2), snapshot.Metadata.Generation)
	})

	t.Run("full input preflight", func(t *testing.T) {
		db, _ := openTestDB(t)
		record := testNetworkContainer(testNCID)
		_, err := db.ApplyNetworkContainer(context.Background(), record, []IPRecord{{
			ID:        testIPID1,
			IPAddress: testIPv4Address,
			NCID:      testNCID,
		}})
		require.NoError(t, err)
		before, err := db.Snapshot(context.Background())
		require.NoError(t, err)

		_, err = db.ApplyNetworkContainer(context.Background(), record, []IPRecord{
			{ID: testIPID2, IPAddress: "10.0.0.5", NCID: testNCID},
			{ID: "ip-3", IPAddress: "::ffff:10.0.0.5", NCID: testNCID},
		})
		require.ErrorIs(t, err, ErrInvalidInput)
		after, snapshotErr := db.Snapshot(context.Background())
		require.NoError(t, snapshotErr)
		assert.Equal(t, before, after)
	})

	t.Run("global duplicate address", func(t *testing.T) {
		db, _ := openTestDB(t)
		_, err := db.ApplyNetworkContainer(context.Background(), testNetworkContainer(testNCID), []IPRecord{{
			ID: testIPID1, IPAddress: testIPv4Address, NCID: testNCID,
		}})
		require.NoError(t, err)
		_, err = db.ApplyNetworkContainer(context.Background(), testNetworkContainer("nc-2"), []IPRecord{{
			ID: testIPID2, IPAddress: "::ffff:10.0.0.4", NCID: "nc-2",
		}})
		require.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("session reference protection", func(t *testing.T) {
		db, _ := openTestDB(t)
		snapshot := endpointOnlySnapshot()
		writeSnapshot(t, db, snapshot)
		before, err := db.Snapshot(context.Background())
		require.NoError(t, err)

		_, err = db.ApplyNetworkContainer(
			context.Background(),
			snapshot.NetworkContainers[testNCID],
			nil,
		)
		require.ErrorIs(t, err, ErrInvalidInput)
		after, snapshotErr := db.Snapshot(context.Background())
		require.NoError(t, snapshotErr)
		assert.Equal(t, before, after)
	})
}

func TestDeleteNetworkContainer(t *testing.T) {
	t.Run("existing and missing are explicit", func(t *testing.T) {
		db, _ := openTestDB(t)
		_, err := db.ApplyNetworkContainer(context.Background(), testNetworkContainer(testNCID), []IPRecord{{
			ID: testIPID1, IPAddress: testIPv4Address, NCID: testNCID,
		}})
		require.NoError(t, err)

		changed, err := db.DeleteNetworkContainer(context.Background(), testNCID)
		require.NoError(t, err)
		assert.True(t, changed)
		changed, err = db.DeleteNetworkContainer(context.Background(), testNCID)
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, uint64(2), readMetadata(t, db).Generation)
	})

	t.Run("references reject atomically", func(t *testing.T) {
		db, _ := openTestDB(t)
		writeSnapshot(t, db, completeSnapshot())
		before, err := db.Snapshot(context.Background())
		require.NoError(t, err)

		_, err = db.DeleteNetworkContainer(context.Background(), testNCID)
		require.ErrorIs(t, err, ErrInvalidInput)
		after, snapshotErr := db.Snapshot(context.Background())
		require.NoError(t, snapshotErr)
		assert.Equal(t, before, after)
	})
}

func TestReplaceDurableState(t *testing.T) {
	db, _ := openTestDB(t)
	initial := completeSnapshot()
	writeSnapshot(t, db, initial)
	durable := durableFromSnapshot(initial)
	durable.Networks["network-2"] = NetworkRecord{
		NetworkName: "network-2",
		NicInfo:     &wireserver.InterfaceInfo{Subnet: "10.2.0.0/24"},
	}
	durable.PnPIDByMAC = map[string]string{"00-11-22-33-44-55": "pnp-replaced"}

	changed, err := db.ReplaceDurableState(context.Background(), 0, durable)
	require.NoError(t, err)
	assert.True(t, changed)
	snapshot, err := db.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, durable.NetworkContainers, snapshot.NetworkContainers)
	assert.Equal(t, durable.IPs, snapshot.IPs)
	assert.Equal(t, durable.Networks, snapshot.Networks)
	assert.Equal(t, durable.OrchestratorContexts, snapshot.OrchestratorContexts)
	assert.Equal(t, map[string]string{testMACAddress: "pnp-replaced"}, snapshot.PnPIDByMAC)
	assert.Equal(t, initial.Assignments, snapshot.Assignments)
	assert.Equal(t, initial.IPOwners, snapshot.IPOwners)
	assert.Equal(t, initial.Endpoints, snapshot.Endpoints)
	assert.Equal(t, initial.DeleteIntents, snapshot.DeleteIntents)
	assert.Equal(t, uint64(1), snapshot.Metadata.Generation)

	t.Run("stale generation", func(t *testing.T) {
		before := snapshot
		_, err := db.ReplaceDurableState(context.Background(), 0, durable)
		require.ErrorIs(t, err, ErrStaleGeneration)
		after, snapshotErr := db.Snapshot(context.Background())
		require.NoError(t, snapshotErr)
		assert.Equal(t, before, after)
	})

	t.Run("invalid candidate and encoding rollback", func(t *testing.T) {
		invalid := durable
		invalid.Networks = map[string]NetworkRecord{
			"bad": {
				NetworkName: "bad",
				Options:     map[string]any{"unsupported": make(chan struct{})},
			},
		}
		before, snapshotErr := db.Snapshot(context.Background())
		require.NoError(t, snapshotErr)
		_, err := db.ReplaceDurableState(context.Background(), before.Metadata.Generation, invalid)
		require.ErrorIs(t, err, ErrInvalidInput)
		after, snapshotErr := db.Snapshot(context.Background())
		require.NoError(t, snapshotErr)
		assert.Equal(t, before, after)
	})

	t.Run("session endpoint reference preservation", func(t *testing.T) {
		replacement := durable
		replacement.IPs = map[string]IPRecord{}
		before, snapshotErr := db.Snapshot(context.Background())
		require.NoError(t, snapshotErr)
		_, err := db.ReplaceDurableState(context.Background(), before.Metadata.Generation, replacement)
		require.ErrorIs(t, err, ErrInvalidInput)
		after, snapshotErr := db.Snapshot(context.Background())
		require.NoError(t, snapshotErr)
		assert.Equal(t, before, after)
	})
}

func TestNetworkAndMappingValidation(t *testing.T) {
	db, _ := openTestDB(t)
	require.NoError(t, db.Update(context.Background(), func(tx *WriteTx) error {
		return tx.PutNetworkContainer(testNetworkContainer(testNCID))
	}))
	before := readMetadata(t, db).Generation

	tests := []struct {
		name string
		run  func(*WriteTx) error
	}{
		{
			name: "invalid network prefix",
			run: func(tx *WriteTx) error {
				return tx.PutNetwork(NetworkRecord{
					NetworkName: testNetworkName,
					NicInfo:     &wireserver.InterfaceInfo{Subnet: "not-a-prefix"},
				})
			},
		},
		{
			name: "duplicate orchestrator reference",
			run: func(tx *WriteTx) error {
				return tx.PutOrchestratorContext("context-1", []string{testNCID, testNCID})
			},
		},
		{
			name: "missing orchestrator reference",
			run: func(tx *WriteTx) error {
				return tx.PutOrchestratorContext("context-1", []string{testMissingID})
			},
		},
		{
			name: "invalid MAC",
			run: func(tx *WriteTx) error {
				return tx.PutPnPID("not-a-mac", testPnPID)
			},
		},
		{
			name: "empty PnP ID",
			run: func(tx *WriteTx) error {
				return tx.PutPnPID(testMACAddress, "")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.Update(context.Background(), tt.run)
			require.ErrorIs(t, err, ErrInvalidInput)
			assert.Equal(t, before, readMetadata(t, db).Generation)
		})
	}
}

func TestUpdateReadinessObservation(t *testing.T) {
	db, _ := openTestDB(t)
	record := testNetworkContainer(testNCID)
	_, err := db.ApplyNetworkContainer(context.Background(), record, nil)
	require.NoError(t, err)
	observation := ReadinessObservation{
		VMVersion:         "vm-new",
		HostVersion:       "host-new",
		VFPUpdateComplete: true,
	}

	changed, err := db.UpdateReadinessObservation(context.Background(), testNCID, observation)
	require.NoError(t, err)
	assert.True(t, changed)
	snapshot, err := db.Snapshot(context.Background())
	require.NoError(t, err)
	got := snapshot.NetworkContainers[testNCID]
	assert.Equal(t, observation.VMVersion, got.VMVersion)
	assert.Equal(t, observation.HostVersion, got.HostVersion)
	assert.Equal(t, observation.VFPUpdateComplete, got.VFPUpdateComplete)
	assert.Equal(t, record.Request, got.Request)

	changed, err = db.UpdateReadinessObservation(context.Background(), testNCID, observation)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, uint64(2), readMetadata(t, db).Generation)

	_, err = db.UpdateReadinessObservation(context.Background(), testMissingID, observation)
	require.ErrorIs(t, err, ErrNotFound)
	assert.Equal(t, uint64(2), readMetadata(t, db).Generation)
}

func TestApplyBoot(t *testing.T) {
	tests := []struct {
		name            string
		policy          BootPolicy
		wantEndpoints   bool
		wantHostVersion string
		wantVFPComplete bool
	}{
		{
			name:            "clear endpoints and reset readiness",
			policy:          BootPolicy{ClearEndpoints: true, ResetReadiness: true},
			wantEndpoints:   false,
			wantHostVersion: "",
		},
		{
			name:            "retain endpoints and preserve readiness",
			policy:          BootPolicy{},
			wantEndpoints:   true,
			wantHostVersion: "1",
			wantVFPComplete: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openTestDB(t)
			initial := completeSnapshot()
			writeSnapshot(t, db, initial)

			changed, err := db.ApplyBoot(context.Background(), "boot-1", tt.policy)
			require.NoError(t, err)
			assert.True(t, changed)
			got, err := db.Snapshot(context.Background())
			require.NoError(t, err)
			assert.Equal(t, "boot-1", got.Metadata.BootID)
			if tt.wantEndpoints {
				assert.Equal(t, initial.Assignments, got.Assignments)
				assert.Equal(t, initial.IPOwners, got.IPOwners)

				replayChanged, replayErr := db.AssignEndpoint(
					context.Background(),
					initial.Assignments["iface-primary"],
					initial.Endpoints["container-1"],
					testNow,
					testDeleteIntentTTL,
				)
				require.NoError(t, replayErr)
				assert.False(t, replayChanged)
			} else {
				assert.Empty(t, got.Assignments)
				assert.Empty(t, got.IPOwners)
			}
			assert.Empty(t, got.DeleteIntents)
			assert.Equal(t, tt.wantEndpoints, len(got.Endpoints) != 0)
			assert.Equal(t, initial.IPs, got.IPs)
			assert.Equal(t, initial.Networks, got.Networks)
			assert.Equal(t, initial.OrchestratorContexts, got.OrchestratorContexts)
			assert.Equal(t, initial.PnPIDByMAC, got.PnPIDByMAC)
			nc := got.NetworkContainers[testNCID]
			assert.Equal(t, initial.NetworkContainers[testNCID].VMVersion, nc.VMVersion)
			assert.Equal(t, initial.NetworkContainers[testNCID].Request, nc.Request)
			assert.Equal(t, tt.wantHostVersion, nc.HostVersion)
			assert.Equal(t, tt.wantVFPComplete, nc.VFPUpdateComplete)
			assert.Equal(t, uint64(1), got.Metadata.Generation)

			changed, err = db.ApplyBoot(context.Background(), "boot-1", BootPolicy{
				ClearEndpoints: !tt.policy.ClearEndpoints,
				ResetReadiness: !tt.policy.ResetReadiness,
			})
			require.NoError(t, err)
			assert.False(t, changed)
			assert.Equal(t, uint64(1), readMetadata(t, db).Generation)
		})
	}

	t.Run("invalid ID", func(t *testing.T) {
		db, _ := openTestDB(t)
		changed, err := db.ApplyBoot(context.Background(), "", BootPolicy{})
		require.ErrorIs(t, err, ErrInvalidInput)
		assert.False(t, changed)
		assert.Zero(t, readMetadata(t, db).Generation)
	})
}

func TestConcurrentDurableUpdates(t *testing.T) {
	db, _ := openTestDB(t)
	const count = 8
	start := make(chan struct{})
	errs := make(chan error, count)
	var ready sync.WaitGroup
	ready.Add(count)
	for index := range count {
		go func() {
			id := "nc-" + string(rune('a'+index)) //#nosec G115 -- index is bounded by count (max 8), safe conversion
			ready.Done()
			<-start
			_, err := db.ApplyNetworkContainer(context.Background(), testNetworkContainer(id), nil)
			errs <- err
		}()
	}
	ready.Wait()
	close(start)
	for range count {
		require.NoError(t, <-errs)
	}

	snapshot, err := db.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Len(t, snapshot.NetworkContainers, count)
	assert.Equal(t, uint64(count), snapshot.Metadata.Generation)
}

func TestConcurrentSameBootTransition(t *testing.T) {
	db, _ := openTestDB(t)
	writeSnapshot(t, db, completeSnapshot())
	const count = 8
	start := make(chan struct{})
	results := make(chan bool, count)
	errs := make(chan error, count)
	var ready sync.WaitGroup
	ready.Add(count)
	for range count {
		go func() {
			ready.Done()
			<-start
			changed, err := db.ApplyBoot(context.Background(), "boot-1", BootPolicy{ClearEndpoints: true})
			results <- changed
			errs <- err
		}()
	}
	ready.Wait()
	close(start)

	changedCount := 0
	for range count {
		require.NoError(t, <-errs)
		if <-results {
			changedCount++
		}
	}
	assert.Equal(t, 1, changedCount)
	assert.Equal(t, uint64(1), readMetadata(t, db).Generation)
}

func TestDurableOperationsContextAndReadOnly(t *testing.T) {
	t.Run("canceled context", func(t *testing.T) {
		db, _ := openTestDB(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := db.ApplyNetworkContainer(ctx, testNetworkContainer(testNCID), nil)
		require.ErrorIs(t, err, context.Canceled)
		assert.Zero(t, readMetadata(t, db).Generation)
	})

	t.Run("read-only failure", func(t *testing.T) {
		db, path := openTestDB(t)
		require.NoError(t, db.Close())
		readOnly, err := Open(path, Options{ReadOnly: true})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, readOnly.Close()) })

		_, err = readOnly.ApplyNetworkContainer(
			context.Background(),
			testNetworkContainer(testNCID),
			nil,
		)
		require.ErrorIs(t, err, bolterrors.ErrDatabaseReadOnly)
		assert.Zero(t, readMetadata(t, readOnly).Generation)
	})
}

func TestDurableStateReopenRoundTrip(t *testing.T) {
	db, path := openTestDB(t)
	record := testNetworkContainer(testNCID)
	ips := []IPRecord{{ID: testIPID1, IPAddress: testIPv4Address, NCID: testNCID, NCVersion: 3}}
	_, err := db.ApplyNetworkContainer(context.Background(), record, ips)
	require.NoError(t, err)
	require.NoError(t, db.Update(context.Background(), func(tx *WriteTx) error {
		if perr := tx.PutNetwork(NetworkRecord{NetworkName: testNetworkName}); perr != nil {
			return perr
		}
		if perr := tx.PutOrchestratorContext("context-1", []string{testNCID}); perr != nil {
			return perr
		}
		return tx.PutPnPID(testMACAddress, testPnPID)
	}))
	require.NoError(t, db.Close())

	reopened, err := Open(path, Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	snapshot, err := reopened.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, map[string]NetworkContainerRecord{testNCID: record}, snapshot.NetworkContainers)
	assert.Equal(t, map[string]IPRecord{testIPID1: ips[0]}, snapshot.IPs)
	assert.Equal(t, map[string]NetworkRecord{testNetworkName: {NetworkName: testNetworkName}}, snapshot.Networks)
	assert.Equal(t, map[string][]string{"context-1": {testNCID}}, snapshot.OrchestratorContexts)
	assert.Equal(t, map[string]string{testMACAddress: testPnPID}, snapshot.PnPIDByMAC)
	assert.Equal(t, uint64(2), snapshot.Metadata.Generation)
}

func testNetworkContainer(id string) NetworkContainerRecord {
	request := completeNetworkContainerRequest()
	request.NetworkContainerid = id
	return NewNetworkContainerRecord(id, "2", "1", true, request)
}

func durableFromSnapshot(snapshot Snapshot) DurableState {
	return DurableState{
		NetworkContainers:    snapshot.NetworkContainers,
		IPs:                  snapshot.IPs,
		Networks:             snapshot.Networks,
		OrchestratorContexts: snapshot.OrchestratorContexts,
		PnPIDByMAC:           snapshot.PnPIDByMAC,
	}
}

func endpointOnlySnapshot() Snapshot {
	snapshot := NewSnapshot()
	snapshot.NetworkContainers[testNCID] = testNetworkContainer(testNCID)
	snapshot.IPs[testIPID1] = IPRecord{
		ID:        testIPID1,
		IPAddress: testIPv4Address,
		NCID:      testNCID,
	}
	snapshot.Endpoints["container-1"] = completeEndpointRecord()
	return snapshot
}
