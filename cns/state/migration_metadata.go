// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"fmt"

	bolt "go.etcd.io/bbolt"
)

const (
	legacyImportMarker   = "complete"
	rollbackExportMarker = "complete"
)

func legacyImportComplete(metadata *bolt.Bucket) (bool, error) {
	if metadata == nil {
		return false, corrupt(fmt.Sprintf("missing bucket %q", bucketMetadata), nil)
	}
	value := metadata.Get(metaKeyLegacyImport)
	if value == nil {
		return false, nil
	}
	if string(value) != legacyImportMarker {
		return false, corrupt("invalid legacy import marker", nil)
	}
	return true, nil
}

func rollbackExportComplete(metadata *bolt.Bucket) (bool, error) {
	if metadata == nil {
		return false, corrupt(fmt.Sprintf("missing bucket %q", bucketMetadata), nil)
	}
	value := metadata.Get(metaKeyRollbackExport)
	if value == nil {
		return false, nil
	}
	if string(value) != rollbackExportMarker {
		return false, corrupt("invalid rollback export marker", nil)
	}
	return true, nil
}

func validateMigrationMetadata(metadata *bolt.Bucket) error {
	if _, err := legacyImportComplete(metadata); err != nil {
		return err
	}
	rollbackComplete, err := rollbackExportComplete(metadata)
	if err != nil {
		return err
	}
	return validateRollbackExportState(Authority(metadata.Get(metaKeyAuthority)), rollbackComplete)
}

func validateRollbackExportState(authority Authority, complete bool) error {
	switch {
	case authority == AuthorityBolt && !complete:
		return nil
	case authority == AuthorityJSON && complete:
		return nil
	case authority == AuthorityBolt:
		return corrupt("rollback marker is complete while Bolt is authoritative", nil)
	case authority == AuthorityJSON:
		return corrupt("JSON is authoritative without a rollback marker", nil)
	default:
		return corrupt(fmt.Sprintf("invalid rollback authority %q", authority), nil)
	}
}
