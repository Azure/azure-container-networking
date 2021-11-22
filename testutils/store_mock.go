//go:build !ignore_uncovered
// +build !ignore_uncovered

package testutils

import (
	"time"

	"github.com/Azure/azure-container-networking/store"
)

var _ store.KeyValueStore = (*KeyValueStoreMock)(nil)

type KeyValueStoreMock struct {
	ExistsBool               bool
	ReadError                error
	WriteError               error
	FlushError               error
	LockError                error
	UnlockError              error
	ModificationTime         time.Time
	GetModificationTimeError error
}

func (mockst *KeyValueStoreMock) Exists() bool {
	return mockst.ExistsBool
}

func (mockst *KeyValueStoreMock) Read(key string, value interface{}) error {
	return mockst.ReadError
}

func (mockst *KeyValueStoreMock) Write(key string, value interface{}) error {
	return mockst.WriteError
}

func (mockst *KeyValueStoreMock) Flush() error {
	return mockst.FlushError
}

func (mockst *KeyValueStoreMock) Lock(time.Duration) error {
	return mockst.LockError
}

func (mockst *KeyValueStoreMock) Unlock() error {
	return mockst.UnlockError
}

func (mockst *KeyValueStoreMock) GetModificationTime() (time.Time, error) {
	if mockst.GetModificationTimeError != nil {
		return time.Time{}, mockst.GetModificationTimeError
	}
	return mockst.ModificationTime, nil
}

func (mockst *KeyValueStoreMock) Remove() {}
