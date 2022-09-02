package store

import (
	"encoding/json"
	"fmt"
	"time"
)

type mockStore struct {
	lockFilePath string
	data         map[string]*json.RawMessage
}

// NewMockStore creates a new jsonFileStore object, accessed as a KeyValueStore.
func NewMockStore(lockFilePath string) KeyValueStore {
	return &mockStore{
		lockFilePath: lockFilePath,
		data:         make(map[string]*json.RawMessage),
	}
}

func (ms *mockStore) Exists() bool {
	return ms.data == nil
}

// Read restores the value for the given key from persistent store.
func (ms *mockStore) Read(key string, value interface{}) error {
	if _, ok := ms.data[key]; !ok {
		return ErrStoreEmpty
	}
	err := json.Unmarshal(*ms.data[key], value)
	if err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

func (ms *mockStore) Write(key string, value interface{}) error {
	var raw json.RawMessage
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	ms.data[key] = &raw
	return nil
}

func (ms *mockStore) Flush() error {
	return nil
}

func (ms *mockStore) Lock(duration time.Duration) error {
	return nil
}

func (ms *mockStore) Unlock() error {
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
