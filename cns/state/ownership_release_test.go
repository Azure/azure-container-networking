// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolterrors "go.etcd.io/bbolt/errors"
)

func TestReleaseEndpointIfGenerationPrecommitAndFailures(t *testing.T) {
	t.Run("candidate callback failure rolls back", func(t *testing.T) {
		db, _ := openTestDB(t)
		assignment, endpoint := seedOwnershipInventory(t, db)
		_, err := db.AssignEndpoint(context.Background(), assignment, endpoint, testNow, testDeleteIntentTTL)
		require.NoError(t, err)
		before := requireValidSnapshot(t, db)
		injected := errors.New("injected release candidate failure")
		callbackCalled := false

		changed, err := db.ReleaseEndpointIfGeneration(
			context.Background(),
			before.Metadata.Generation,
			assignment.Pod,
			testNow.Add(time.Minute),
			func(candidate Snapshot) error {
				callbackCalled = true
				assert.Equal(t, before.Metadata.Generation+1, candidate.Metadata.Generation)
				assert.Empty(t, candidate.Assignments)
				assert.Empty(t, candidate.IPOwners)
				assert.Contains(t, candidate.Endpoints, assignment.Pod.InfraContainerID)
				assert.Equal(t, testNow.Add(time.Minute), candidate.DeleteIntents[assignment.Pod.InfraContainerID].CreatedAt)
				return injected
			},
		)
		require.ErrorIs(t, err, injected)
		assert.False(t, changed)
		assert.True(t, callbackCalled)
		assert.Equal(t, before, requireValidSnapshot(t, db))
	})

	t.Run("stale generation does not invoke callback", func(t *testing.T) {
		db, _ := openTestDB(t)
		assignment, endpoint := seedOwnershipInventory(t, db)
		_, err := db.AssignEndpoint(context.Background(), assignment, endpoint, testNow, testDeleteIntentTTL)
		require.NoError(t, err)
		before := requireValidSnapshot(t, db)

		changed, err := db.ReleaseEndpointIfGeneration(
			context.Background(),
			before.Metadata.Generation+1,
			assignment.Pod,
			testNow,
			func(Snapshot) error {
				t.Fatal("stale generation invoked release callback")
				return nil
			},
		)
		require.ErrorIs(t, err, ErrStaleGeneration)
		assert.False(t, changed)
		assert.Equal(t, before, requireValidSnapshot(t, db))
	})

	t.Run("canceled while waiting for writer gate", func(t *testing.T) {
		db, _ := openTestDB(t)
		assignment, endpoint := seedOwnershipInventory(t, db)
		_, err := db.AssignEndpoint(context.Background(), assignment, endpoint, testNow, testDeleteIntentTTL)
		require.NoError(t, err)
		before := requireValidSnapshot(t, db)
		db.writeGate <- struct{}{}
		t.Cleanup(func() {
			select {
			case <-db.writeGate:
			default:
			}
		})
		ctx, cancel := context.WithCancel(context.Background())
		started := make(chan struct{})
		result := make(chan error, 1)
		go func() {
			close(started)
			_, err := db.ReleaseEndpointIfGeneration(
				ctx,
				before.Metadata.Generation,
				assignment.Pod,
				testNow,
				nil,
			)
			result <- err
		}()
		<-started
		cancel()
		require.ErrorIs(t, <-result, context.Canceled)
		<-db.writeGate
		assert.Equal(t, before, requireValidSnapshot(t, db))
	})

	t.Run("read only", func(t *testing.T) {
		db, path := openTestDB(t)
		assignment, endpoint := seedOwnershipInventory(t, db)
		_, err := db.AssignEndpoint(context.Background(), assignment, endpoint, testNow, testDeleteIntentTTL)
		require.NoError(t, err)
		before := requireValidSnapshot(t, db)
		require.NoError(t, db.Close())
		readOnly, err := Open(path, Options{ReadOnly: true})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, readOnly.Close()) })

		changed, err := readOnly.ReleaseEndpointIfGeneration(
			context.Background(),
			before.Metadata.Generation,
			assignment.Pod,
			testNow,
			nil,
		)
		require.ErrorIs(t, err, bolterrors.ErrDatabaseReadOnly)
		assert.False(t, changed)
		assert.Equal(t, before, requireValidSnapshot(t, readOnly))
	})

	t.Run("successful callback commits candidate", func(t *testing.T) {
		db, _ := openTestDB(t)
		assignment, endpoint := seedOwnershipInventory(t, db)
		_, err := db.AssignEndpoint(context.Background(), assignment, endpoint, testNow, testDeleteIntentTTL)
		require.NoError(t, err)
		before := requireValidSnapshot(t, db)

		changed, err := db.ReleaseEndpointIfGeneration(
			context.Background(),
			before.Metadata.Generation,
			assignment.Pod,
			testNow,
			func(candidate Snapshot) error {
				assert.Empty(t, candidate.Assignments)
				assert.Contains(t, candidate.DeleteIntents, assignment.Pod.InfraContainerID)
				return nil
			},
		)
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Empty(t, requireValidSnapshot(t, db).Assignments)
	})
}

