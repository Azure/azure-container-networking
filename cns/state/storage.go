// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"errors"
	"fmt"
	"os"
)

type StorageBackend string

const StorageBackendBolt StorageBackend = "bolt"

type StorageMetadata struct {
	Backend       StorageBackend `json:"backend"`
	FilePresent   bool           `json:"filePresent"`
	FileSizeBytes int64          `json:"fileSizeBytes"`
}

func (s *DB) StorageMetadata() (StorageMetadata, error) {
	metadata := StorageMetadata{Backend: StorageBackendBolt}
	info, err := os.Stat(s.db.Path())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return metadata, nil
		}
		return StorageMetadata{}, fmt.Errorf("stat cns state database: %w", err)
	}
	metadata.FilePresent = true
	metadata.FileSizeBytes = info.Size()
	return metadata, nil
}
