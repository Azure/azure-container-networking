// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns/state"
	"github.com/Azure/azure-container-networking/processlock"
	"github.com/Azure/azure-container-networking/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errBoltBootProvider = errors.New("boot provider failure")
	errBoltAdapter      = errors.New("adapter failure")
	errBoltOpen         = errors.New("open failure")
	errBoltProjection   = errors.New("projection failure")
	errBoltLegacyLock   = errors.New("legacy lock failure")
)

const startupEndpointLockFile = "endpoint.lock"

type startupTestLock struct {
	name        string
	events      *[]string
	lockCalls   int
	unlockCalls int
	lockErr     error
}

func TestProductionBoltPersistentStateDependencies(t *testing.T) {
	deps := productionBoltPersistentStateDependencies(
		nil,
		func(context.Context, store.KeyValueStore, store.KeyValueStore) error { return nil },
	)
	assert.NotNil(t, deps.createDirectory)
	assert.NotNil(t, deps.newFileLock)
	assert.NotNil(t, deps.openStore)
	assert.NotNil(t, deps.openDB)
	assert.NotNil(t, deps.currentBootID)
	assert.NotNil(t, deps.attachBolt)
	assert.NotNil(t, deps.restoreJSON)
}

func (l *startupTestLock) Lock() error {
	l.lockCalls++
	*l.events = append(*l.events, "lock:"+l.name)
	return l.lockErr
}

func (l *startupTestLock) Unlock() error {
	l.unlockCalls++
	*l.events = append(*l.events, "unlock:"+l.name)
	return nil
}

func TestBoltPersistentStateStartupFirstImportAndRestart(t *testing.T) {
	paths := writeStartupLegacyState(t, "node-before-migration")
	var events []string
	deps, locks, restoreCount, closeCount := startupTestDependencies(t, &events)
	listenerCount := 0
	config := boltPersistentStateConfig{
		paths:               paths,
		mode:                boltStartupNormal,
		manageEndpointState: true,
		options:             state.Options{Timeout: 50 * time.Millisecond},
	}

	startup, err := newBoltPersistentStateStartup(context.Background(), config, func(context.Context) error {
		listenerCount++
		events = append(events, "listener")
		return nil
	}, deps)
	require.NoError(t, err)
	require.Zero(t, listenerCount)
	require.NoError(t, startup.Start(context.Background()))
	require.Equal(t, 1, listenerCount)
	require.Equal(t, 1, *restoreCount)
	require.Equal(t, []string{"lock:state", "lock:endpoint", "restore", "listener"}, events)
	require.NoError(t, startup.Close())
	require.Equal(t, 1, *closeCount)
	require.Equal(t, 1, (*locks)[0].unlockCalls)
	require.Equal(t, 1, (*locks)[1].unlockCalls)
	assert.Equal(t, "unlock:endpoint", events[len(events)-2])
	assert.Equal(t, "unlock:state", events[len(events)-1])

	db, err := state.Open(paths.databaseFile, state.Options{})
	require.NoError(t, err)
	snapshot, err := db.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, state.AuthorityBolt, snapshot.Metadata.Authority)
	assert.True(t, snapshot.Metadata.LegacyImportComplete)
	assert.Equal(t, "boot-1", snapshot.Metadata.BootID)
	assert.Equal(t, "node-before-migration", snapshot.Metadata.NodeID)
	require.NoError(t, db.Close())

	require.NoError(t, os.WriteFile(paths.stateFile, []byte(`not JSON`), 0o600))
	require.NoError(t, os.Remove(paths.endpointFile))
	events = nil
	deps, _, restoreCount, closeCount = startupTestDependencies(t, &events)
	restarted, err := newBoltPersistentStateStartup(context.Background(), config, func(context.Context) error {
		listenerCount++
		return nil
	}, deps)
	require.NoError(t, err)
	require.NoError(t, restarted.Start(context.Background()))
	require.Equal(t, 2, listenerCount)
	require.Equal(t, 1, *restoreCount)
	require.NoError(t, restarted.Close())
	require.Equal(t, 1, *closeCount)
	db, err = state.Open(paths.databaseFile, state.Options{})
	require.NoError(t, err)
	restartedSnapshot, err := db.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, snapshot.Metadata.Generation, restartedSnapshot.Metadata.Generation)
	assert.Equal(t, snapshot.Metadata.BootID, restartedSnapshot.Metadata.BootID)
	require.NoError(t, db.Close())

	events = nil
	deps, _, _, _ = startupTestDependencies(t, &events)
	deps.currentBootID = func() (string, error) { return "boot-2", nil }
	newBoot, err := newBoltPersistentStateStartup(context.Background(), config, func(context.Context) error {
		listenerCount++
		return nil
	}, deps)
	require.NoError(t, err)
	require.NoError(t, newBoot.Start(context.Background()))
	require.NoError(t, newBoot.Close())
	db, err = state.Open(paths.databaseFile, state.Options{})
	require.NoError(t, err)
	newBootSnapshot, err := db.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "boot-2", newBootSnapshot.Metadata.BootID)
	assert.Equal(t, restartedSnapshot.Metadata.Generation+1, newBootSnapshot.Metadata.Generation)
	require.NoError(t, db.Close())
}

