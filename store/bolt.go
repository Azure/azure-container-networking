package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
	"go.uber.org/zap"
)

const boltBucket = "state"

var errBoltConfig = errors.New("store: Bolt path and logger are required")

// BoltStore holds the database file lock from Lock to Unlock, serializing the
// caller's entire read/modify/write operation across CNI and bridge processes.
type BoltStore struct {
	fileName string
	db       *bolt.DB
	logger   *zap.Logger
	mu       sync.Mutex
	readOnly bool
	// Observe is configured before use and must not block or access the store.
	Observe func(operation string, duration time.Duration, err error)
}

// NewReadOnlyBoltStore opens existing state without creating or changing it.
func NewReadOnlyBoltStore(fileName string, logger *zap.Logger) (*BoltStore, error) {
	s, err := NewBoltStore(fileName, logger)
	if err != nil {
		return nil, err
	}
	s.readOnly = true
	return s, nil
}

func NewBoltStore(fileName string, logger *zap.Logger) (*BoltStore, error) {
	if fileName == "" || logger == nil {
		return nil, errBoltConfig
	}
	return &BoltStore{fileName: fileName, logger: logger}, nil
}

func (s *BoltStore) Lock(timeout time.Duration) (resultErr error) {
	start := time.Now()
	defer func() {
		if s.Observe != nil {
			s.Observe("lock", time.Since(start), resultErr)
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return ErrStoreLocked
	}
	if timeout <= 0 {
		return ErrTimeoutLockingStore
	}
	db, err := bolt.Open(s.fileName, 0o600, &bolt.Options{Timeout: timeout, ReadOnly: s.readOnly})
	if err != nil {
		return fmt.Errorf("opening Bolt store: %w", err)
	}
	s.db = db
	return nil
}

func (s *BoltStore) Unlock() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return ErrStoreNotLocked
	}
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("closing Bolt store: %w", err)
	}
	s.db = nil
	return nil
}

func (s *BoltStore) Read(key string, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return ErrStoreNotLocked
	}
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(boltBucket))
		if bucket == nil {
			return ErrKeyNotFound
		}
		raw := bucket.Get([]byte(key))
		if raw == nil {
			return ErrKeyNotFound
		}
		return json.Unmarshal(raw, value)
	})
	if errors.Is(err, ErrKeyNotFound) {
		return ErrKeyNotFound
	}
	if err != nil {
		return fmt.Errorf("reading Bolt value: %w", err)
	}
	return nil
}

func (s *BoltStore) Write(key string, value interface{}) (resultErr error) {
	start := time.Now()
	defer func() {
		if s.Observe != nil {
			s.Observe("write", time.Since(start), resultErr)
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return ErrStoreNotLocked
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encoding Bolt value: %w", err)
	}
	err = s.db.Update(func(tx *bolt.Tx) error {
		bucket, bucketErr := tx.CreateBucketIfNotExists([]byte(boltBucket))
		if bucketErr != nil {
			return fmt.Errorf("creating Bolt bucket: %w", bucketErr)
		}
		return bucket.Put([]byte(key), raw)
	})
	if err != nil {
		return fmt.Errorf("writing Bolt value: %w", err)
	}
	return nil
}

// Flush is unnecessary: every Write commits a synchronous Bolt transaction.
func (s *BoltStore) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return ErrStoreNotLocked
	}
	return nil
}

func (s *BoltStore) Exists() bool {
	_, err := os.Stat(s.fileName)
	return err == nil
}

func (s *BoltStore) GetModificationTime() (time.Time, error) {
	info, err := os.Stat(s.fileName)
	if err != nil {
		return time.Time{}, fmt.Errorf("reading Bolt file metadata: %w", err)
	}
	return info.ModTime().UTC(), nil
}

// Remove clears stored state without unlinking the inode used for locking.
func (s *BoltStore) Remove() {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := ErrStoreNotLocked
	if s.db != nil {
		err = s.db.Update(func(tx *bolt.Tx) error {
			if tx.Bucket([]byte(boltBucket)) == nil {
				return nil
			}
			return tx.DeleteBucket([]byte(boltBucket))
		})
	}
	if err != nil {
		s.logger.Error("clearing Bolt store failed", zap.Error(err))
	}
}
