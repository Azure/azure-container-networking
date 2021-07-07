// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package store

import (
	"fmt"
	"time"
)

// KeyValueStore represents a persistent store of (key,value) pairs.
type KeyValueStore interface {
	Read(key string, value interface{}) error
	Write(key string, value interface{}) error
	Flush() error
	Lock(block bool) error
	Unlock(forceUnlock bool) error
	GetModificationTime() (time.Time, error)
	GetLockFileModificationTime() (time.Time, error)
	GetLockFileName() string
	Remove()
}

var (
	// Errors returned by KeyValueStore methods.
	ErrKeyNotFound                    = fmt.Errorf("key not found")
	ErrStoreLocked                    = fmt.Errorf("store is already locked")
	ErrStoreNotLocked                 = fmt.Errorf("store is not locked")
	ErrStoreEmpty                     = fmt.Errorf("store is empty")
	ErrTimeoutLockingStore            = fmt.Errorf("timed out locking store")
	ErrNonBlockingLockIsAlreadyLocked = fmt.Errorf("attempted to perform non-blocking lock on an already locked store")
)
