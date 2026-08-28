// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
	bolterrors "go.etcd.io/bbolt/errors"
)

const testDeleteIntentTTL = 10 * time.Minute

var testNow = time.Date(2026, time.July, 24, 0, 0, 0, 0, time.UTC)

func TestOwnershipTransactions(t *testing.T) {
	db, _ := openTestDB(t)
	assignment, endpoint := seedOwnershipInventory(t, db)
	intent := DeleteIntent{CreatedAt: testNow}

	t.Run("put get and list exact keys", func(t *testing.T) {
		require.NoError(t, db.Update(context.Background(), func(tx *WriteTx) error {
			if err := tx.PutEndpoint(assignment.Pod.InfraContainerID, endpoint); err != nil {
				return err
			}
			if err := tx.PutAssignment(assignment); err != nil {
				return err
			}
			for _, ipID := range assignment.IPIDs {
				if err := tx.PutIPOwner(ipID, assignment.Pod.PodKey); err != nil {
					return err
				}
			}
			return tx.PutDeleteIntent("old-container", intent)
		}))

		require.NoError(t, db.View(context.Background(), func(tx *ReadTx) error {
			gotAssignment, err := tx.GetAssignment(assignment.Pod.PodKey)
			require.NoError(t, err)
			assert.Equal(t, assignment, gotAssignment)
			gotOwner, err := tx.GetIPOwner("ip-v4")
			require.NoError(t, err)
			assert.Equal(t, assignment.Pod.PodKey, gotOwner)
			gotEndpoint, err := tx.GetEndpoint(assignment.Pod.InfraContainerID)
			require.NoError(t, err)
			equal, err := endpointsEqual(endpoint, gotEndpoint)
			require.NoError(t, err)
			assert.True(t, equal)
			gotIntent, err := tx.GetDeleteIntent("old-container")
			require.NoError(t, err)
			assert.Equal(t, intent, gotIntent)

			assignments, err := tx.ListAssignments()
			require.NoError(t, err)
			owners, err := tx.ListIPOwners()
			require.NoError(t, err)
			endpoints, err := tx.ListEndpoints()
			require.NoError(t, err)
			intents, err := tx.ListDeleteIntents()
			require.NoError(t, err)
			assert.Equal(t, map[string]AssignmentRecord{assignment.Pod.PodKey: assignment}, assignments)
			assert.Equal(t, ownersFromAssignment(assignment), owners)
			assert.Contains(t, endpoints, assignment.Pod.InfraContainerID)
			assert.Equal(t, map[string]DeleteIntent{"old-container": intent}, intents)
			return nil
		}))
	})

	t.Run("delete and not found", func(t *testing.T) {
		require.NoError(t, db.Update(context.Background(), func(tx *WriteTx) error {
			for _, ipID := range assignment.IPIDs {
				if err := tx.DeleteIPOwner(ipID); err != nil {
					return err
				}
			}
			if err := tx.DeleteAssignment(assignment.Pod.PodKey); err != nil {
				return err
			}
			if err := tx.DeleteEndpoint(assignment.Pod.InfraContainerID); err != nil {
				return err
			}
			return tx.DeleteDeleteIntent("old-container")
		}))
		require.NoError(t, db.View(context.Background(), func(tx *ReadTx) error {
			_, err := tx.GetAssignment(assignment.Pod.PodKey)
			require.ErrorIs(t, err, ErrNotFound)
			_, err = tx.GetIPOwner("ip-v4")
			require.ErrorIs(t, err, ErrNotFound)
			_, err = tx.GetEndpoint(assignment.Pod.InfraContainerID)
			require.ErrorIs(t, err, ErrNotFound)
			_, err = tx.GetDeleteIntent("old-container")
			require.ErrorIs(t, err, ErrNotFound)
			return nil
		}))
		for _, run := range []func(*WriteTx) error{
			func(tx *WriteTx) error { return tx.DeleteAssignment(assignment.Pod.PodKey) },
			func(tx *WriteTx) error { return tx.DeleteIPOwner("ip-v4") },
			func(tx *WriteTx) error { return tx.DeleteEndpoint(assignment.Pod.InfraContainerID) },
			func(tx *WriteTx) error { return tx.DeleteDeleteIntent("old-container") },
		} {
			err := db.Update(context.Background(), run)
			require.ErrorIs(t, err, ErrNotFound)
		}
	})
}