func TestBoltPersistentStateStartupRollbackAndReupgrade(t *testing.T) {
	paths := writeStartupLegacyState(t, "node-before-rollback")
	var events []string
	deps, _, _, _ := startupTestDependencies(t, &events)
	config := boltPersistentStateConfig{
		paths:               paths,
		mode:                boltStartupNormal,
		manageEndpointState: true,
		options:             state.Options{Timeout: 50 * time.Millisecond},
	}
	startup, err := newBoltPersistentStateStartup(
		context.Background(),
		config,
		func(context.Context) error { return nil },
		deps,
	)
	require.NoError(t, err)
	require.NoError(t, startup.Start(context.Background()))
	require.NoError(t, startup.Close())

	events = nil
	deps, _, _, _ = startupTestDependencies(t, &events)
	var restoredNode string
	deps.restoreJSON = func(_ context.Context, stateStore, endpointStore store.KeyValueStore) error {
		var cnsState struct {
			NodeID string
		}
		if readErr := stateStore.Read("ContainerNetworkService", &cnsState); readErr != nil {
			return fmt.Errorf("reading rollback CNS state: %w", readErr)
		}
		var endpoints map[string]any
		if readErr := endpointStore.Read("Endpoints", &endpoints); readErr != nil {
			return fmt.Errorf("reading rollback endpoint state: %w", readErr)
		}
		restoredNode = cnsState.NodeID
		events = append(events, "json-restore")
		return nil
	}
	config.mode = boltStartupRollback
	listenerCount := 0
	rollback, err := newBoltPersistentStateStartup(context.Background(), config, func(context.Context) error {
		listenerCount++
		events = append(events, "listener")
		return nil
	}, deps)
	require.NoError(t, err)
	require.Zero(t, listenerCount)
	require.NotNil(t, rollback.stateStore)
	require.NotNil(t, rollback.endpointStateStore)
	require.NoError(t, rollback.Start(context.Background()))
	require.Equal(t, 1, listenerCount)
	require.Equal(t, "node-before-rollback", restoredNode)
	require.Equal(t, []string{"lock:state", "lock:endpoint", "json-restore", "listener"}, events)
	require.NoError(t, rollback.Close())

	mutateStartupNodeID(t, paths.stateFile, "node-after-rollback")
	events = nil
	deps, _, restoreCount, closeCount := startupTestDependencies(t, &events)
	deps.currentBootID = func() (string, error) { return "boot-2", nil }
	config.mode = boltStartupNormal
	reupgrade, err := newBoltPersistentStateStartup(context.Background(), config, func(context.Context) error {
		listenerCount++
		return nil
	}, deps)
	require.NoError(t, err)
	require.NoError(t, reupgrade.Start(context.Background()))
	require.Equal(t, 2, listenerCount)
	require.Equal(t, 1, *restoreCount)
	require.NoError(t, reupgrade.Close())
	require.Equal(t, 1, *closeCount)

	db, err := state.Open(paths.databaseFile, state.Options{})
	require.NoError(t, err)
	defer db.Close()
	snapshot, err := db.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, state.AuthorityBolt, snapshot.Metadata.Authority)
	assert.False(t, snapshot.Metadata.RollbackExportComplete)
	assert.True(t, snapshot.Metadata.LegacyImportComplete)
	assert.Equal(t, "boot-2", snapshot.Metadata.BootID)
	assert.Equal(t, "node-after-rollback", snapshot.Metadata.NodeID)
}

