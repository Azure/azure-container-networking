// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// assertDatabaseFileMode verifies the database file is writable on Windows.
// Windows only tracks a read-only attribute, so Go maps the owner-write bit
// (0o200) requested by bolt.Open to "not read-only", which os.FileInfo.Mode
// reports as 0o666 regardless of the other Unix-style bits passed in.
func assertDatabaseFileMode(t *testing.T, info os.FileInfo) {
	t.Helper()
	assert.Equal(t, os.FileMode(0o666), info.Mode().Perm())
}