func TestOwnershipTransactionsReadOnly(t *testing.T) {
	db, path := openTestDB(t)
	assignment, endpoint := seedOwnershipInventory(t, db)
	require.NoError(t, db.Close())
	readOnly, err := Open(path, Options{ReadOnly: true})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, readOnly.Close()) })
	tx, err := readOnly.db.Begin(false)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, tx.Rollback()) })
	writeTx := &WriteTx{ReadTx: ReadTx{tx: tx, ctx: context.Background()}}

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "assignment", run: func() error { return writeTx.PutAssignment(assignment) }},
		{name: "IP owner", run: func() error { return writeTx.PutIPOwner("ip-v4", assignment.Pod.PodKey) }},
		{name: "endpoint", run: func() error {
			return writeTx.PutEndpoint(assignment.Pod.InfraContainerID, endpoint)
		}},
		{name: "delete intent", run: func() error {
			return writeTx.PutDeleteIntent("old-container", DeleteIntent{CreatedAt: testNow})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorIs(t, tt.run(), bolterrors.ErrTxNotWritable)
		})
	}
}

func TestAssignEndpoint(t *testing.T) {
	t.Run("dual stack multi-NIC exact snapshot and idempotence", func(t *testing.T) {
		db, _ := openTestDB(t)
		assignment, endpoint := seedOwnershipInventory(t, db)
		before := readMetadata(t, db).Generation

		changed, err := db.AssignEndpoint(
			context.Background(),
			assignment,
			endpoint,
			testNow,
			testDeleteIntentTTL,
		)
		require.NoError(t, err)
		assert.True(t, changed)
		snapshot := requireValidSnapshot(t, db)
		assert.Equal(t, map[string]AssignmentRecord{assignment.Pod.PodKey: assignment}, snapshot.Assignments)
		assert.Equal(t, ownersFromAssignment(assignment), snapshot.IPOwners)
		assert.Contains(t, snapshot.Endpoints, assignment.Pod.InfraContainerID)
		assert.Equal(t, before+1, snapshot.Metadata.Generation)

		changed, err = db.AssignEndpoint(
			context.Background(),
			assignment,
			endpoint,
			testNow,
			testDeleteIntentTTL,
		)
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, before+1, readMetadata(t, db).Generation)

		changedAssignment := assignment
		changedAssignment.IPIDs = []string{"ip-v4"}
		_, err = db.AssignEndpoint(
			context.Background(),
			changedAssignment,
			endpoint,
			testNow,
			testDeleteIntentTTL,
		)
		require.ErrorIs(t, err, ErrInvalidInput)
		assert.Equal(t, before+1, readMetadata(t, db).Generation)
	})

	t.Run("single IP", func(t *testing.T) {
		db, _ := openTestDB(t)
		ip := IPRecord{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-1"}
		_, err := db.ApplyNetworkContainer(
			context.Background(),
			testNetworkContainer("nc-1"),
			[]IPRecord{ip},
		)
		require.NoError(t, err)
		assignment := testAssignment("container-1", "pod-1", "ns-1", "ip-1")
		endpoint := testEndpoint("pod-1", "ns-1", "10.0.0.4", "nc-1")
		changed, err := db.AssignEndpoint(
			context.Background(),
			assignment,
			endpoint,
			testNow,
			testDeleteIntentTTL,
		)
		require.NoError(t, err)
		assert.True(t, changed)
		requireValidSnapshot(t, db)
	})

	t.Run("missing and duplicate IP IDs", func(t *testing.T) {
		db, _ := openTestDB(t)
		assignment, endpoint := seedOwnershipInventory(t, db)
		duplicate := assignment
		duplicate.IPIDs = []string{"ip-v4", "ip-v4"}
		_, err := db.AssignEndpoint(
			context.Background(),
			duplicate,
			endpoint,
			testNow,
			testDeleteIntentTTL,
		)
		require.ErrorIs(t, err, ErrInvalidInput)
		missing := assignment
		missing.IPIDs = []string{"missing"}
		_, err = db.AssignEndpoint(
			context.Background(),
			missing,
			endpoint,
			testNow,
			testDeleteIntentTTL,
		)
		require.ErrorIs(t, err, ErrInvalidInput)
		assert.Empty(t, requireValidSnapshot(t, db).Assignments)
	})
}

