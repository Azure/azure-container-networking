// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package store

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/processlock"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	// LockExtension - Extension added to the file name for lock.
	LockExtension = ".lock"

	// DefaultLockTimeout - lock timeout in milliseconds
	DefaultLockTimeout        = 10000 * time.Millisecond
	DefaultLockTimeoutLinux   = 30000 * time.Millisecond
	DefaultLockTimeoutWindows = 60000 * time.Millisecond
)

// jsonFileStore is an implementation of KeyValueStore using a local JSON file.
type jsonFileStore struct {
	fileName    string
	data        map[string]*json.RawMessage
	inSync      bool
	processLock processlock.Interface
	sync.Mutex
	logger *zap.Logger
}

// NewJsonFileStore creates a new jsonFileStore object, accessed as a KeyValueStore.
//
//nolint:revive // ignoring name change
func NewJsonFileStore(fileName string, lockclient processlock.Interface, logger *zap.Logger) (KeyValueStore, error) {
	if fileName == "" {
		return &jsonFileStore{}, errors.New("need to pass in a json file path")
	}
	kvs := &jsonFileStore{
		fileName:    fileName,
		processLock: lockclient,
		data:        make(map[string]*json.RawMessage),
		logger:      logger,
	}

	return kvs, nil
}

func (kvs *jsonFileStore) Exists() bool {
	if _, err := os.Stat(kvs.fileName); err != nil {
		return false
	}
	return true
}

// Read restores the value for the given key from persistent store.
func (kvs *jsonFileStore) Read(key string, value interface{}) error {
	kvs.Mutex.Lock()
	defer kvs.Mutex.Unlock()

	// Read contents from file if memory is not in sync.
	if !kvs.inSync {
		// Open and parse the file if it exists.
		file, err := os.Open(kvs.fileName)
		if err != nil {
			if os.IsNotExist(err) {
				return ErrKeyNotFound
			}
			return err
		}
		defer file.Close()

		b, err := io.ReadAll(file)
		if err != nil {
			return err
		}

		if len(b) == 0 {
			if kvs.logger != nil {
				kvs.logger.Info("Unable to read empty file", zap.String("fileName", kvs.fileName))
			} else {
				log.Printf("Unable to read file %s, was empty", kvs.fileName)
			}

			return ErrStoreEmpty
		}

		// Decode to raw JSON messages.
		if err := json.Unmarshal(b, &kvs.data); err != nil {
			return err
		}

		kvs.inSync = true
	}

	raw, ok := kvs.data[key]
	if !ok {
		return ErrKeyNotFound
	}

	return json.Unmarshal(*raw, value)
}

// Write saves the given key value pair to persistent store.
func (kvs *jsonFileStore) Write(key string, value interface{}) error {
	kvs.Mutex.Lock()
	defer kvs.Mutex.Unlock()

	var raw json.RawMessage
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}

	kvs.data[key] = &raw

	return kvs.flush()
}

// Flush commits in-memory state to persistent store.
func (kvs *jsonFileStore) Flush() error {
	kvs.Mutex.Lock()
	defer kvs.Mutex.Unlock()

	return kvs.flush()
}

// Lock-free flush for internal callers.
func (kvs *jsonFileStore) flush() error {
	buf, err := json.MarshalIndent(&kvs.data, "", "\t")
	if err != nil {
		return err
	}

	dir, file := filepath.Split(kvs.fileName)
	if dir == "" {
		dir = "."
	}

	f, err := os.CreateTemp(dir, file)
	if err != nil {
		return fmt.Errorf("cannot create temp file: %v", err)
	}

	tmpFileName := f.Name()

	defer func() {
		if err != nil {
			// remove temp file after job is done
			_ = os.Remove(tmpFileName)
			// close is idempotent. just to catch if write returns error
			f.Close()
		}
	}()

	if _, err = f.Write(buf); err != nil {
		return fmt.Errorf("Temp file write failed with: %v", err)
	}

	if err = f.Close(); err != nil {
		return fmt.Errorf("temp file close failed with: %v", err)
	}

	// atomic replace
	if err = platform.ReplaceFile(tmpFileName, kvs.fileName); err != nil {
		return fmt.Errorf("rename temp file to state file failed:%v", err)
	}

	return nil
}

func (kvs *jsonFileStore) lockUtil(status chan error) {
	err := kvs.processLock.Lock()
	status <- err
}

// Lock locks the store for exclusive access.
func (kvs *jsonFileStore) Lock(timeout time.Duration) error {
	kvs.Mutex.Lock()
	defer kvs.Mutex.Unlock()

	afterTime := time.After(timeout)
	status := make(chan error)

	if kvs.logger != nil {
		kvs.logger.Info("Acquiring process lock")
	} else {
		log.Printf("Acquiring process lock")
	}

	go kvs.lockUtil(status)

	var err error
	select {
	case <-afterTime:
		return ErrTimeoutLockingStore
	case err = <-status:
	}

	if err != nil {
		return errors.Wrap(err, "processLock acquire error")
	}

	if kvs.logger != nil {
		kvs.logger.Info("Acquired process lock with timeout value of", zap.Any("timeout", timeout))
	} else {
		log.Printf("Acquired process lock with timeout value of %v", timeout)
	}

	return nil
}

// Unlock unlocks the store.
func (kvs *jsonFileStore) Unlock() error {
	kvs.Mutex.Lock()
	defer kvs.Mutex.Unlock()

	err := kvs.processLock.Unlock()
	if err != nil {
		return errors.Wrap(err, "unlock error")
	}

	if kvs.logger != nil {
		kvs.logger.Info("Released process lock")
	} else {
		log.Printf("Released process lock")
	}

	return nil
}

// GetModificationTime returns the modification time of the persistent store.
func (kvs *jsonFileStore) GetModificationTime() (time.Time, error) {
	kvs.Mutex.Lock()
	defer kvs.Mutex.Unlock()

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

func (kvs *jsonFileStore) Remove() {
	kvs.Mutex.Lock()
	if err := os.Remove(kvs.fileName); err != nil {
		log.Errorf("could not remove file %s. Error: %v", kvs.fileName, err)
	}
	kvs.Mutex.Unlock()
}
