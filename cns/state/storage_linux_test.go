// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorageMetadataAfterDatabaseFileRemoval(t *testing.T) {
	db, path := openTestDB(t)
	require.NoError(t, os.Remove(path))

	metadata, err := db.StorageMetadata()
	require.NoError(t, err)
	assert.Equal(t, StorageBackendBolt, metadata.Backend)
	assert.False(t, metadata.FilePresent)
	assert.Zero(t, metadata.FileSizeBytes)
}