func TestConcurrentDuplicateOwnership(t *testing.T) {
	db, _ := openTestDB(t)
	ip := IPRecord{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-1"}
	_, err := db.ApplyNetworkContainer(
		context.Background(),
		testNetworkContainer("nc-1"),
		[]IPRecord{ip},
	)
	require.NoError(t, err)
	requests := []struct {
		assignment AssignmentRecord
		endpoint   EndpointRecord
	}{
		{
			assignment: testAssignment("container-1", "pod-1", "ns-1", "ip-1"),
			endpoint:   testEndpoint("pod-1", "ns-1", "10.0.0.4", "nc-1"),
		},
		{
			assignment: testAssignment("container-2", "pod-2", "ns-1", "ip-1"),
			endpoint:   testEndpoint("pod-2", "ns-1", "10.0.0.4", "nc-1"),
		},
	}
	start := make(chan struct{})
	errs := make(chan error, len(requests))
	var ready sync.WaitGroup
	ready.Add(len(requests))
	for _, request := range requests {
		go func() {
			ready.Done()
			<-start
			_, err := db.AssignEndpoint(
				context.Background(),
				request.assignment,
				request.endpoint,
				testNow,
				testDeleteIntentTTL,
			)
			errs <- err
		}()
	}
	ready.Wait()
	close(start)

	successes := 0
	conflicts := 0
	for range requests {
		err := <-errs
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrIPAlreadyAssigned):
			conflicts++
		default:
			require.NoError(t, err)
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, conflicts)
	snapshot := requireValidSnapshot(t, db)
	assert.Len(t, snapshot.Assignments, 1)
	assert.Len(t, snapshot.IPOwners, 1)
	assert.Len(t, snapshot.Endpoints, 1)
}

func TestDeleteIntentOrderingAndExpiry(t *testing.T) {
	db, _ := openTestDB(t)
	assignment, endpoint := seedOwnershipInventory(t, db)
	_, err := db.AssignEndpoint(
		context.Background(),
		assignment,
		endpoint,
		testNow,
		testDeleteIntentTTL,
	)
	require.NoError(t, err)

	changed, err := db.ReleaseEndpoint(context.Background(), assignment.Pod, testNow)
	require.NoError(t, err)
	assert.True(t, changed)
	released := requireValidSnapshot(t, db)
	assert.Empty(t, released.Assignments)
	assert.Empty(t, released.IPOwners)
	assert.Contains(t, released.Endpoints, assignment.Pod.InfraContainerID)
	assert.Equal(t, testNow, released.DeleteIntents[assignment.Pod.InfraContainerID].CreatedAt)
	releaseGeneration := released.Metadata.Generation

	changed, err = db.ReleaseEndpoint(context.Background(), assignment.Pod, testNow.Add(time.Minute))
	require.NoError(t, err)
	assert.False(t, changed)
	repeated := requireValidSnapshot(t, db)
	assert.Equal(t, testNow, repeated.DeleteIntents[assignment.Pod.InfraContainerID].CreatedAt)
	assert.Equal(t, releaseGeneration, repeated.Metadata.Generation)

	_, err = db.AssignEndpoint(
		context.Background(),
		assignment,
		endpoint,
		testNow.Add(testDeleteIntentTTL-time.Nanosecond),
		testDeleteIntentTTL,
	)
	require.ErrorIs(t, err, ErrDeleteIntent)
	changed, err = db.AssignEndpoint(
		context.Background(),
		assignment,
		endpoint,
		testNow.Add(testDeleteIntentTTL),
		testDeleteIntentTTL,
	)
	require.NoError(t, err)
	assert.True(t, changed)
	reassigned := requireValidSnapshot(t, db)
	assert.NotContains(t, reassigned.DeleteIntents, assignment.Pod.InfraContainerID)
	assert.Contains(t, reassigned.Assignments, assignment.Pod.PodKey)
}

