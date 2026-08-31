// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"time"

	bolt "go.etcd.io/bbolt"
	bolterrors "go.etcd.io/bbolt/errors"
)

var (
	ErrCorrupt        = errors.New("cns state: corrupt database")
	ErrSchemaMismatch = errors.New("cns state: schema version mismatch")

	errUnknownAuthority = errors.New("unknown authority")
	errInvalidUint32    = errors.New("invalid uint32 encoding length")
	errInvalidUint64    = errors.New("invalid uint64 encoding length")
)

const defaultOpenTimeout = 5 * time.Second

var (
	bucketMetadata             = []byte("metadata")
	bucketNetworkContainers    = []byte("network_containers")
	bucketIPs                  = []byte("ips")
	bucketNetworks             = []byte("networks")
	bucketOrchestratorContexts = []byte("orchestrator_contexts")
	bucketPnPIDByMAC           = []byte("pnp_id_by_mac")
	bucketAssignments          = []byte("assignments")
	bucketIPOwners             = []byte("ip_owners")
	bucketEndpoints            = []byte("endpoints")
	bucketDeleteIntents        = []byte("delete_intents")
)

var allBuckets = [][]byte{
	bucketMetadata,
	bucketNetworkContainers,
	bucketIPs,
	bucketNetworks,
	bucketOrchestratorContexts,
	bucketPnPIDByMAC,
	bucketAssignments,
	bucketIPOwners,
	bucketEndpoints,
	bucketDeleteIntents,
}

var (
	metaKeySchemaVersion = []byte("schema_version")
	metaKeyAuthority     = []byte("authority")
	metaKeyGeneration    = []byte("generation")
	metaKeyBootID        = []byte("boot_id")
	metaKeyService       = []byte("service")
)

type Options struct {
	Timeout  time.Duration
	ReadOnly bool
	NoSync   bool
}

type DB struct {
	db        *bolt.DB
	writeGate chan struct{}
}

func Open(path string, opts Options) (*DB, error) {
	_, statErr := os.Stat(path)
	exists := statErr == nil
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, fmt.Errorf("stat cns state database: %w", statErr)
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = defaultOpenTimeout
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{
		Timeout:  timeout,
		ReadOnly: opts.ReadOnly,
		NoSync:   opts.NoSync,
	})
	if err != nil {
		if isBoltCorruption(err) {
			return nil, corrupt("opening cns state database", err)
		}
		return nil, fmt.Errorf("opening cns state database: %w", err)
	}

	store := &DB{db: db, writeGate: make(chan struct{}, 1)}
	created := !exists && !opts.ReadOnly
	if created {
		err = store.initialize()
	} else {
		err = store.validate()
	}
	if err != nil {
		_ = db.Close()
		if created {
			if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				return nil, fmt.Errorf("%w: removing incomplete database: %w", err, removeErr)
			}
		}
		return nil, err
	}
	return store, nil
}

