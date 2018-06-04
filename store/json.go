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
	lockMaxRetries = 20

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
				return nil
			}
			return err
		}

		// Decode to raw JSON messages.
		err = json.NewDecoder(file).Decode(&kvs.data)
		if err != nil {
			return err
		}

		err = file.Close()
		if err != nil {
			return err
		}

		kvs.inSync = true
	}

	raw := kvs.data[key]
	if raw == nil {
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

	buf, err := json.MarshalIndent(&kvs.data, "", "\t")
	if err != nil {
		return err
	}

	_, err = file.Write(buf)
	if err != nil {
		return err
	}

	return file.Close()
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
	for i := 0; ; i++ {
		lockFile, err = os.OpenFile(lockName, os.O_CREATE|os.O_EXCL|os.O_RDWR, lockPerm)
		if err == nil {
			break
		}

		if !block || i == lockMaxRetries {
			return ErrStoreLocked
		}

		time.Sleep(lockRetryDelay)
	}

	// Write the process ID for easy identification.
	_, err = lockFile.WriteString(strconv.Itoa(os.Getpid()))
	if err != nil {
		return err
	}

	err = lockFile.Close()
	if err != nil {
		return err
	}

	kvs.locked = true

	return nil
}

// Unlock unlocks the store.
func (kvs *jsonFileStore) Unlock() error {
	kvs.Mutex.Lock()
	defer kvs.Mutex.Unlock()

	if !kvs.locked {
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
	info, err := os.Stat(kvs.fileName)
	if err != nil {
		log.Printf("os.stat() for file %v failed with error %v", kvs.fileName, err)
		return time.Time{}.UTC(), err
	}

	return info.ModTime().UTC(), nil
}
