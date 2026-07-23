// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/processlock"
	"github.com/Azure/azure-container-networking/store"
	"github.com/stretchr/testify/require"
)

type trackedFileLock struct {
	unlockCalls int
	unlockErr   error
}

func (*trackedFileLock) Lock() error {
	return nil
}

func (l *trackedFileLock) Unlock() error {
	l.unlockCalls++
	return l.unlockErr
}

func TestNewPersistentStateStartup_GatesStartOnFailure(t *testing.T) {
	failureErr := errors.New("startup failure")

	tests := []struct {
		name              string
		failDirectoryCall int
		failLockCall      int
		failStoreCall     int
		wantUnlocks       int
	}{
		{
			name:              "state directory",
			failDirectoryCall: 1,
		},
		{
			name:         "state lock",
			failLockCall: 1,
		},
		{
			name:          "state store",
			failStoreCall: 1,
			wantUnlocks:   1,
		},
		{
			name:         "endpoint lock",
			failLockCall: 2,
			wantUnlocks:  1,
		},
		{
			name:              "endpoint directory",
			failDirectoryCall: 2,
			wantUnlocks:       2,
		},
		{
			name:          "endpoint store",
			failStoreCall: 2,
			wantUnlocks:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				directoryCalls int
				lockCalls      int
				storeCalls     int
				locks          []*trackedFileLock
				startCalled    bool
			)

			startup, err := newPersistentStateStartup(
				testPersistentStatePaths(),
				true,
				func(context.Context) error {
					startCalled = true
					return nil
				},
				persistentStateDependencies{
					createDirectory: func(string) error {
						directoryCalls++
						if directoryCalls == tt.failDirectoryCall {
							return failureErr
						}
						return nil
					},
					newFileLock: func(string) (processlock.Interface, error) {
						lockCalls++
						if lockCalls == tt.failLockCall {
							return nil, failureErr
						}
						lock := &trackedFileLock{}
						locks = append(locks, lock)
						return lock, nil
					},
					openStore: func(path string, _ processlock.Interface) (store.KeyValueStore, error) {
						storeCalls++
						if storeCalls == tt.failStoreCall {
							return nil, failureErr
						}
						return store.NewMockStore(path), nil
					},
				},
			)

			require.ErrorIs(t, err, failureErr)
			require.Nil(t, startup)
			require.False(t, startCalled)

			var unlocks int
			for _, lock := range locks {
				unlocks += lock.unlockCalls
			}
			require.Equal(t, tt.wantUnlocks, unlocks)
		})
	}
}

func TestPersistentStateStartup_StartPropagatesContextAndCleansUpFailure(t *testing.T) {
	startErr := errors.New("listener failure")
	cleanupErr := errors.New("cleanup failure")
	stateLock := &trackedFileLock{}
	endpointLock := &trackedFileLock{unlockErr: cleanupErr}
	locks := []processlock.Interface{stateLock, endpointLock}
	lockIndex := 0

	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "startup-context")
	var receivedCtx context.Context

	startup, err := newPersistentStateStartup(
		testPersistentStatePaths(),
		true,
		func(ctx context.Context) error {
			receivedCtx = ctx
			return startErr
		},
		persistentStateDependencies{
			createDirectory: func(string) error { return nil },
			newFileLock: func(string) (processlock.Interface, error) {
				lock := locks[lockIndex]
				lockIndex++
				return lock, nil
			},
			openStore: func(path string, _ processlock.Interface) (store.KeyValueStore, error) {
				return store.NewMockStore(path), nil
			},
		},
	)
	require.NoError(t, err)

	err = startup.Start(ctx)
	require.ErrorIs(t, err, startErr)
	require.ErrorIs(t, err, cleanupErr)
	require.Same(t, ctx, receivedCtx)
	require.Equal(t, 1, stateLock.unlockCalls)
	require.Equal(t, 1, endpointLock.unlockCalls)

	require.ErrorIs(t, startup.Close(), cleanupErr)
	require.Equal(t, 1, stateLock.unlockCalls)
	require.Equal(t, 1, endpointLock.unlockCalls)
}