func TestBoltPersistentStateStartupFailuresGateListenerAndReleaseLocks(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*boltPersistentStateDependencies, *boltPersistentStateConfig)
		want   error
	}{
		{
			name: "open",
			mutate: func(deps *boltPersistentStateDependencies, _ *boltPersistentStateConfig) {
				deps.openDB = func(context.Context, string, state.Options) (*state.DB, error) {
					return nil, errBoltOpen
				}
			},
			want: errBoltOpen,
		},
		{
			name: "boot identity",
			mutate: func(deps *boltPersistentStateDependencies, _ *boltPersistentStateConfig) {
				deps.currentBootID = func() (string, error) { return "", errBoltBootProvider }
			},
			want: errBoltBootProvider,
		},
		{
			name: "adapter",
			mutate: func(deps *boltPersistentStateDependencies, _ *boltPersistentStateConfig) {
				deps.attachBolt = func(*state.DB) (persistentStateAttachment, error) {
					return persistentStateAttachment{}, errBoltAdapter
				}
			},
			want: errBoltAdapter,
		},
		{
			name: "malformed import",
			mutate: func(_ *boltPersistentStateDependencies, config *boltPersistentStateConfig) {
				require.NoError(t, os.WriteFile(config.paths.stateFile, []byte(`not JSON`), 0o600))
			},
			want: state.ErrLegacyImportSource,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := writeStartupLegacyState(t, "node-1")
			var events []string
			deps, locks, _, _ := startupTestDependencies(t, &events)
			config := boltPersistentStateConfig{
				paths:               paths,
				mode:                boltStartupNormal,
				manageEndpointState: true,
				options:             state.Options{Timeout: 50 * time.Millisecond},
			}
			tt.mutate(&deps, &config)
			listenerCount := 0
			startup, err := newBoltPersistentStateStartup(context.Background(), config, func(context.Context) error {
				listenerCount++
				return nil
			}, deps)
			require.ErrorIs(t, err, tt.want)
			require.Nil(t, startup)
			require.Zero(t, listenerCount)
			for _, lock := range *locks {
				require.Equal(t, 1, lock.unlockCalls)
			}
			if !errors.Is(tt.want, errBoltOpen) {
				db, reopenErr := state.Open(paths.databaseFile, state.Options{Timeout: 50 * time.Millisecond})
				require.NoError(t, reopenErr)
				require.NoError(t, db.Close())
			}
		})
	}
}

func TestBoltPersistentStateStartupRestoreFailureClosesDatabase(t *testing.T) {
	paths := writeStartupLegacyState(t, "node-1")
	var events []string
	deps, locks, _, closeCount := startupTestDependencies(t, &events)
	deps.attachBolt = func(db *state.DB) (persistentStateAttachment, error) {
		return persistentStateAttachment{
			restore: func(context.Context) error { return errBoltProjection },
			close: func() error {
				*closeCount++
				return db.Close()
			},
		}, nil
	}
	listenerCount := 0
	startup, err := newBoltPersistentStateStartup(context.Background(), boltPersistentStateConfig{
		paths:               paths,
		mode:                boltStartupNormal,
		manageEndpointState: true,
	}, func(context.Context) error {
		listenerCount++
		return nil
	}, deps)
	require.NoError(t, err)
	err = startup.Start(context.Background())
	require.ErrorIs(t, err, errBoltProjection)
	require.Zero(t, listenerCount)
	require.Equal(t, 1, *closeCount)
	for _, lock := range *locks {
		require.Equal(t, 1, lock.unlockCalls)
	}
	reopened, err := state.Open(paths.databaseFile, state.Options{Timeout: 50 * time.Millisecond})
	require.NoError(t, err)
	require.NoError(t, reopened.Close())
}

