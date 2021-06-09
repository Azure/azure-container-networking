package testutils

import (
	"time"
)

type KeyValueStoreMock struct {
	ReadError                error
	WriteError               error
	FlushError               error
	LockError                error
	UnlockError              error
	ModificationTime         time.Time
	GetModificationTimeError error
}

func (store *KeyValueStoreMock) Read(key string, value interface{}) error {
	return store.ReadError
}

func (store *KeyValueStoreMock) Write(key string, value interface{}) error {
	return store.WriteError
}
func (store *KeyValueStoreMock) Flush() error {
	return store.FlushError
}
func (store *KeyValueStoreMock) Lock(block bool) error {
	return store.LockError
}
func (store *KeyValueStoreMock) Unlock(forceUnlock bool) error {
	return store.UnlockError
}

func (store *KeyValueStoreMock) GetModificationTime() (time.Time, error) {
	if store.GetModificationTimeError != nil {
		return time.Time{}, store.GetModificationTimeError
	} else {
		return store.ModificationTime, nil
	}
}

func (store *KeyValueStoreMock) GetLockFileModificationTime() (time.Time, error) {
	return time.Now(), nil
}

func (store *KeyValueStoreMock) GetLockFileName() string {
	return ""
}

func (store *KeyValueStoreMock) Remove() {
	return
}