func TestReleaseEndpointRemovesAllContainerAssignments(t *testing.T) {
	db, _ := openTestDB(t)
	snapshot := completeSnapshot()
	writeSnapshot(t, db, snapshot)

	changed, err := db.ReleaseEndpoint(
		context.Background(),
		snapshot.Assignments["iface-primary"].Pod,
		testNow,
	)
	require.NoError(t, err)
	assert.True(t, changed)
	released := requireValidSnapshot(t, db)
	assert.Empty(t, released.Assignments)
	assert.Empty(t, released.IPOwners)
	assert.Contains(t, released.Endpoints, "container-1")
	assert.Equal(t, DeleteIntent{CreatedAt: testNow}, released.DeleteIntents["container-1"])
}

func TestAssignRequiresRetainedEndpointCleanupBeforeContainerChange(t *testing.T) {
	db, _ := openTestDB(t)
	ips := []IPRecord{
		{ID: "ip-1", IPAddress: "10.0.0.4", NCID: "nc-1"},
		{ID: "ip-2", IPAddress: "10.0.0.5", NCID: "nc-1"},
	}
	_, err := db.ApplyNetworkContainer(
		context.Background(),
		testNetworkContainer("nc-1"),
		ips,
	)
	require.NoError(t, err)
	oldAssignment := testAssignment("container-1", "pod-1", "ns-1", "ip-1")
	oldEndpoint := testEndpoint("pod-1", "ns-1", "10.0.0.4", "nc-1")
	_, err = db.AssignEndpoint(
		context.Background(),
		oldAssignment,
		oldEndpoint,
		testNow,
		testDeleteIntentTTL,
	)
	require.NoError(t, err)
	_, err = db.ReleaseEndpoint(context.Background(), oldAssignment.Pod, testNow)
	require.NoError(t, err)

	_, err = db.AssignEndpoint(
		context.Background(),
		testAssignment("container-2", "pod-1", "ns-1", "ip-2"),
		testEndpoint("pod-1", "ns-1", "10.0.0.5", "nc-1"),
		testNow.Add(testDeleteIntentTTL),
		testDeleteIntentTTL,
	)
	require.ErrorIs(t, err, ErrInvalidInput)
	snapshot := requireValidSnapshot(t, db)
	assert.Contains(t, snapshot.Endpoints, "container-1")
	assert.Empty(t, snapshot.Assignments)
}

