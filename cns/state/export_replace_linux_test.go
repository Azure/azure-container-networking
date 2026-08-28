// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type faultSyncDirectory struct {
	syncErr  error
	closeErr error
	closed   bool
}

func (f *faultSyncDirectory) Sync() error {
	return f.syncErr
}

func (f *faultSyncDirectory) Close() error {
	f.closed = true
	return f.closeErr
}

func TestDurableReplaceLinux(t *testing.T) {
	t.Run("real replace and permissions", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		require.NoError(t, os.WriteFile(source, []byte("new"), 0o600))
		require.NoError(t, os.WriteFile(destination, []byte("old"), 0o400))
		require.NoError(t, durableReplace(source, destination))
		data, err := os.ReadFile(destination)
		require.NoError(t, err)
		assert.Equal(t, []byte("new"), data)
		_, err = os.Stat(source)
		require.ErrorIs(t, err, os.ErrNotExist)
		info, err := os.Stat(destination)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})

	t.Run("replace failure does not open parent", func(t *testing.T) {
		var opened bool
		err := durableReplaceWith(
			"source",
			"destination",
			func(string, string) error { return errRollbackFault },
			func(string) (syncDirectory, error) {
				opened = true
				return nil, nil
			},
		)
		require.ErrorIs(t, err, errRollbackFault)
		assert.False(t, opened)
	})

	t.Run("parent open sync and close failures", func(t *testing.T) {
		tests := []struct {
			name       string
			openErr    error
			syncErr    error
			closeErr   error
			wantClosed bool
		}{
			{name: "open", openErr: errRollbackFault},
			{name: "sync", syncErr: errRollbackFault, wantClosed: true},
			{name: "close", closeErr: errRollbackFault, wantClosed: true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				directory := &faultSyncDirectory{syncErr: tt.syncErr, closeErr: tt.closeErr}
				err := durableReplaceWith(
					"source",
					filepath.Join("parent", "destination"),
					func(string, string) error { return nil },
					func(path string) (syncDirectory, error) {
						assert.Equal(t, "parent", path)
						return directory, tt.openErr
					},
				)
				require.ErrorIs(t, err, errRollbackFault)
				assert.Equal(t, tt.wantClosed, directory.closed)
			})
		}
	})
}

func TestExportLegacyInjectedPermissionFailure(t *testing.T) {
	db, _ := openPopulatedExportDB(t)
	opts := exportPaths(t)
	files := osRollbackFileOperations()
	files.mkdirAll = func(string, os.FileMode) error { return os.ErrPermission }
	changed, err := db.exportLegacy(context.Background(), opts, files)
	require.ErrorIs(t, err, os.ErrPermission)
	assert.False(t, changed)
	assertRollbackDestinationsMissing(t, opts)
	assertNoRollbackTemps(t, opts)
	assert.Equal(t, AuthorityBolt, readMetadata(t, db).Authority)
}

func TestExportLegacyReadOnlyParent(t *testing.T) {
	db, _ := openPopulatedExportDB(t)
	parent := filepath.Join(t.TempDir(), "read-only")
	require.NoError(t, os.Mkdir(parent, 0o500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(parent, 0o700)) })

	probe := filepath.Join(parent, "probe")
	if err := os.WriteFile(probe, []byte("probe"), 0o600); err == nil {
		require.NoError(t, os.Remove(probe))
		t.Skip("filesystem permissions are not enforced for this user")
	}

	opts := ExportOptions{
		CNSJSONPath:      filepath.Join(parent, "cns", "azure-cns.json"),
		EndpointJSONPath: filepath.Join(t.TempDir(), "azure-endpoints.json"),
	}
	changed, err := db.ExportLegacy(context.Background(), opts)
	require.Error(t, err)
	assert.False(t, changed)
	assertRollbackDestinationsMissing(t, opts)
	assertNoRollbackTemps(t, opts)
	assert.Equal(t, AuthorityBolt, readMetadata(t, db).Authority)
}
