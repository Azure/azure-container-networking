// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/cns/state"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/processlock"
	"github.com/Azure/azure-container-networking/store"
)

type boltStartupMode uint8

const (
	boltStartupNormal boltStartupMode = iota
	boltStartupRollback
)

type boltPersistentStateConfig struct {
	paths               persistentStatePaths
	mode                boltStartupMode
	manageEndpointState bool
	bootPolicy          state.BootPolicy
	options             state.Options
}

type boltPersistentStateDependencies struct {
	createDirectory func(string) error
	newFileLock     func(string) (processlock.Interface, error)
	openStore       func(string, processlock.Interface) (store.KeyValueStore, error)
	openDB          func(context.Context, string, state.Options) (*state.DB, error)
	currentBootID   func() (string, error)
	attachBolt      func(*state.DB) (persistentStateAttachment, error)
	restoreJSON     func(context.Context, store.KeyValueStore, store.KeyValueStore) error
}

func productionBoltPersistentStateDependencies(
	service *restserver.HTTPRestService,
	restoreJSON func(context.Context, store.KeyValueStore, store.KeyValueStore) error,
) boltPersistentStateDependencies {
	return boltPersistentStateDependencies{
		createDirectory: platform.CreateDirectory,
		newFileLock:     processlock.NewFileLock,
		openStore: func(path string, lock processlock.Interface) (store.KeyValueStore, error) {
			return store.NewJsonFileStore(path, lock, nil)
		},
		openDB:        state.OpenContext,
		currentBootID: platform.BootID,
		attachBolt: func(db *state.DB) (persistentStateAttachment, error) {
			restore, close, err := restserver.NewDurableStateLifecycle(service, db)
			if err != nil {
				return persistentStateAttachment{}, err
			}
			return persistentStateAttachment{restore: restore, close: close}, nil
		},
		restoreJSON: restoreJSON,
	}
}

func newBoltPersistentStateStartup(
	ctx context.Context,
	config boltPersistentStateConfig,
	start func(context.Context) error,
	deps boltPersistentStateDependencies,
) (*persistentStateStartup, error) {
	if err := validateBoltStartup(config, start, deps); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("initializing Bolt persistent state: %w", err)
	}

	startup := &persistentStateStartup{start: start}
	if err := deps.createDirectory(config.paths.stateDirectory); err != nil {
		return nil, fmt.Errorf("creating state store directory: %w", err)
	}

	stateLock, err := acquireLegacyLock(ctx, config.paths.stateLockFile, deps.newFileLock)
	if err != nil {
		return nil, fmt.Errorf("acquiring state store lock: %w", err)
	}
	startup.locks = append(startup.locks, stateLock)

	endpointLock, lockErr := acquireLegacyLock(ctx, config.paths.endpointLockFile, deps.newFileLock)
	if lockErr != nil {
		return nil, startup.closeAfterError(fmt.Errorf("acquiring endpoint state store lock: %w", lockErr))
	}
	startup.locks = append(startup.locks, endpointLock)

	db, err := deps.openDB(ctx, config.paths.databaseFile, config.options)
	if err != nil {
		return nil, startup.closeAfterError(err)
	}
	closeDBAfterError := func(startupErr error) error {
		return errors.Join(startupErr, db.Close(), startup.Close())
	}
	if err := ctx.Err(); err != nil {
		return nil, closeDBAfterError(fmt.Errorf("opening Bolt persistent state: %w", err))
	}
	status, err := db.Status(ctx)
	if err != nil {
		return nil, closeDBAfterError(err)
	}
	if status.InvariantStatus != state.InvariantHealthy {
		return nil, closeDBAfterError(fmt.Errorf("persistent state invariant %q failed", status.FailedInvariant))
	}

	if config.mode == boltStartupRollback {
		if _, err := db.ExportLegacy(ctx, state.ExportOptions{
			CNSJSONPath:      config.paths.stateFile,
			EndpointJSONPath: config.paths.endpointFile,
		}); err != nil {
			return nil, closeDBAfterError(err)
		}
		status, err := db.Status(ctx)
		if err != nil {
			return nil, closeDBAfterError(err)
		}
		if status.Authority != state.AuthorityJSON || !status.RollbackExported {
			return nil, closeDBAfterError(errors.New("rollback export did not establish JSON authority"))
		}
		if err := db.Close(); err != nil {
			return nil, errors.Join(err, startup.Close())
		}
		if err := initializeRollbackJSON(ctx, startup, config, deps); err != nil {
			return nil, startup.closeAfterError(err)
		}
		return startup, nil
	}

	bootID, err := deps.currentBootID()
	if err != nil {
		return nil, closeDBAfterError(fmt.Errorf("getting current boot ID: %w", err))
	}
	importOptions := state.ImportOptions{
		CNSPath:             config.paths.stateFile,
		EndpointPath:        config.paths.endpointFile,
		ManageEndpointState: config.manageEndpointState,
		BootID:              bootID,
	}
	switch status.Authority {
	case state.AuthorityBolt:
		if _, err := db.ImportLegacy(ctx, importOptions); err != nil {
			return nil, closeDBAfterError(err)
		}
	case state.AuthorityJSON:
		if _, err := db.ReimportLegacy(ctx, importOptions, config.bootPolicy); err != nil {
			return nil, closeDBAfterError(err)
		}
	default:
		return nil, closeDBAfterError(fmt.Errorf("unsupported persistent state authority %q", status.Authority))
	}
	if _, err := db.ApplyBoot(ctx, bootID, config.bootPolicy); err != nil {
		return nil, closeDBAfterError(err)
	}
	if _, err := db.RefreshMetrics(ctx); err != nil {
		return nil, closeDBAfterError(err)
	}
	attachment, err := deps.attachBolt(db)
	if err != nil {
		return nil, closeDBAfterError(fmt.Errorf("attaching Bolt persistent state: %w", err))
	}
	startup.attachments = append(startup.attachments, attachment)
	return startup, nil
}