func (s *DB) initialize() error {
	if err := s.db.Update(func(tx *bolt.Tx) error {
		for _, name := range allBuckets {
			if _, err := tx.CreateBucket(name); err != nil {
				return fmt.Errorf("creating bucket %q: %w", name, err)
			}
		}
		meta := tx.Bucket(bucketMetadata)
		if err := meta.Put(metaKeySchemaVersion, encodeUint32(SchemaVersion)); err != nil {
			return fmt.Errorf("writing schema version: %w", err)
		}
		if err := meta.Put(metaKeyAuthority, []byte(AuthorityBolt)); err != nil {
			return fmt.Errorf("writing state authority: %w", err)
		}
		if err := meta.Put(metaKeyGeneration, encodeUint64(0)); err != nil {
			return fmt.Errorf("writing state generation: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("initializing cns state database: %w", err)
	}
	return nil
}

func (s *DB) validate() error {
	if err := s.db.View(func(tx *bolt.Tx) error {
		var checkErr error
		for err := range tx.Check() {
			if checkErr == nil {
				checkErr = err
			}
		}
		if checkErr != nil {
			return corrupt("checking cns state database", checkErr)
		}
		for _, name := range allBuckets {
			if tx.Bucket(name) == nil {
				return corrupt(fmt.Sprintf("missing bucket %q", name), nil)
			}
		}

		meta := tx.Bucket(bucketMetadata)
		version, err := decodeUint32(meta.Get(metaKeySchemaVersion))
		if err != nil {
			return corrupt("invalid schema version", err)
		}
		if version != SchemaVersion {
			return fmt.Errorf("%w: database=%d code=%d", ErrSchemaMismatch, version, SchemaVersion)
		}
		if err := validateAuthority(Authority(meta.Get(metaKeyAuthority))); err != nil {
			return corrupt("invalid authority", err)
		}
		if _, err := decodeUint64(meta.Get(metaKeyGeneration)); err != nil {
			return corrupt("invalid generation", err)
		}
		return nil
	}); err != nil {
		if errors.Is(err, ErrCorrupt) || errors.Is(err, ErrSchemaMismatch) {
			return fmt.Errorf("validating cns state database: %w", err)
		}
		return corrupt("validating cns state database", err)
	}
	return nil
}

func (s *DB) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("closing cns state database: %w", err)
	}
	return nil
}

func (s *DB) View(ctx context.Context, fn func(*ReadTx) error) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("viewing cns state: %w", err)
	}
	if err := s.db.View(func(tx *bolt.Tx) error {
		return fn(&ReadTx{tx: tx, ctx: ctx})
	}); err != nil {
		return fmt.Errorf("viewing cns state: %w", err)
	}
	return nil
}

func (s *DB) Update(ctx context.Context, fn func(*WriteTx) error) error {
	_, err := s.update(ctx, func(tx *WriteTx) (bool, error) {
		if err := fn(tx); err != nil {
			return false, err
		}
		return true, nil
	})
	return err
}

func (s *DB) update(ctx context.Context, fn func(*WriteTx) (bool, error)) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("updating cns state: %w", err)
	}
	select {
	case s.writeGate <- struct{}{}:
		defer func() { <-s.writeGate }()
	case <-ctx.Done():
		return false, fmt.Errorf("updating cns state: %w", ctx.Err())
	}
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("updating cns state: %w", err)
	}

	changed := false
	if err := s.db.Update(func(tx *bolt.Tx) error {
		var err error
		changed, err = fn(&WriteTx{ReadTx: ReadTx{tx: tx, ctx: ctx}})
		if err != nil || !changed {
			return err
		}
		meta := tx.Bucket(bucketMetadata)
		generation, err := decodeUint64(meta.Get(metaKeyGeneration))
		if err != nil {
			return corrupt("invalid generation", err)
		}
		if generation == math.MaxUint64 {
			return corrupt("generation overflow", nil)
		}
		if err := meta.Put(metaKeyGeneration, encodeUint64(generation+1)); err != nil {
			return fmt.Errorf("writing state generation: %w", err)
		}
		return nil
	}); err != nil {
		return false, fmt.Errorf("updating cns state: %w", err)
	}
	return changed, nil
}

func validateAuthority(authority Authority) error {
	switch authority {
	case AuthorityBolt, AuthorityJSON:
		return nil
	default:
		return fmt.Errorf("%w: %q", errUnknownAuthority, authority)
	}
}

func encodeUint32(value uint32) []byte {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, value)
	return data
}

func decodeUint32(data []byte) (uint32, error) {
	if len(data) != 4 {
		return 0, fmt.Errorf("%w: got %d bytes", errInvalidUint32, len(data))
	}
	return binary.LittleEndian.Uint32(data), nil
}

func encodeUint64(value uint64) []byte {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, value)
	return data
}

func decodeUint64(data []byte) (uint64, error) {
	if len(data) != 8 {
		return 0, fmt.Errorf("%w: got %d bytes", errInvalidUint64, len(data))
	}
	return binary.LittleEndian.Uint64(data), nil
}

func corrupt(operation string, err error) error {
	if err == nil {
		return fmt.Errorf("%w: %s", ErrCorrupt, operation)
	}
	return fmt.Errorf("%w: %s: %w", ErrCorrupt, operation, err)
}

func isBoltCorruption(err error) bool {
	return errors.Is(err, bolterrors.ErrInvalid) ||
		errors.Is(err, bolterrors.ErrVersionMismatch) ||
		errors.Is(err, bolterrors.ErrChecksum)
}
