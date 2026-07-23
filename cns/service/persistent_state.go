// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package main

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/processlock"
	"github.com/Azure/azure-container-networking/store"
)

type persistentStatePaths struct {
	stateDirectory    string
	stateFile         string
	stateLockFile     string
	endpointDirectory string
	endpointFile      string
	endpointLockFile  string
}

type persistentStateDependencies struct {
	createDirectory func(string) error
	newFileLock     func(string) (processlock.Interface, error)
	openStore       func(string, processlock.Interface) (store.KeyValueStore, error)
}

type persistentStateStartup struct {
	stateStore         store.KeyValueStore
	endpointStateStore store.KeyValueStore
	start              func(context.Context) error
	locks              []processlock.Interface
	closeOnce          sync.Once
	closeErr           error
}

func newPersistentStateStartup(
	paths persistentStatePaths,
	manageEndpointState bool,
	start func(context.Context) error,
	deps persistentStateDependencies,
) (*persistentStateStartup, error) {
	startup := &persistentStateStartup{start: start}

	if err := deps.createDirectory(paths.stateDirectory); err != nil {
		return nil, fmt.Errorf("creating state store directory: %w", err)
	}

	stateLock, err := deps.newFileLock(paths.stateLockFile)
	if err != nil {
		return nil, fmt.Errorf("creating state store lock: %w", err)
	}
	startup.locks = append(startup.locks, stateLock)

	startup.stateStore, err = deps.openStore(paths.stateFile, stateLock)
	if err != nil {
		return nil, startup.closeAfterError(fmt.Errorf("opening state store: %w", err))
	}

	if !manageEndpointState {
		return startup, nil
	}

	endpointLock, err := deps.newFileLock(paths.endpointLockFile)
	if err != nil {
		return nil, startup.closeAfterError(fmt.Errorf("creating endpoint state store lock: %w", err))
	}
	startup.locks = append(startup.locks, endpointLock)

	if err := deps.createDirectory(paths.endpointDirectory); err != nil {
		return nil, startup.closeAfterError(fmt.Errorf("creating endpoint state store directory: %w", err))
	}

	startup.endpointStateStore, err = deps.openStore(paths.endpointFile, endpointLock)
	if err != nil {
		return nil, startup.closeAfterError(fmt.Errorf("opening endpoint state store: %w", err))
	}

	return startup, nil
}

func newJSONPersistentStateStartup(
	paths persistentStatePaths,
	manageEndpointState bool,
	start func(context.Context) error,
) (*persistentStateStartup, error) {
	return newPersistentStateStartup(paths, manageEndpointState, start, persistentStateDependencies{
		createDirectory: platform.CreateDirectory,
		newFileLock:     processlock.NewFileLock,
		openStore: func(path string, lock processlock.Interface) (store.KeyValueStore, error) {
			return store.NewJsonFileStore(path, lock, nil)
		},
	})
}

func (s *persistentStateStartup) Start(ctx context.Context) error {
	if err := s.start(ctx); err != nil {
		return errors.Join(err, s.Close())
	}
	return nil
}

func (s *persistentStateStartup) Close() error {
	s.closeOnce.Do(func() {
		var closeErrs []error
		for _, lock := range s.locks {
			if err := lock.Unlock(); err != nil && !errors.Is(err, processlock.ErrInvalidFile) {
				closeErrs = append(closeErrs, err)
			}
		}
		s.closeErr = errors.Join(closeErrs...)
	})
	return s.closeErr
}

func (s *persistentStateStartup) closeAfterError(err error) error {
	return errors.Join(err, s.Close())
}
