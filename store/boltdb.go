// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package store

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/processlock"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
	"go.uber.org/zap"
)

const (
	boltDefaultBucketName = "state"
)

// boltDBStore is an implementation of KeyValueStore using BoltDB.
type boltDBStore struct {
	fileName    string
	logger      *zap.Logger
	mu          sync.Mutex
	db          *bolt.DB
}

// NewBoltDBStore creates a new boltDBStore object, accessed as a KeyValueStore.
//
//nolint:revive // ignoring name change
func NewBoltDBStore(fileName string, lockclient processlock.Interface, logger *zap.Logger) (KeyValueStore, error) {
	if fileName == "" {
		return &boltDBStore{}, errors.New("need to pass in a boltdb file path")
	}
	_ = lockclient

	db, err := bolt.Open(fileName, 0o600, &bolt.Options{Timeout: DefaultLockTimeout})
	if err != nil {
		return &boltDBStore{}, errors.Wrap(err, "failed to open boltdb file")
	}

	return &boltDBStore{
		fileName: fileName,
		logger:   logger,
		db:       db,
	}, nil
}

func (kvs *boltDBStore) Exists() bool {
	if _, err := os.Stat(kvs.fileName); err != nil {
		return false
	}
	return true
}

// Read restores the value for the given key from persistent store.
func (kvs *boltDBStore) Read(key string, value interface{}) error {
	kvs.mu.Lock()
	defer kvs.mu.Unlock()

	if kvs.db == nil {
		return errors.New("boltdb is not initialized")
	}

	return kvs.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(boltDefaultBucketName))
		if bucket == nil {
			return ErrKeyNotFound
		}

		raw := bucket.Get([]byte(key))
		if raw == nil {
			return ErrKeyNotFound
		}

		return json.Unmarshal(raw, value)
	})
}

// Write saves the given key value pair to persistent store.
func (kvs *boltDBStore) Write(key string, value interface{}) error {
	kvs.mu.Lock()
	defer kvs.mu.Unlock()

	if kvs.db == nil {
		return errors.New("boltdb is not initialized")
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return kvs.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(boltDefaultBucketName))
		if err != nil {
			return err
		}

		return bucket.Put([]byte(key), raw)
	})
}

// Flush commits in-memory state to persistent store.
func (kvs *boltDBStore) Flush() error {
	kvs.mu.Lock()
	defer kvs.mu.Unlock()

	if kvs.db == nil {
		return nil
	}

	return kvs.db.Sync()
}

// Lock locks the store for exclusive access.
func (kvs *boltDBStore) Lock(timeout time.Duration) error {
	kvs.mu.Lock()
	defer kvs.mu.Unlock()
	_ = timeout
	return nil
}

// Unlock unlocks the store.
func (kvs *boltDBStore) Unlock() error {
	kvs.mu.Lock()
	defer kvs.mu.Unlock()
	return nil
}

// Close closes the underlying BoltDB file handle.
func (kvs *boltDBStore) Close() error {
	kvs.mu.Lock()
	defer kvs.mu.Unlock()

	if kvs.db == nil {
		return nil
	}

	err := kvs.db.Close()
	kvs.db = nil
	if err != nil {
		return errors.Wrap(err, "close error")
	}

	return nil
}

// GetModificationTime returns the modification time of the persistent store.
func (kvs *boltDBStore) GetModificationTime() (time.Time, error) {
	kvs.mu.Lock()
	defer kvs.mu.Unlock()

	info, err := os.Stat(kvs.fileName)
	if err != nil {
		if kvs.logger != nil {
			kvs.logger.Info("os.stat() for file", zap.String("fileName", kvs.fileName), zap.Error(err))
		} else {
			log.Printf("os.stat() for file %v failed: %v", kvs.fileName, err)
		}

		return time.Time{}.UTC(), err
	}

	return info.ModTime().UTC(), nil
}

func (kvs *boltDBStore) Remove() {
	kvs.mu.Lock()
	if kvs.db != nil {
		if err := kvs.db.Close(); err != nil {
			log.Errorf("could not close boltdb %s. Error: %v", kvs.fileName, err)
		}
		kvs.db = nil
	}
	if err := os.Remove(kvs.fileName); err != nil {
		log.Errorf("could not remove file %s. Error: %v", kvs.fileName, err)
	}
	kvs.mu.Unlock()
}
