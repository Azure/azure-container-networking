// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
	bolterrors "go.etcd.io/bbolt/errors"
)

var (
	errAbort              = errors.New("abort transaction")
	errUnexpectedCallback = errors.New("unexpected callback")
)

const testNodeID = "node-1"

func openTestDB(t *testing.T) (db *DB, path string) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "azure-cns.db")
	db, err := Open(path, Options{})
	require.NoError(t, err)
	t.Cleanup(func() {
		err := db.Close()
		require.True(t, err == nil || errors.Is(err, bolterrors.ErrDatabaseNotOpen))
	})
	return db, path
}

func readMetadata(t *testing.T, db *DB) Metadata {
	t.Helper()
	var metadata Metadata
	require.NoError(t, db.View(context.Background(), func(tx *ReadTx) error {
		var err error
		metadata, err = tx.Metadata()
		return err
	}))
	return metadata
}

func TestOpenNewDatabase(t *testing.T) {
	db, path := openTestDB(t)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assertDatabaseFileMode(t, info)

	require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
		var names [][]byte
		require.NoError(t, tx.ForEach(func(name []byte, _ *bolt.Bucket) error {
			names = append(names, append([]byte(nil), name...))
			return nil
		}))
		assert.ElementsMatch(t, allBuckets, names)
		return nil
	}))

	metadata := readMetadata(t, db)
	assert.Equal(t, SchemaVersion, metadata.SchemaVersion)
	assert.Equal(t, AuthorityBolt, metadata.Authority)
	assert.Zero(t, metadata.Generation)
}

func TestUpdateTransaction(t *testing.T) {
	tests := []struct {
		name           string
		run            func(*DB) error
		wantNodeID     string
		wantGeneration uint64
	}{
		{
			name: "commit increments generation once",
			run: func(db *DB) error {
				return db.Update(context.Background(), func(tx *WriteTx) error {
					return tx.PutMetadata(Metadata{NodeID: testNodeID})
				})
			},
			wantNodeID:     testNodeID,
			wantGeneration: 1,
		},
		{
			name: "abort preserves data and generation",
			run: func(db *DB) error {
				return db.Update(context.Background(), func(tx *WriteTx) error {
					if err := tx.PutMetadata(Metadata{NodeID: testNodeID}); err != nil {
						return err
					}
					return errAbort
				})
			},
			wantGeneration: 0,
		},
		{
			name: "internal no-op does not increment generation",
			run: func(db *DB) error {
				changed, err := db.update(context.Background(), func(*WriteTx) (bool, error) {
					return false, nil
				})
				assert.False(t, changed)
				return err
			},
			wantGeneration: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openTestDB(t)
			err := tt.run(db)
			if tt.name == "abort preserves data and generation" {
				require.ErrorIs(t, err, errAbort)
			} else {
				require.NoError(t, err)
			}
			metadata := readMetadata(t, db)
			assert.Equal(t, tt.wantNodeID, metadata.NodeID)
			assert.Equal(t, tt.wantGeneration, metadata.Generation)
		})
	}
}

func TestReopenPersistence(t *testing.T) {
	db, path := openTestDB(t)
	require.NoError(t, db.Update(context.Background(), func(tx *WriteTx) error {
		return tx.PutMetadata(Metadata{NodeID: testNodeID})
	}))
	require.NoError(t, db.Close())

	reopened, err := Open(path, Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	metadata := readMetadata(t, reopened)
	assert.Equal(t, testNodeID, metadata.NodeID)
	assert.Equal(t, uint64(1), metadata.Generation)
}

func TestOpenLockTimeout(t *testing.T) {
	_, path := openTestDB(t)
	_, err := Open(path, Options{Timeout: 250 * time.Millisecond})
	require.ErrorIs(t, err, bolterrors.ErrTimeout)
}

func TestCanceledContext(t *testing.T) {
	db, _ := openTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "view",
			run: func() error {
				return db.View(ctx, func(*ReadTx) error {
					t.Fatal("view callback called")
					return nil
				})
			},
		},
		{
			name: "update",
			run: func() error {
				return db.Update(ctx, func(*WriteTx) error {
					t.Fatal("update callback called")
					return nil
				})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorIs(t, tt.run(), context.Canceled)
		})
	}
}

func TestUpdateCancellationWhileWaitingForWriteGate(t *testing.T) {
	db, _ := openTestDB(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- db.Update(context.Background(), func(*WriteTx) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	ctx, cancel := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- db.Update(ctx, func(*WriteTx) error {
			return errUnexpectedCallback
		})
	}()
	cancel()
	require.ErrorIs(t, <-secondDone, context.Canceled)

	close(release)
	require.NoError(t, <-firstDone)
}

func TestReadOnlyOpen(t *testing.T) {
	db, path := openTestDB(t)
	require.NoError(t, db.Update(context.Background(), func(tx *WriteTx) error {
		return tx.PutMetadata(Metadata{NodeID: testNodeID})
	}))
	require.NoError(t, db.Close())

	readOnly, err := Open(path, Options{ReadOnly: true})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, readOnly.Close()) })
	assert.Equal(t, testNodeID, readMetadata(t, readOnly).NodeID)

	err = readOnly.Update(context.Background(), func(*WriteTx) error {
		t.Fatal("read-only update callback called")
		return nil
	})
	require.ErrorIs(t, err, bolterrors.ErrDatabaseReadOnly)
	assert.Equal(t, uint64(1), readMetadata(t, readOnly).Generation)
}