func TestPatchAndDeleteEndpoint(t *testing.T) {
	db, _ := openTestDB(t)
	assignment, endpoint := seedOwnershipInventory(t, db)
	_, err := db.AssignEndpoint(
		context.Background(),
		assignment,
		endpoint,
		testNow,
		testDeleteIntentTTL,
	)
	require.NoError(t, err)

	patched := completeEndpointRecord()
	patched.IfnameToIPMap["eth0"].HostVethName = "veth-patched"
	changed, err := db.PatchEndpoint(
		context.Background(),
		assignment.Pod,
		patched,
		testNow,
		testDeleteIntentTTL,
	)
	require.NoError(t, err)
	assert.True(t, changed)
	afterPatch := requireValidSnapshot(t, db)
	assert.Equal(t, ownersFromAssignment(assignment), afterPatch.IPOwners)
	patchGeneration := afterPatch.Metadata.Generation

	changed, err = db.PatchEndpoint(
		context.Background(),
		assignment.Pod,
		patched,
		testNow,
		testDeleteIntentTTL,
	)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, patchGeneration, readMetadata(t, db).Generation)

	omitted := completeEndpointRecord()
	omitted.IfnameToIPMap["eth0"].IPv4 = nil
	_, err = db.PatchEndpoint(
		context.Background(),
		assignment.Pod,
		omitted,
		testNow,
		testDeleteIntentTTL,
	)
	require.ErrorIs(t, err, ErrInvalidInput)
	wrongPod := assignment.Pod
	wrongPod.PodName = "other"
	_, err = db.PatchEndpoint(
		context.Background(),
		wrongPod,
		patched,
		testNow,
		testDeleteIntentTTL,
	)
	require.ErrorIs(t, err, ErrInvalidInput)

	_, err = db.ReleaseEndpoint(context.Background(), assignment.Pod, testNow)
	require.NoError(t, err)
	_, err = db.PatchEndpoint(
		context.Background(),
		assignment.Pod,
		patched,
		testNow,
		testDeleteIntentTTL,
	)
	require.ErrorIs(t, err, ErrDeleteIntent)
	changed, err = db.DeleteEndpointRecord(context.Background(), assignment.Pod.InfraContainerID)
	require.NoError(t, err)
	assert.True(t, changed)
	deleted := requireValidSnapshot(t, db)
	assert.NotContains(t, deleted.Endpoints, assignment.Pod.InfraContainerID)
	assert.Contains(t, deleted.DeleteIntents, assignment.Pod.InfraContainerID)
	deleteGeneration := deleted.Metadata.Generation
	changed, err = db.DeleteEndpointRecord(context.Background(), assignment.Pod.InfraContainerID)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, deleteGeneration, readMetadata(t, db).Generation)
}

func TestPruneDeleteIntents(t *testing.T) {
	db, _ := openTestDB(t)
	require.NoError(t, db.Update(context.Background(), func(tx *WriteTx) error {
		for containerID, createdAt := range map[string]time.Time{
			"before": testNow.Add(-testDeleteIntentTTL - time.Nanosecond),
			"at":     testNow.Add(-testDeleteIntentTTL),
			"after":  testNow.Add(-testDeleteIntentTTL + time.Nanosecond),
		} {
			if err := tx.PutDeleteIntent(containerID, DeleteIntent{CreatedAt: createdAt}); err != nil {
				return err
			}
		}
		return nil
	}))
	before := readMetadata(t, db).Generation
	count, err := db.PruneDeleteIntents(context.Background(), testNow, testDeleteIntentTTL)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	snapshot := requireValidSnapshot(t, db)
	assert.Equal(t, map[string]DeleteIntent{
		"after": {CreatedAt: testNow.Add(-testDeleteIntentTTL + time.Nanosecond)},
	}, snapshot.DeleteIntents)
	assert.Equal(t, before+1, snapshot.Metadata.Generation)

	count, err = db.PruneDeleteIntents(context.Background(), testNow, testDeleteIntentTTL)
	require.NoError(t, err)
	assert.Zero(t, count)
	assert.Equal(t, before+1, readMetadata(t, db).Generation)

	tests := []struct {
		name string
		now  time.Time
		ttl  time.Duration
	}{
		{name: "zero time", ttl: testDeleteIntentTTL},
		{name: "zero TTL", now: testNow},
		{name: "negative TTL", now: testNow, ttl: -time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := db.PruneDeleteIntents(context.Background(), tt.now, tt.ttl)
			require.ErrorIs(t, err, ErrInvalidInput)
		})
	}
}

