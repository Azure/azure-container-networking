// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package main

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/processlock"
	"github.com/Azure/azure-container-networking/store"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
)

func migrateStateStores(
	mode string,
	mainStore store.KeyValueStore,
	mainJSONPath string,
	mainJSONLock processlock.Interface,
	endpointStore store.KeyValueStore,
	endpointJSONPath string,
	endpointJSONLock processlock.Interface,
	manageEndpointState bool,
) error {
	if mainStore == nil {
		return errors.New("main state store is nil")
	}

	normalizedMode := strings.ToLower(strings.TrimSpace(mode))
	switch normalizedMode {
	case "":
		return nil
	case configuration.StateStoreMigrationModeJSONToBolt:
		jsonStore, err := store.NewJsonFileStore(mainJSONPath, mainJSONLock, nil)
		if err != nil {
			return errors.Wrap(err, "failed to create json store for migration")
		}

		if _, err := migrateKeyIfNeeded(jsonStore, mainStore, restserver.StoreKey, "cns-state"); err != nil {
			return err
		}

		if manageEndpointState && endpointStore != nil {
			missing, err := endpointStateMissing(endpointStore)
			if err != nil {
				return err
			}
			if missing {
				endpointJSONStore, err := store.NewJsonFileStore(endpointJSONPath, endpointJSONLock, nil)
				if err != nil {
					return errors.Wrap(err, "failed to create endpoint json store for migration")
				}
				if _, err := migrateKeyIfNeeded(endpointJSONStore, endpointStore, restserver.EndpointStoreKey, "endpoint-state"); err != nil {
					return err
				}
			}
		}
	case configuration.StateStoreMigrationModeBoltToJSON:
		jsonStore, err := store.NewJsonFileStore(mainJSONPath, mainJSONLock, nil)
		if err != nil {
			return errors.Wrap(err, "failed to create json store for migration")
		}

		if _, err := migrateKeyIfNeeded(mainStore, jsonStore, restserver.StoreKey, "cns-state"); err != nil {
			return err
		}

		if manageEndpointState && endpointStore != nil {
			endpointJSONStore, err := store.NewJsonFileStore(endpointJSONPath, endpointJSONLock, nil)
			if err != nil {
				return errors.Wrap(err, "failed to create endpoint json store for migration")
			}
			if _, err := migrateKeyIfNeeded(endpointStore, endpointJSONStore, restserver.EndpointStoreKey, "endpoint-state"); err != nil {
				return err
			}
		}
	default:
		logger.Printf("[Azure CNS] Unknown StateStoreMigrationMode: %s", mode)
	}

	return nil
}

func migrateKeyIfNeeded(source store.KeyValueStore, destination store.KeyValueStore, key, label string) (bool, error) {
	var destinationRaw json.RawMessage
	if err := destination.Read(key, &destinationRaw); err == nil {
		logger.Printf("[Azure CNS] %s already present in destination store; skipping migration", label)
		return false, nil
	} else if err != store.ErrKeyNotFound && err != store.ErrStoreEmpty {
		return false, errors.Wrapf(err, "failed to read destination key %s", label)
	}

	var sourceRaw json.RawMessage
	if err := source.Read(key, &sourceRaw); err != nil {
		if err == store.ErrKeyNotFound || err == store.ErrStoreEmpty {
			logger.Printf("[Azure CNS] %s not found in source store; skipping migration", label)
			return false, nil
		}
		return false, errors.Wrapf(err, "failed to read source key %s", label)
	}

	if err := destination.Write(key, sourceRaw); err != nil {
		return false, errors.Wrapf(err, "failed to write destination key %s", label)
	}

	var verifyRaw json.RawMessage
	if err := destination.Read(key, &verifyRaw); err != nil {
		return false, errors.Wrapf(err, "failed to validate destination key %s", label)
	}

	if !bytes.Equal(sourceRaw, verifyRaw) {
		var sourceValue interface{}
		if err := json.Unmarshal(sourceRaw, &sourceValue); err != nil {
			return false, errors.Wrapf(err, "failed to parse source key %s", label)
		}
		var verifyValue interface{}
		if err := json.Unmarshal(verifyRaw, &verifyValue); err != nil {
			return false, errors.Wrapf(err, "failed to parse destination key %s", label)
		}
		if !cmp.Equal(sourceValue, verifyValue) {
			return false, errors.Errorf("validation failed for %s: data mismatch", label)
		}
	}

	logger.Printf("[Azure CNS] Migrated %s to destination store", label)
	return true, nil
}

func endpointStateMissing(endpointStore store.KeyValueStore) (bool, error) {
	if endpointStore == nil {
		return true, nil
	}

	var endpointState map[string]*restserver.EndpointInfo
	err := endpointStore.Read(restserver.EndpointStoreKey, &endpointState)
	if err == nil {
		return false, nil
	} else if err == store.ErrKeyNotFound || err == store.ErrStoreEmpty {
		return true, nil
	}

	return false, errors.Wrap(err, "failed to read endpoint state")
}