func TestNewPersistentStateStartup_EndpointStateDisabled(t *testing.T) {
	var (
		directories []string
		lockFiles   []string
		storeFiles  []string
		lock        trackedFileLock
	)

	startup, err := newPersistentStateStartup(
		testPersistentStatePaths(),
		false,
		func(context.Context) error { return nil },
		persistentStateDependencies{
			createDirectory: func(path string) error {
				directories = append(directories, path)
				return nil
			},
			newFileLock: func(path string) (processlock.Interface, error) {
				lockFiles = append(lockFiles, path)
				return &lock, nil
			},
			openStore: func(path string, _ processlock.Interface) (store.KeyValueStore, error) {
				storeFiles = append(storeFiles, path)
				return store.NewMockStore(path), nil
			},
		},
	)
	require.NoError(t, err)
	require.NotNil(t, startup.stateStore)
	require.Nil(t, startup.endpointStateStore)
	require.Equal(t, []string{"state-dir"}, directories)
	require.Equal(t, []string{"state.lock"}, lockFiles)
	require.Equal(t, []string{"state.json"}, storeFiles)
	require.NoError(t, startup.Start(context.Background()))
	require.NoError(t, startup.Close())
	require.Equal(t, 1, lock.unlockCalls)
}

func TestJSONPersistentStateStartup(t *testing.T) {
	baseDir, err := os.MkdirTemp(".", ".persistent-state-test-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(baseDir))
	})

	stateDirectory := filepath.Join(baseDir, "state") + string(os.PathSeparator)
	endpointDirectory := filepath.Join(baseDir, "endpoint") + string(os.PathSeparator)
	stateLockFile := filepath.Join(baseDir, "locks", "state.lock")
	endpointLockFile := filepath.Join(baseDir, "locks", "endpoint.lock")
	paths := persistentStatePaths{
		stateDirectory:    stateDirectory,
		stateFile:         stateDirectory + "azure-cns.json",
		stateLockFile:     stateLockFile,
		endpointDirectory: endpointDirectory,
		endpointFile:      endpointDirectory + "azure-endpoints.json",
		endpointLockFile:  endpointLockFile,
	}

	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "startup-context")
	var startup *persistentStateStartup
	startup, err = newJSONPersistentStateStartup(paths, true, func(receivedCtx context.Context) error {
		require.Same(t, ctx, receivedCtx)

		require.NoError(t, startup.stateStore.Lock(time.Second))
		require.NoError(t, startup.stateStore.Write("state", map[string]string{"backend": "json"}))
		require.NoError(t, startup.endpointStateStore.Lock(time.Second))
		require.NoError(t, startup.endpointStateStore.Write("endpoint", "10.0.0.4"))
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, startup.Start(ctx))
	require.NoError(t, startup.Close())

	for _, file := range []string{paths.stateFile, paths.endpointFile} {
		contents, readErr := os.ReadFile(file)
		require.NoError(t, readErr)
		require.True(t, json.Valid(contents), "invalid JSON in %s", file)
		require.Equal(t, ".json", filepath.Ext(file))
	}

	for _, lockFile := range []string{stateLockFile, endpointLockFile} {
		lock, lockErr := processlock.NewFileLock(lockFile)
		require.NoError(t, lockErr)
		require.NoError(t, lock.Lock())
		require.NoError(t, lock.Unlock())
	}
}

func testPersistentStatePaths() persistentStatePaths {
	return persistentStatePaths{
		stateDirectory:    "state-dir",
		stateFile:         "state.json",
		stateLockFile:     "state.lock",
		endpointDirectory: "endpoint-dir",
		endpointFile:      "endpoint.json",
		endpointLockFile:  "endpoint.lock",
	}
}
