package common

import (
	"sync"

	"github.com/Azure/azure-container-networking/log"
)

// NamedLock holds a mutex and a map of locks. Mutex is used to
// get exclusive lock on the map while initializing the lock in the
// map.
type NamedLock struct {
	mutex   sync.Mutex
	lockMap map[string]*refCountedLock
}

// refCountedLock holds the lock and ref count for it
type refCountedLock struct {
	mutex    sync.RWMutex
	refCount int
}

// InitNamedLock initializes the named lock struct
func InitNamedLock() *NamedLock {
	return &NamedLock{
		mutex:   sync.Mutex{},
		lockMap: make(map[string]*refCountedLock),
	}
}

// LockAcquire acquires the lock with specified name
func (namedLock *NamedLock) LockAcquire(lockName string) {
	namedLock.mutex.Lock()
	lock, ok := namedLock.lockMap[lockName]
	if !ok {
		lock = &refCountedLock{refCount: 0}
		namedLock.lockMap[lockName] = lock
	}

	lock.AddRef()
	namedLock.mutex.Unlock()
	// The ref count keeps this entry in the map, so lock it outside the map mutex.
	lock.Lock()
}

// LockRelease releases the lock with specified name
func (namedLock *NamedLock) LockRelease(lockName string) {
	namedLock.mutex.Lock()
	defer namedLock.mutex.Unlock()

	lock, ok := namedLock.lockMap[lockName]
	if ok {
		lock.Unlock()
		lock.RemoveRef()
		if lock.refCount == 0 {
			delete(namedLock.lockMap, lockName)
		}
	} else {
		log.Printf("[Azure CNS] Attempt to unlock: %s without acquiring the lock", lockName)
	}
}

// AddRef increments the ref count on the lock
func (refCountedLock *refCountedLock) AddRef() {
	refCountedLock.refCount++
}

// RemoveRef decrements the ref count on the lock
func (refCountedLock *refCountedLock) RemoveRef() {
	refCountedLock.refCount--
}

// Lock locks the named lock
func (refCountedLock *refCountedLock) Lock() {
	refCountedLock.mutex.Lock()
}

// Unlock unlocks the named lock
func (refCountedLock *refCountedLock) Unlock() {
	refCountedLock.mutex.Unlock()
}