func TestOwnershipFailuresRollback(t *testing.T) {
	t.Run("invalid candidate", func(t *testing.T) {
		db, _ := openTestDB(t)
		assignment, endpoint := seedOwnershipInventory(t, db)
		before := requireValidSnapshot(t, db)
		endpoint.IfnameToIPMap["eth0"].MACAddress = "invalid"
		_, err := db.AssignEndpoint(
			context.Background(),
			assignment,
			endpoint,
			testNow,
			testDeleteIntentTTL,
		)
		require.ErrorIs(t, err, ErrInvalidInput)
		assert.Equal(t, before, requireValidSnapshot(t, db))
	})

	t.Run("callback failure", func(t *testing.T) {
		db, _ := openTestDB(t)
		assignment, endpoint := seedOwnershipInventory(t, db)
		before := requireValidSnapshot(t, db)
		err := db.Update(context.Background(), func(tx *WriteTx) error {
			if err := tx.PutEndpoint(assignment.Pod.InfraContainerID, endpoint); err != nil {
				return err
			}
			if err := tx.PutAssignment(assignment); err != nil {
				return err
			}
			for _, ipID := range assignment.IPIDs {
				if err := tx.PutIPOwner(ipID, assignment.Pod.PodKey); err != nil {
					return err
				}
			}
			return errAbort
		})

		t.Run("final transaction guard", func(t *testing.T) {
			db, _ := openTestDB(t)
			assignment, _ := seedOwnershipInventory(t, db)
			before := requireValidSnapshot(t, db)
			err := db.Update(context.Background(), func(tx *WriteTx) error {
				return tx.PutAssignment(assignment)
			})
			require.ErrorIs(t, err, ErrInvalidInput)
			assert.Equal(t, before, requireValidSnapshot(t, db))
		})
		require.ErrorIs(t, err, errAbort)
		assert.Equal(t, before, requireValidSnapshot(t, db))
	})

	t.Run("commit generation failure", func(t *testing.T) {
		db, _ := openTestDB(t)
		assignment, endpoint := seedOwnershipInventory(t, db)
		require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
			return tx.Bucket(bucketMetadata).Put(metaKeyGeneration, encodeUint64(^uint64(0)))
		}))
		_, err := db.AssignEndpoint(
			context.Background(),
			assignment,
			endpoint,
			testNow,
			testDeleteIntentTTL,
		)
		require.ErrorIs(t, err, ErrCorrupt)
		snapshot := requireValidSnapshot(t, db)
		assert.Empty(t, snapshot.Assignments)
		assert.Empty(t, snapshot.IPOwners)
		assert.Empty(t, snapshot.Endpoints)
		assert.Equal(t, ^uint64(0), snapshot.Metadata.Generation)
	})

	t.Run("corrupt owner preflight", func(t *testing.T) {
		db, _ := openTestDB(t)
		assignment, endpoint := seedOwnershipInventory(t, db)
		_, err := db.AssignEndpoint(
			context.Background(),
			assignment,
			endpoint,
			testNow,
			testDeleteIntentTTL,
		)
		require.NoError(t, err)
		putRaw(t, db, bucketIPOwners, []byte("ip-v4"), []byte(`"other"`))
		beforeOwner := rawValue(t, db, bucketIPOwners, []byte("ip-v4"))
		beforeGeneration := readMetadata(t, db).Generation
		_, err = db.ReleaseEndpoint(context.Background(), assignment.Pod, testNow)
		require.ErrorIs(t, err, ErrInconsistentState)
		assert.Equal(t, beforeOwner, rawValue(t, db, bucketIPOwners, []byte("ip-v4")))
		assert.Equal(t, beforeGeneration, readMetadata(t, db).Generation)
	})
}

func TestOwnershipContextReadOnlyAndClosedFailures(t *testing.T) {
	db, path := openTestDB(t)
	assignment, endpoint := seedOwnershipInventory(t, db)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := db.AssignEndpoint(ctx, assignment, endpoint, testNow, testDeleteIntentTTL)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, requireValidSnapshot(t, db).Assignments)

	require.NoError(t, db.Close())
	_, err = db.AssignEndpoint(
		context.Background(),
		assignment,
		endpoint,
		testNow,
		testDeleteIntentTTL,
	)
	require.ErrorIs(t, err, bolterrors.ErrDatabaseNotOpen)

	readOnly, err := Open(path, Options{ReadOnly: true})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, readOnly.Close()) })
	_, err = readOnly.AssignEndpoint(
		context.Background(),
		assignment,
		endpoint,
		testNow,
		testDeleteIntentTTL,
	)
	require.ErrorIs(t, err, bolterrors.ErrDatabaseReadOnly)
}

