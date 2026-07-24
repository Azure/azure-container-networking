// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package main

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/cns/state"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/processlock"
	"github.com/Azure/azure-container-networking/store"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	errPersistentInvariantFailed    = errors.New("persistent state invariant failed")
	errRollbackJSONAuthorityMissing = errors.New("rollback export did not establish JSON authority")
	errUnsupportedStateAuthority    = errors.New("unsupported persistent state authority")
	errNilStateStartCallback        = errors.New("persistent state start callback is nil")
	errInvalidStateStartupMode      = errors.New("persistent state startup mode is invalid")
	errEmptyStateDirectory          = errors.New("persistent state directory is empty")
	errEmptyStateDatabasePath       = errors.New("persistent state database path is empty")
	errEmptyLegacyCNSPath           = errors.New("legacy CNS state path is empty")
	errEmptyLegacyCNSLockPath       = errors.New("legacy CNS lock path is empty")
	errEmptyLegacyEndpointPath      = errors.New("legacy endpoint state path is empty")
	errEmptyLegacyEndpointLockPath  = errors.New("legacy endpoint lock path is empty")
	errNilStateDirectoryCreator     = errors.New("persistent state directory creator is nil")
	errNilStateLockFactory          = errors.New("persistent state lock factory is nil")
	errNilStateDatabaseOpener       = errors.New("persistent state database opener is nil")
	errNilStateBootIDProvider       = errors.New("persistent state boot ID provider is nil")
	errNilStateBoltAttachment       = errors.New("persistent state Bolt attachment is nil")
	errNilStateJSONOpener           = errors.New("persistent state JSON opener is nil")
	errNilStateJSONRestoreCallback  = errors.New("persistent state JSON restore callback is nil")
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
	attachBolt      func(*state.DB, bool) (persistentStateAttachment, error)
	restoreJSON     func(context.Context, store.KeyValueStore, store.KeyValueStore) error
}

var (
	productionPersistentStateMetricsOnce sync.Once
	productionPersistentStateMetrics     *state.Metrics
	productionPersistentStateMetricsErr  error
)

func getProductionPersistentStateMetrics() (*state.Metrics, error) {
	productionPersistentStateMetricsOnce.Do(func() {
		productionPersistentStateMetrics, productionPersistentStateMetricsErr = state.NewMetrics(prometheus.DefaultRegisterer)
	})
	return productionPersistentStateMetrics, productionPersistentStateMetricsErr
}

