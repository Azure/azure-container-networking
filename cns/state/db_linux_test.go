// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// assertDatabaseFileMode verifies that the database file was created with
// the exact 0o600 permissions passed to bolt.Open.
func assertDatabaseFileMode(t *testing.T, info os.FileInfo) {
	t.Helper()
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}