func TestClose(t *testing.T) {
	t.Run("nil database", func(t *testing.T) {
		var db *DB
		require.NoError(t, db.Close())
	})

	t.Run("closed database operations", func(t *testing.T) {
		db, _ := openTestDB(t)
		require.NoError(t, db.Close())
		require.ErrorIs(t, db.View(context.Background(), func(*ReadTx) error {
			return nil
		}), bolterrors.ErrDatabaseNotOpen)
		require.ErrorIs(t, db.Update(context.Background(), func(*WriteTx) error {
			return nil
		}), bolterrors.ErrDatabaseNotOpen)
	})
}

func TestOpenRejectsSchemaMismatch(t *testing.T) {
	path := createValidClosedDB(t)
	mutateDatabase(t, path, func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMetadata).Put(metaKeySchemaVersion, encodeUint32(SchemaVersion+1))
	})

	for _, tt := range []struct {
		name string
		opts Options
	}{
		{name: "writable"},
		{name: "read-only", opts: Options{ReadOnly: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Open(path, tt.opts)
			require.ErrorIs(t, err, ErrSchemaMismatch)
			assert.NotErrorIs(t, err, ErrCorrupt)
		})
	}
}

func TestOpenRejectsCorruption(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*bolt.Tx) error
	}{
		{
			name: "missing schema",
			mutate: func(tx *bolt.Tx) error {
				return tx.Bucket(bucketMetadata).Delete(metaKeySchemaVersion)
			},
		},
		{
			name: "malformed schema",
			mutate: func(tx *bolt.Tx) error {
				return tx.Bucket(bucketMetadata).Put(metaKeySchemaVersion, []byte{1})
			},
		},
		{
			name: "missing generation",
			mutate: func(tx *bolt.Tx) error {
				return tx.Bucket(bucketMetadata).Delete(metaKeyGeneration)
			},
		},
		{
			name: "malformed generation",
			mutate: func(tx *bolt.Tx) error {
				return tx.Bucket(bucketMetadata).Put(metaKeyGeneration, []byte{1})
			},
		},
		{
			name: "missing authority",
			mutate: func(tx *bolt.Tx) error {
				return tx.Bucket(bucketMetadata).Delete(metaKeyAuthority)
			},
		},
		{
			name: "invalid authority",
			mutate: func(tx *bolt.Tx) error {
				return tx.Bucket(bucketMetadata).Put(metaKeyAuthority, []byte("other"))
			},
		},
		{
			name: "missing bucket",
			mutate: func(tx *bolt.Tx) error {
				return tx.DeleteBucket(bucketEndpoints)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := createValidClosedDB(t)
			mutateDatabase(t, path, tt.mutate)
			for _, mode := range []struct {
				name string
				opts Options
			}{
				{name: "writable"},
				{name: "read-only", opts: Options{ReadOnly: true}},
			} {
				t.Run(mode.name, func(t *testing.T) {
					_, err := Open(path, mode.opts)
					require.ErrorIs(t, err, ErrCorrupt)
				})
			}
		})
	}
}

func TestOpenAcceptsKnownAuthorities(t *testing.T) {
	for _, authority := range []Authority{AuthorityBolt, AuthorityJSON} {
		t.Run(string(authority), func(t *testing.T) {
			path := createValidClosedDB(t)
			mutateDatabase(t, path, func(tx *bolt.Tx) error {
				return tx.Bucket(bucketMetadata).Put(metaKeyAuthority, []byte(authority))
			})
			db, err := Open(path, Options{ReadOnly: true})
			require.NoError(t, err)
			require.NoError(t, db.Close())
		})
	}
}

func TestOpenRejectsNonBoltFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-bolt.db")
	require.NoError(t, os.WriteFile(path, []byte("not a bbolt database"), 0o600))

	_, err := Open(path, Options{})
	require.ErrorIs(t, err, ErrCorrupt)
}

func createValidClosedDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "azure-cns.db")
	db, err := Open(path, Options{})
	require.NoError(t, err)
	require.NoError(t, db.Close())
	return path
}

func mutateDatabase(t *testing.T, path string, mutate func(*bolt.Tx) error) {
	t.Helper()
	db, err := bolt.Open(path, 0o600, nil)
	require.NoError(t, err)
	require.NoError(t, db.Update(mutate))
	require.NoError(t, db.Close())
}
