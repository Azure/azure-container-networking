package store

import (
	"time"
)

type mockStore struct {
	lockFilePath string
}

// NewMockStore creates a new jsonFileStore object, accessed as a KeyValueStore.
func NewMockStore(lockFilePath string) KeyValueStore {
	return &mockStore{
		lockFilePath: lockFilePath,
	}
}

// Read restores the value for the given key from persistent store.
func (ms *mockStore) Read(key string, value interface{}) error {
	return nil
}

func (ms *mockStore) Write(key string, value interface{}) error {
	return nil
}

func (ms *mockStore) Flush() error {
	return nil
}

func (ms *mockStore) Lock(block bool) error {
	return nil
}

func (ms *mockStore) Unlock(forceUnlock bool) error {
	return nil
}

func (ms *mockStore) GetModificationTime() (time.Time, error) {
	return time.Time{}, nil
}

func (ms *mockStore) GetLockFileModificationTime() (time.Time, error) {
	return time.Time{}, nil
}

func (ms *mockStore) GetLockFileName() string {
	return ms.lockFilePath
}

func (ms *mockStore) Remove() {}