func TestConcurrentAssignReleasePatch(t *testing.T) {
	db, _ := openTestDB(t)
	assignment, endpoint := seedOwnershipInventory(t, db)
	_, err := db.AssignEndpoint(
		context.Background(),
		assignment,
		endpoint,
		testNow,
		testDeleteIntentTTL,
	)
	require.NoError(t, err)
	patched := completeEndpointRecord()
	patched.IfnameToIPMap["eth0"].HostVethName = "veth-patched"

	start := make(chan struct{})
	errs := make(chan error, 3)
	var ready sync.WaitGroup
	ready.Add(3)
	go func() {
		ready.Done()
		<-start
		_, err := db.AssignEndpoint(
			context.Background(),
			assignment,
			endpoint,
			testNow,
			testDeleteIntentTTL,
		)
		errs <- err
	}()
	go func() {
		ready.Done()
		<-start
		_, err := db.PatchEndpoint(
			context.Background(),
			assignment.Pod,
			patched,
			testNow,
			testDeleteIntentTTL,
		)
		errs <- err
	}()
	go func() {
		ready.Done()
		<-start
		_, err := db.ReleaseEndpoint(context.Background(), assignment.Pod, testNow)
		errs <- err
	}()
	ready.Wait()
	close(start)
	for range 3 {
		err := <-errs
		if err != nil {
			expected := errors.Is(err, ErrDeleteIntent) ||
				errors.Is(err, ErrInvalidInput) ||
				errors.Is(err, ErrNotFound)
			require.True(t, expected, err)
		}
	}
	snapshot := requireValidSnapshot(t, db)
	assert.Empty(t, snapshot.Assignments)
	assert.Empty(t, snapshot.IPOwners)
	assert.Contains(t, snapshot.DeleteIntents, assignment.Pod.InfraContainerID)
}

func TestOwnershipStateMachine(t *testing.T) {
	db, _ := openTestDB(t)
	assignment, endpoint := seedOwnershipInventory(t, db)
	assertStateValid := func() {
		t.Helper()
		requireValidSnapshot(t, db)
	}

	_, err := db.AssignEndpoint(
		context.Background(),
		assignment,
		endpoint,
		testNow,
		testDeleteIntentTTL,
	)
	require.NoError(t, err)
	assertStateValid()
	patched := completeEndpointRecord()
	patched.IfnameToIPMap["net1"].HNSNetworkID = "patched-network"
	_, err = db.PatchEndpoint(
		context.Background(),
		assignment.Pod,
		patched,
		testNow,
		testDeleteIntentTTL,
	)
	require.NoError(t, err)
	assertStateValid()
	_, err = db.ReleaseEndpoint(context.Background(), assignment.Pod, testNow)
	require.NoError(t, err)
	assertStateValid()
	_, err = db.AssignEndpoint(
		context.Background(),
		assignment,
		patched,
		testNow.Add(time.Minute),
		testDeleteIntentTTL,
	)
	require.ErrorIs(t, err, ErrDeleteIntent)
	assertStateValid()
	_, err = db.DeleteEndpointRecord(context.Background(), assignment.Pod.InfraContainerID)
	require.NoError(t, err)
	assertStateValid()
	count, err := db.PruneDeleteIntents(
		context.Background(),
		testNow.Add(testDeleteIntentTTL),
		testDeleteIntentTTL,
	)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assertStateValid()
	_, err = db.AssignEndpoint(
		context.Background(),
		assignment,
		endpoint,
		testNow.Add(testDeleteIntentTTL),
		testDeleteIntentTTL,
	)
	require.NoError(t, err)
	assertStateValid()
}