func initializeRollbackJSON(
	ctx context.Context,
	startup *persistentStateStartup,
	config boltPersistentStateConfig,
	deps boltPersistentStateDependencies,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("initializing rollback JSON state: %w", err)
	}
	if err := deps.createDirectory(config.paths.endpointDirectory); err != nil {
		return fmt.Errorf("creating endpoint state store directory: %w", err)
	}
	var err error
	startup.stateStore, err = deps.openStore(config.paths.stateFile, startup.locks[0])
	if err != nil {
		return fmt.Errorf("opening rollback state store: %w", err)
	}
	if config.manageEndpointState {
		startup.endpointStateStore, err = deps.openStore(config.paths.endpointFile, startup.locks[1])
		if err != nil {
			return fmt.Errorf("opening rollback endpoint state store: %w", err)
		}
	}
	return startup.attach(
		func(restoreCtx context.Context) error {
			return deps.restoreJSON(restoreCtx, startup.stateStore, startup.endpointStateStore)
		},
		func() error { return nil },
	)
}

func acquireLegacyLock(
	ctx context.Context,
	path string,
	newLock func(string) (processlock.Interface, error),
) (processlock.Interface, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lock, err := newLock(path)
	if err != nil {
		return nil, err
	}
	if err := lock.Lock(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(err, lock.Unlock())
	}
	return lock, nil
}

func validateBoltStartup(
	config boltPersistentStateConfig,
	start func(context.Context) error,
	deps boltPersistentStateDependencies,
) error {
	switch {
	case start == nil:
		return errors.New("persistent state start callback is nil")
	case config.mode != boltStartupNormal && config.mode != boltStartupRollback:
		return errors.New("persistent state startup mode is invalid")
	case config.paths.stateDirectory == "":
		return errors.New("persistent state directory is empty")
	case config.paths.databaseFile == "":
		return errors.New("persistent state database path is empty")
	case config.paths.stateFile == "":
		return errors.New("legacy CNS state path is empty")
	case config.paths.stateLockFile == "":
		return errors.New("legacy CNS lock path is empty")
	case config.paths.endpointFile == "":
		return errors.New("legacy endpoint state path is empty")
	case config.paths.endpointLockFile == "":
		return errors.New("legacy endpoint lock path is empty")
	case deps.createDirectory == nil:
		return errors.New("persistent state directory creator is nil")
	case deps.newFileLock == nil:
		return errors.New("persistent state lock factory is nil")
	case deps.openDB == nil:
		return errors.New("persistent state database opener is nil")
	case config.mode == boltStartupNormal && deps.currentBootID == nil:
		return errors.New("persistent state boot ID provider is nil")
	case config.mode == boltStartupNormal && deps.attachBolt == nil:
		return errors.New("persistent state Bolt attachment is nil")
	case config.mode == boltStartupRollback && deps.openStore == nil:
		return errors.New("persistent state JSON opener is nil")
	case config.mode == boltStartupRollback && deps.restoreJSON == nil:
		return errors.New("persistent state JSON restore callback is nil")
	}
	return nil
}