func TestBoltPersistentStateStartupLockFailures(t *testing.T) {
	for _, tt := range []struct {
		name     string
		failCall int
	}{
		{name: "state lock", failCall: 1},
		{name: "endpoint lock", failCall: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			paths := writeStartupLegacyState(t, "node-1")
			var events []string
			deps, locks, _, _ := startupTestDependencies(t, &events)
			lockCalls := 0
			deps.newFileLock = func(path string) (processlock.Interface, error) {
				lockCalls++
				name := "state"
				if filepath.Base(path) == startupEndpointLockFile {
					name = "endpoint"
				}
				lock := &startupTestLock{name: name, events: &events}
				if lockCalls == tt.failCall {
					lock.lockErr = errBoltLegacyLock
				}
				*locks = append(*locks, lock)
				return lock, nil
			}
			listenerCount := 0
			startup, err := newBoltPersistentStateStartup(context.Background(), boltPersistentStateConfig{
				paths:               paths,
				mode:                boltStartupNormal,
				manageEndpointState: true,
			}, func(context.Context) error {
				listenerCount++
				return nil
			}, deps)
			require.ErrorIs(t, err, errBoltLegacyLock)
			require.Nil(t, startup)
			require.Zero(t, listenerCount)
			require.NoFileExists(t, paths.databaseFile)
			if tt.failCall == 2 {
				require.Equal(t, 1, (*locks)[0].unlockCalls)
			}
			require.Equal(t, 0, (*locks)[tt.failCall-1].unlockCalls)
		})
	}
}

func TestBoltPersistentStateStartupCancellationAndDBLockTimeout(t *testing.T) {
	t.Run("canceled before startup", func(t *testing.T) {
		paths := writeStartupLegacyState(t, "node-1")
		var events []string
		deps, locks, _, _ := startupTestDependencies(t, &events)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		listenerCount := 0
		startup, err := newBoltPersistentStateStartup(ctx, boltPersistentStateConfig{
			paths:               paths,
			mode:                boltStartupNormal,
			manageEndpointState: true,
		}, func(context.Context) error {
			listenerCount++
			return nil
		}, deps)
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, startup)
		require.Zero(t, listenerCount)
		require.Empty(t, *locks)
	})

	t.Run("database locked", func(t *testing.T) {
		paths := writeStartupLegacyState(t, "node-1")
		held, err := state.Open(paths.databaseFile, state.Options{})
		require.NoError(t, err)
		defer held.Close()
		var events []string
		deps, locks, _, _ := startupTestDependencies(t, &events)
		listenerCount := 0
		started := time.Now()
		startup, err := newBoltPersistentStateStartup(context.Background(), boltPersistentStateConfig{
			paths:               paths,
			mode:                boltStartupNormal,
			manageEndpointState: true,
			options:             state.Options{Timeout: 30 * time.Millisecond},
		}, func(context.Context) error {
			listenerCount++
			return nil
		}, deps)
		require.Error(t, err)
		require.Nil(t, startup)
		require.Zero(t, listenerCount)
		assert.Less(t, time.Since(started), time.Second)
		for _, lock := range *locks {
			require.Equal(t, 1, lock.unlockCalls)
		}
	})

	t.Run("canceled while opening database", func(t *testing.T) {
		paths := writeStartupLegacyState(t, "node-1")
		var events []string
		deps, locks, _, _ := startupTestDependencies(t, &events)
		deps.openDB = func(ctx context.Context, _ string, _ state.Options) (*state.DB, error) {
			<-ctx.Done()
			return nil, fmt.Errorf("waiting for startup cancellation: %w", ctx.Err())
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		listenerCount := 0
		startup, err := newBoltPersistentStateStartup(ctx, boltPersistentStateConfig{
			paths:               paths,
			mode:                boltStartupNormal,
			manageEndpointState: true,
		}, func(context.Context) error {
			listenerCount++
			return nil
		}, deps)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Nil(t, startup)
		require.Zero(t, listenerCount)
		for _, lock := range *locks {
			require.Equal(t, 1, lock.unlockCalls)
		}
	})
}