func TestOwnershipReopenRoundTrip(t *testing.T) {
	db, path := openTestDB(t)
	assignment, endpoint := seedOwnershipInventory(t, db)
	_, err := db.AssignEndpoint(
		context.Background(),
		assignment,
		endpoint,
		testNow,
		testDeleteIntentTTL,
	)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	reopened, err := Open(path, Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	snapshot := requireValidSnapshot(t, reopened)
	assert.Equal(t, map[string]AssignmentRecord{assignment.Pod.PodKey: assignment}, snapshot.Assignments)
	assert.Equal(t, ownersFromAssignment(assignment), snapshot.IPOwners)
	assert.Contains(t, snapshot.Endpoints, assignment.Pod.InfraContainerID)
}

func TestApplyBootRetainedEndpointFailuresRollback(t *testing.T) {
	t.Run("missing inventory", func(t *testing.T) {
		db, _ := openTestDB(t)
		snapshot := endpointOnlySnapshot()
		writeSnapshot(t, db, snapshot)

		changed, err := db.ApplyBoot(context.Background(), "boot-1", BootPolicy{})
		require.ErrorIs(t, err, ErrInvalidInput)
		assert.False(t, changed)
		got := requireValidSnapshot(t, db)
		assert.Empty(t, got.Metadata.BootID)
		assert.Zero(t, got.Metadata.Generation)
		assert.Empty(t, got.Assignments)
		assert.Empty(t, got.IPOwners)
		assert.Equal(t, snapshot.Endpoints, got.Endpoints)
	})

	t.Run("duplicate inventory", func(t *testing.T) {
		db, _ := openTestDB(t)
		snapshot := endpointOnlySnapshot()
		snapshot.Endpoints["container-1"] = testEndpoint("pod-1", "ns-1", "10.0.0.4", "nc-1")
		snapshot.IPs["ip-duplicate"] = IPRecord{
			ID:        "ip-duplicate",
			IPAddress: "10.0.0.4",
			NCID:      "nc-1",
		}
		writeSnapshot(t, db, snapshot)

		changed, err := db.ApplyBoot(context.Background(), "boot-1", BootPolicy{})
		require.ErrorIs(t, err, ErrInconsistentState)
		assert.False(t, changed)
		metadata := readMetadata(t, db)
		assert.Empty(t, metadata.BootID)
		assert.Zero(t, metadata.Generation)
		assert.Nil(t, rawValue(t, db, bucketAssignments, []byte("container-1")))
		assert.NotNil(t, rawValue(t, db, bucketEndpoints, []byte("container-1")))
	})
}

func seedOwnershipInventory(t *testing.T, db *DB) (AssignmentRecord, EndpointRecord) {
	t.Helper()
	snapshot := completeSnapshot()
	ips := make([]IPRecord, 0, len(snapshot.IPs))
	for _, ipID := range sortedKeys(snapshot.IPs) {
		ips = append(ips, snapshot.IPs[ipID])
	}
	_, err := db.ApplyNetworkContainer(
		context.Background(),
		snapshot.NetworkContainers["nc-1"],
		ips,
	)
	require.NoError(t, err)
	return testAssignment(
		"container-1",
		"pod-1",
		"ns-1",
		"ip-secondary",
		"ip-v4",
		"ip-v6",
	), completeEndpointRecord()
}

func testAssignment(containerID, podName, podNamespace string, ipIDs ...string) AssignmentRecord {
	return AssignmentRecord{
		Pod: PodIdentity{
			PodKey:           containerID,
			InfraContainerID: containerID,
			PodName:          podName,
			PodNamespace:     podNamespace,
		},
		IPIDs: ipIDs,
	}
}

func testEndpoint(podName, podNamespace, address, ncID string) EndpointRecord {
	return EndpointRecord{
		PodName:      podName,
		PodNamespace: podNamespace,
		IfnameToIPMap: map[string]*IPInfoRecord{
			"eth0": {
				IPv4: []net.IPNet{{
					IP:   net.ParseIP(address),
					Mask: net.CIDRMask(24, 32),
				}},
				MACAddress:         "00:11:22:33:44:55",
				NetworkContainerID: ncID,
			},
		},
	}
}

func requireValidSnapshot(t *testing.T, db *DB) Snapshot {
	t.Helper()
	snapshot, err := db.Snapshot(context.Background())
	require.NoError(t, err)
	require.NoError(t, snapshot.Validate())
	return snapshot
}