func productionBoltPersistentStateDependencies(
	service *restserver.HTTPRestService,
	cniState cns.CNIEndpointStateProvider,
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
		attachBolt: func(db *state.DB, projectEndpointState bool) (persistentStateAttachment, error) {
			restore, closeFn, err := restserver.NewDurableStateLifecycleWithCNIImport(
				service,
				db,
				projectEndpointState,
				cniState,
			)
			if err != nil {
				return persistentStateAttachment{}, fmt.Errorf("creating durable state lifecycle: %w", err)
			}
			return persistentStateAttachment{restore: restore, close: closeFn}, nil
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
	if validationErr := validateBoltStartup(config, start, deps); validationErr != nil {
		return nil, validationErr
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, fmt.Errorf("initializing Bolt persistent state: %w", contextErr)
	}

	startup := &persistentStateStartup{start: start}
	if directoryErr := deps.createDirectory(config.paths.stateDirectory); directoryErr != nil {
		return nil, fmt.Errorf("creating state store directory: %w", directoryErr)
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

	db, openErr := deps.openDB(ctx, config.paths.databaseFile, config.options)
	if openErr != nil {
		return nil, startup.closeAfterError(openErr)
	}
	closeDBAfterError := func(startupErr error) error {
		return errors.Join(startupErr, db.Close(), startup.Close())
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, closeDBAfterError(fmt.Errorf("opening Bolt persistent state: %w", contextErr))
	}
	status, statusErr := db.Status(ctx)
	if statusErr != nil {
		return nil, closeDBAfterError(statusErr)
	}
	if status.InvariantStatus != state.InvariantHealthy {
		return nil, closeDBAfterError(fmt.Errorf("%w: %q", errPersistentInvariantFailed, status.FailedInvariant))
	}

	if config.mode == boltStartupRollback {
		if _, exportErr := db.ExportLegacy(ctx, state.ExportOptions{
			CNSJSONPath:      config.paths.stateFile,
			EndpointJSONPath: config.paths.endpointFile,
		}); exportErr != nil {
			return nil, closeDBAfterError(exportErr)
		}
		rollbackStatus, rollbackStatusErr := db.Status(ctx)
		if rollbackStatusErr != nil {
			return nil, closeDBAfterError(rollbackStatusErr)
		}
		if rollbackStatus.Authority != state.AuthorityJSON || !rollbackStatus.RollbackExported {
			return nil, closeDBAfterError(errRollbackJSONAuthorityMissing)
		}
		if closeErr := db.Close(); closeErr != nil {
			return nil, errors.Join(closeErr, startup.Close())
		}
		if initializeErr := initializeRollbackJSON(ctx, startup, config, deps); initializeErr != nil {
			return nil, startup.closeAfterError(initializeErr)
		}
		return startup, nil
	}

	bootID, bootIDErr := deps.currentBootID()
	if bootIDErr != nil {
		return nil, closeDBAfterError(fmt.Errorf("getting current boot ID: %w", bootIDErr))
	}
	importOptions := state.ImportOptions{
		CNSPath:             config.paths.stateFile,
		EndpointPath:        config.paths.endpointFile,
		ManageEndpointState: config.manageEndpointState,
		BootID:              bootID,
	}
	switch status.Authority {
	case state.AuthorityBolt:
		if _, importErr := db.ImportLegacy(ctx, importOptions); importErr != nil {
			return nil, closeDBAfterError(importErr)
		}
	case state.AuthorityJSON:
		if _, reimportErr := db.ReimportLegacy(ctx, importOptions, config.bootPolicy); reimportErr != nil {
			return nil, closeDBAfterError(reimportErr)
		}
	default:
		return nil, closeDBAfterError(fmt.Errorf("%w: %q", errUnsupportedStateAuthority, status.Authority))
	}
	if _, bootErr := db.ApplyBoot(ctx, bootID, config.bootPolicy); bootErr != nil {
		return nil, closeDBAfterError(bootErr)
	}
	attachment, attachmentErr := deps.attachBolt(db, config.manageEndpointState)
	if attachmentErr != nil {
		return nil, closeDBAfterError(fmt.Errorf("attaching Bolt persistent state: %w", attachmentErr))
	}
	startup.attachments = append(startup.attachments, attachment)
	startup.status = db.Status
	startup.snapshot = db.Snapshot
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
		return nil, fmt.Errorf("acquiring legacy state lock: %w", err)
	}
	lock, err := newLock(path)
	if err != nil {
		return nil, fmt.Errorf("creating legacy state lock: %w", err)
	}
	if lockErr := lock.Lock(); lockErr != nil {
		return nil, fmt.Errorf("locking legacy state: %w", lockErr)
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, errors.Join(contextErr, lock.Unlock())
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
		return errNilStateStartCallback
	case config.mode != boltStartupNormal && config.mode != boltStartupRollback:
		return errInvalidStateStartupMode
	case config.paths.stateDirectory == "":
		return errEmptyStateDirectory
	case config.paths.databaseFile == "":
		return errEmptyStateDatabasePath
	case config.paths.stateFile == "":
		return errEmptyLegacyCNSPath
	case config.paths.stateLockFile == "":
		return errEmptyLegacyCNSLockPath
	case config.paths.endpointFile == "":
		return errEmptyLegacyEndpointPath
	case config.paths.endpointLockFile == "":
		return errEmptyLegacyEndpointLockPath
	case deps.createDirectory == nil:
		return errNilStateDirectoryCreator
	case deps.newFileLock == nil:
		return errNilStateLockFactory
	case deps.openDB == nil:
		return errNilStateDatabaseOpener
	case config.mode == boltStartupNormal && deps.currentBootID == nil:
		return errNilStateBootIDProvider
	case config.mode == boltStartupNormal && deps.attachBolt == nil:
		return errNilStateBoltAttachment
	case config.mode == boltStartupRollback && deps.openStore == nil:
		return errNilStateJSONOpener
	case config.mode == boltStartupRollback && deps.restoreJSON == nil:
		return errNilStateJSONRestoreCallback
	}
	return nil
}
