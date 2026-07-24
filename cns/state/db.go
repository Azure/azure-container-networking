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
	"go.uber.org/zap"
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
	metaKeySchemaVersion  = []byte("schema_version")
	metaKeyAuthority      = []byte("authority")
	metaKeyGeneration     = []byte("generation")
	metaKeyBootID         = []byte("boot_id")
	metaKeyService        = []byte("service")
	metaKeyLegacyImport   = []byte("legacy_import_complete")
	metaKeyRollbackExport = []byte("rollback_export_complete")
)

type Options struct {
	Timeout  time.Duration
	ReadOnly bool
	NoSync   bool
	Metrics  *Metrics
	Logger   *zap.Logger
}

type DB struct {
	db        *bolt.DB
	writeGate chan struct{}
	metrics   *Metrics
	logger    *zap.Logger
}

func Open(path string, opts Options) (store *DB, returnErr error) {
	return OpenContext(context.TODO(), path, opts)
}

// OpenContext opens and validates a persistent state database while preserving
// startup cancellation through observability and bounded lock waits.
func OpenContext(ctx context.Context, path string, opts Options) (store *DB, returnErr error) {
	if ctx == nil {
		return nil, errors.New("opening cns state database: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("opening cns state database: %w", err)
	}
	if opts.Metrics != nil || opts.Logger != nil {
		started := metricNow(opts.Metrics)
		defer func() {
			result := classifyResult(true, returnErr)
			duration := metricDuration(opts.Metrics, started)
			_ = opts.Metrics.ObserveLifecycle(LifecycleStartup, result, duration)
			if returnErr == nil {
				store.observeLifecycle(ctx, LifecycleStartup, result, duration)
			}
		}()
	}

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
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("opening cns state database: %w", ctxErr)
		}
		if isBoltCorruption(err) {
			return nil, corrupt("opening cns state database", err)
		}
		return nil, fmt.Errorf("opening cns state database: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = db.Close()
		if !exists && !opts.ReadOnly {
			_ = os.Remove(path)
		}
		return nil, fmt.Errorf("opening cns state database: %w", err)
	}

	store = &DB{
		db:        db,
		writeGate: make(chan struct{}, 1),
		metrics:   opts.Metrics,
	}
	if opts.Logger != nil {
		store.logger = opts.Logger.With(zap.String("component", "persistent_state"))
	}
	created := !exists && !opts.ReadOnly
	if created {
		err = store.initialize()
	} else {
		err = store.validate()
	}
	if err == nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = fmt.Errorf("opening cns state database: %w", ctxErr)
		}
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
		if err := validateMigrationMetadata(meta); err != nil {
			return err
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

func (s *DB) View(ctx context.Context, fn func(*ReadTx) error) (returnErr error) {
	if s.metrics != nil {
		started := metricNow(s.metrics)
		defer func() {
			_ = s.metrics.ObserveTransaction(
				TransactionView,
				classifyResult(true, returnErr),
				metricDuration(s.metrics, started),
			)
		}()
	}
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

func (s *DB) update(ctx context.Context, fn func(*WriteTx) (bool, error)) (changed bool, returnErr error) {
	if s.metrics != nil {
		started := metricNow(s.metrics)
		defer func() {
			_ = s.metrics.ObserveTransaction(
				TransactionUpdate,
				classifyResult(changed, returnErr),
				metricDuration(s.metrics, started),
			)
		}()
	}
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("updating cns state: %w", err)
	}
	if err := s.acquireWriteGate(ctx); err != nil {
		return false, fmt.Errorf("updating cns state: %w", err)
	}
	defer s.releaseWriteGate()

	return s.updateLocked(ctx, fn)
}

func (s *DB) acquireWriteGate(ctx context.Context) error {
	select {
	case s.writeGate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("acquiring write gate: %w", ctx.Err())
	}
}

func (s *DB) releaseWriteGate() {
	<-s.writeGate
}

func (s *DB) updateLocked(ctx context.Context, fn func(*WriteTx) (bool, error)) (bool, error) {
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
		candidate, err := snapshotFromTx(ctx, &ReadTx{tx: tx, ctx: ctx})
		if err != nil {
			return err
		}
		if validationErr := validateInput(candidate); validationErr != nil {
			return validationErr
		}
		meta := tx.Bucket(bucketMetadata)
		migrationErr := validateMigrationMetadata(meta)
		if migrationErr != nil {
			return migrationErr
		}
		generation, err := decodeUint64(meta.Get(metaKeyGeneration))
		if err != nil {
			return corrupt("invalid generation", err)
		}
		if generation == math.MaxUint64 {
			return corrupt("generation overflow", nil)
		}
		writeErr := meta.Put(metaKeyGeneration, encodeUint64(generation+1))
		if writeErr != nil {
			return fmt.Errorf("writing state generation: %w", writeErr)
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