func TestDeleteAndPruneIfGenerationPrecommit(t *testing.T) {
	t.Run("stale generations do not mutate", func(t *testing.T) {
		db, _ := openTestDB(t)
		assignment, endpoint := seedOwnershipInventory(t, db)
		_, err := db.AssignEndpoint(context.Background(), assignment, endpoint, testNow, testDeleteIntentTTL)
		require.NoError(t, err)
		before := requireValidSnapshot(t, db)

		changed, err := db.DeleteEndpointRecordIfGeneration(
			context.Background(),
			before.Metadata.Generation+1,
			assignment.Pod.InfraContainerID,
			nil,
		)
		require.ErrorIs(t, err, ErrStaleGeneration)
		assert.False(t, changed)
		count, err := db.PruneDeleteIntentsIfGeneration(
			context.Background(),
			before.Metadata.Generation+1,
			testNow,
			testDeleteIntentTTL,
			nil,
		)
		require.ErrorIs(t, err, ErrStaleGeneration)
		assert.Zero(t, count)
		assert.Equal(t, before, requireValidSnapshot(t, db))
	})

	t.Run("endpoint delete and repeated no-op callbacks", func(t *testing.T) {
		db, _ := openTestDB(t)
		assignment, endpoint := seedOwnershipInventory(t, db)
		_, err := db.AssignEndpoint(context.Background(), assignment, endpoint, testNow, testDeleteIntentTTL)
		require.NoError(t, err)
		_, err = db.ReleaseEndpoint(context.Background(), assignment.Pod, testNow)
		require.NoError(t, err)
		before := requireValidSnapshot(t, db)
		callbacks := 0

		changed, err := db.DeleteEndpointRecordIfGeneration(
			context.Background(),
			before.Metadata.Generation,
			assignment.Pod.InfraContainerID,
			func(candidate Snapshot) error {
				callbacks++
				assert.Equal(t, before.Metadata.Generation+1, candidate.Metadata.Generation)
				return nil
			},
		)
		require.NoError(t, err)
		assert.True(t, changed)
		after := requireValidSnapshot(t, db)
		changed, err = db.DeleteEndpointRecordIfGeneration(
			context.Background(),
			after.Metadata.Generation,
			assignment.Pod.InfraContainerID,
			func(candidate Snapshot) error {
				callbacks++
				assert.Equal(t, after.Metadata.Generation, candidate.Metadata.Generation)
				return nil
			},
		)
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, 2, callbacks)
		injected := errors.New("injected no-op callback failure")
		changed, err = db.DeleteEndpointRecordIfGeneration(
			context.Background(),
			after.Metadata.Generation,
			assignment.Pod.InfraContainerID,
			func(Snapshot) error { return injected },
		)
		require.ErrorIs(t, err, injected)
		assert.False(t, changed)
	})

	t.Run("endpoint delete callback failure rolls back", func(t *testing.T) {
		db, _ := openTestDB(t)
		assignment, endpoint := seedOwnershipInventory(t, db)
		_, err := db.AssignEndpoint(context.Background(), assignment, endpoint, testNow, testDeleteIntentTTL)
		require.NoError(t, err)
		_, err = db.ReleaseEndpoint(context.Background(), assignment.Pod, testNow)
		require.NoError(t, err)
		before := requireValidSnapshot(t, db)
		injected := errors.New("injected endpoint delete candidate failure")

		changed, err := db.DeleteEndpointRecordIfGeneration(
			context.Background(),
			before.Metadata.Generation,
			assignment.Pod.InfraContainerID,
			func(candidate Snapshot) error {
				assert.NotContains(t, candidate.Endpoints, assignment.Pod.InfraContainerID)
				assert.Contains(t, candidate.DeleteIntents, assignment.Pod.InfraContainerID)
				return injected
			},
		)
		require.ErrorIs(t, err, injected)
		assert.False(t, changed)
		assert.Equal(t, before, requireValidSnapshot(t, db))
	})

	t.Run("intent prune callback failure rolls back", func(t *testing.T) {
		db, _ := openTestDB(t)
		require.NoError(t, db.Update(context.Background(), func(tx *WriteTx) error {
			return tx.PutDeleteIntent("container-1", DeleteIntent{
				CreatedAt: testNow.Add(-testDeleteIntentTTL),
			})
		}))
		before := requireValidSnapshot(t, db)
		injected := errors.New("injected prune candidate failure")

		count, err := db.PruneDeleteIntentsIfGeneration(
			context.Background(),
			before.Metadata.Generation,
			testNow,
			testDeleteIntentTTL,
			func(candidate Snapshot) error {
				assert.NotContains(t, candidate.DeleteIntents, "container-1")
				return injected
			},
		)
		require.ErrorIs(t, err, injected)
		assert.Zero(t, count)
		assert.Equal(t, before, requireValidSnapshot(t, db))
	})

	t.Run("intent prune and repeated no-op callbacks", func(t *testing.T) {
		db, _ := openTestDB(t)
		require.NoError(t, db.Update(context.Background(), func(tx *WriteTx) error {
			return tx.PutDeleteIntent("container-1", DeleteIntent{
				CreatedAt: testNow.Add(-testDeleteIntentTTL),
			})
		}))
		before := requireValidSnapshot(t, db)
		callbacks := 0
		count, err := db.PruneDeleteIntentsIfGeneration(
			context.Background(),
			before.Metadata.Generation,
			testNow,
			testDeleteIntentTTL,
			func(candidate Snapshot) error {
				callbacks++
				assert.Empty(t, candidate.DeleteIntents)
				return nil
			},
		)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
		after := requireValidSnapshot(t, db)
		count, err = db.PruneDeleteIntentsIfGeneration(
			context.Background(),
			after.Metadata.Generation,
			testNow,
			testDeleteIntentTTL,
			func(candidate Snapshot) error {
				callbacks++
				assert.Equal(t, after.Metadata.Generation, candidate.Metadata.Generation)
				return nil
			},
		)
		require.NoError(t, err)
		assert.Zero(t, count)
		assert.Equal(t, 2, callbacks)
	})
}