func startupTestDependencies(
	t *testing.T,
	events *[]string,
) (
	deps boltPersistentStateDependencies,
	locksOut *[]*startupTestLock,
	restoreCountOut *int,
	closeCountOut *int,
) {
	t.Helper()
	var locks []*startupTestLock
	restoreCount := 0
	closeCount := 0
	deps = boltPersistentStateDependencies{
		createDirectory: func(path string) error { return os.MkdirAll(path, 0o755) },
		newFileLock: func(path string) (processlock.Interface, error) {
			name := "state"
			if filepath.Base(path) == startupEndpointLockFile {
				name = "endpoint"
			}
			lock := &startupTestLock{name: name, events: events}
			locks = append(locks, lock)
			return lock, nil
		},
		openStore: func(path string, lock processlock.Interface) (store.KeyValueStore, error) {
			return store.NewJsonFileStore(path, lock, nil)
		},
		openDB:        state.OpenContext,
		currentBootID: func() (string, error) { return "boot-1", nil },
		attachBolt: func(db *state.DB) (persistentStateAttachment, error) {
			return persistentStateAttachment{
				restore: func(ctx context.Context) error {
					restoreCount++
					if _, err := db.Snapshot(ctx); err != nil {
						return fmt.Errorf("reading startup test snapshot: %w", err)
					}
					*events = append(*events, "restore")
					return nil
				},
				close: func() error {
					closeCount++
					return db.Close()
				},
			}, nil
		},
		restoreJSON: func(context.Context, store.KeyValueStore, store.KeyValueStore) error { return nil },
	}
	return deps, &locks, &restoreCount, &closeCount
}

func writeStartupLegacyState(t *testing.T, nodeID string) persistentStatePaths {
	t.Helper()
	base := t.TempDir()
	stateDirectory := filepath.Join(base, "state")
	endpointDirectory := filepath.Join(base, "endpoint")
	require.NoError(t, os.MkdirAll(stateDirectory, 0o755))
	require.NoError(t, os.MkdirAll(endpointDirectory, 0o755))
	paths := persistentStatePaths{
		stateDirectory:    stateDirectory,
		stateFile:         filepath.Join(stateDirectory, "azure-cns.json"),
		databaseFile:      filepath.Join(stateDirectory, "azure-cns.db"),
		stateLockFile:     filepath.Join(base, "state.lock"),
		endpointDirectory: endpointDirectory,
		endpointFile:      filepath.Join(endpointDirectory, "azure-endpoints.json"),
		endpointLockFile:  filepath.Join(base, startupEndpointLockFile),
	}
	cnsData, err := json.Marshal(map[string]any{
		"ContainerNetworkService": map[string]any{
			"Location":                         "eastus",
			"NetworkType":                      "azure",
			"OrchestratorType":                 "KubernetesCRD",
			"NodeID":                           nodeID,
			"Initialized":                      true,
			"ContainerIDByOrchestratorContext": map[string]any{},
			"ContainerStatus":                  map[string]any{},
			"Networks":                         map[string]any{},
			"TimeStamp":                        time.Now().UTC(),
			"PnpIDByMacAddress":                map[string]any{},
		},
	})
	require.NoError(t, err)
	endpointData, err := json.Marshal(map[string]any{
		"Endpoints":     map[string]any{},
		"DeleteIntents": map[string]any{},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(paths.stateFile, cnsData, 0o600))
	require.NoError(t, os.WriteFile(paths.endpointFile, endpointData, 0o600))
	return paths
}

func mutateStartupNodeID(t *testing.T, path, nodeID string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(data, &envelope))
	envelope["ContainerNetworkService"].(map[string]any)["NodeID"] = nodeID
	data, err = json.Marshal(envelope)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}
