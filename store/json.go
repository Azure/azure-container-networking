// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package store

import (
	"encoding/json"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/log"
)

const (
	// Default file name for backing persistent store.
	defaultFileName = "azure-container-networking.json"

	// Extension added to the file name for lock.
	lockExtension = ".lock"

	// Maximum number of retries before failing a lock call.
	lockMaxRetries = 200

	// Delay between lock retries.
	lockRetryDelay = 100 * time.Millisecond
)

// jsonFileStore is an implementation of KeyValueStore using a local JSON file.
type jsonFileStore struct {
	fileName string
	data     map[string]*json.RawMessage
	inSync   bool
	locked   bool
	sync.Mutex
}

// NewJsonFileStore creates a new jsonFileStore object, accessed as a KeyValueStore.
func NewJsonFileStore(fileName string) (KeyValueStore, error) {
	if fileName == "" {
		fileName = defaultFileName
	}

	kvs := &jsonFileStore{
		fileName: fileName,
		data:     make(map[string]*json.RawMessage),
	}

	return kvs, nil
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

		// Decode to raw JSON messages.
		if err := json.NewDecoder(file).Decode(&kvs.data); err != nil {
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
	file, err := os.Create(kvs.fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	buf, err := json.MarshalIndent(&kvs.data, "", "\t")
	if err != nil {
		return err
	}

	if _, err := file.Write(buf); err != nil {
		return err
	}
	return nil
}

// Lock locks the store for exclusive access.
func (kvs *jsonFileStore) Lock(block bool) error {
	kvs.Mutex.Lock()
	defer kvs.Mutex.Unlock()

	if kvs.locked {
		return ErrStoreLocked
	}

	var lockFile *os.File
	var err error
	lockName := kvs.fileName + lockExtension
	lockPerm := os.FileMode(0664) + os.FileMode(os.ModeExclusive)

	// Try to acquire the lock file.
	var lockRetryCount uint
	var modTimeCur time.Time
	var modTimePrev time.Time
	for lockRetryCount < lockMaxRetries {
		lockFile, err = os.OpenFile(lockName, os.O_CREATE|os.O_EXCL|os.O_RDWR, lockPerm)
		if err == nil {
			break
		}

		if !block {
			return ErrNonBlockingLockIsAlreadyLocked
		}

		// Reset the lock retry count if the timestamp for the lock file changes.
		if fileInfo, err := os.Stat(lockName); err == nil {
			modTimeCur = fileInfo.ModTime()
			if !modTimeCur.Equal(modTimePrev) {
				lockRetryCount = 0
			}
			modTimePrev = modTimeCur
		}

		time.Sleep(lockRetryDelay)

		lockRetryCount++
	}

	if lockRetryCount == lockMaxRetries {
		return ErrTimeoutLockingStore
	}

	defer lockFile.Close()

	// Write the process ID for easy identification.
	if _, err = lockFile.WriteString(strconv.Itoa(os.Getpid())); err != nil {
		return err
	}

	kvs.locked = true

	return nil
}

// Unlock unlocks the store.
func (kvs *jsonFileStore) Unlock(forceUnlock bool) error {
	kvs.Mutex.Lock()
	defer kvs.Mutex.Unlock()

	if !forceUnlock && !kvs.locked {
		return ErrStoreNotLocked
	}

	err := os.Remove(kvs.fileName + lockExtension)
	if err != nil {
		return err
	}

	kvs.inSync = false
	kvs.locked = false

	return nil
}

// GetModificationTime returns the modification time of the persistent store.
func (kvs *jsonFileStore) GetModificationTime() (time.Time, error) {
	kvs.Mutex.Lock()
	defer kvs.Mutex.Unlock()

	info, err := os.Stat(kvs.fileName)
	if err != nil {
		log.Printf("os.stat() for file %v failed: %v", kvs.fileName, err)
		return time.Time{}.UTC(), err
	}

	return info.ModTime().UTC(), nil
}

// GetLockFileModificationTime returns the modification time of the lock file of the persistent store.
func (kvs *jsonFileStore) GetLockFileModificationTime() (time.Time, error) {
	kvs.Mutex.Lock()
	defer kvs.Mutex.Unlock()

	lockFileName := kvs.fileName + lockExtension

	// Check if the file exists.
	file, err := os.Open(lockFileName)
	if err != nil {
		return time.Time{}.UTC(), err
	}

	defer file.Close()

	info, err := os.Stat(lockFileName)
	if err != nil {
		log.Printf("os.stat() for file %v failed: %v", lockFileName, err)
		return time.Time{}.UTC(), err
	}

	return info.ModTime().UTC(), nil
}
