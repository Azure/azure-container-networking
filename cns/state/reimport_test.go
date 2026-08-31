// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var errInjectedReimport = errors.New("injected reimport failure")

const reimportTestNewBoot = "new-boot"

func TestReimportLegacyReplacesRolledBackState(t *testing.T) {
	ctx := context.Background()
	db, _ := openPopulatedExportDB(t)
	metrics, err := NewMetrics(prometheus.NewRegistry())
	require.NoError(t, err)
	db.metrics = metrics
	db.logger = zap.NewNop()
	before := requireValidSnapshot(t, db)
	paths := exportPaths(t)
	changed, err := db.ExportLegacy(ctx, paths)
	require.NoError(t, err)
	require.True(t, changed)

	cnsData := mutateLegacyCNS(t, readRollbackFile(t, paths.CNSJSONPath), func(source map[string]any) {
		source["NodeID"] = "node-after-rollback"
	})
	endpointData := mutateLegacyEndpoints(t, readRollbackFile(t, paths.EndpointJSONPath), func(endpoints map[string]any) {
		endpoints["container-1"].(map[string]any)["PodName"] = "pod-after-rollback"
	})
	require.NoError(t, os.WriteFile(paths.CNSJSONPath, cnsData, 0o600))
	require.NoError(t, os.WriteFile(paths.EndpointJSONPath, endpointData, 0o600))

	changed, err = db.ReimportLegacy(ctx, ImportOptions{
		CNSPath:             paths.CNSJSONPath,
		EndpointPath:        paths.EndpointJSONPath,
		ManageEndpointState: true,
		BootID:              "boot-after-rollback",
	}, BootPolicy{ResetReadiness: true})
	require.NoError(t, err)
	require.True(t, changed)

	after := requireValidSnapshot(t, db)
	assert.Equal(t, AuthorityBolt, after.Metadata.Authority)
	assert.True(t, after.Metadata.LegacyImportComplete)
	assert.False(t, after.Metadata.RollbackExportComplete)
	assert.Equal(t, "boot-after-rollback", after.Metadata.BootID)
	assert.Equal(t, before.Metadata.Generation+2, after.Metadata.Generation)
	assert.Equal(t, "node-after-rollback", after.Metadata.NodeID)
	assert.Equal(t, "pod-after-rollback", after.Endpoints["container-1"].PodName)
	assert.Empty(t, after.DeleteIntents)
	for _, record := range after.NetworkContainers {
		assert.Empty(t, record.HostVersion)
		assert.False(t, record.VFPUpdateComplete)
	}

	path := db.db.Path()
	require.NoError(t, db.Close())
	reopened, err := Open(path, Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	restarted := requireValidSnapshot(t, reopened)
	assert.Equal(t, after, restarted)
}

func TestReimportLegacyClearsEndpointsByBootPolicy(t *testing.T) {
	db, _ := openPopulatedExportDB(t)
	paths := exportPaths(t)
	changed, err := db.ExportLegacy(context.Background(), paths)
	require.NoError(t, err)
	require.True(t, changed)

	changed, err = db.ReimportLegacy(context.Background(), ImportOptions{
		CNSPath:             paths.CNSJSONPath,
		EndpointPath:        paths.EndpointJSONPath,
		ManageEndpointState: true,
		BootID:              reimportTestNewBoot,
	}, BootPolicy{ClearEndpoints: true})
	require.NoError(t, err)
	require.True(t, changed)

	snapshot := requireValidSnapshot(t, db)
	assert.Empty(t, snapshot.Endpoints)
	assert.Empty(t, snapshot.Assignments)
	assert.Empty(t, snapshot.IPOwners)
}

func TestReimportLegacyFailuresAreAtomic(t *testing.T) {
	t.Run("malformed source", func(t *testing.T) {
		db, _ := openPopulatedExportDB(t)
		paths := exportPaths(t)
		changed, err := db.ExportLegacy(context.Background(), paths)
		require.NoError(t, err)
		require.True(t, changed)
		before := requireValidSnapshot(t, db)
		require.NoError(t, os.WriteFile(paths.EndpointJSONPath, []byte(`not JSON`), 0o600))

		changed, err = db.ReimportLegacy(context.Background(), ImportOptions{
			CNSPath:             paths.CNSJSONPath,
			EndpointPath:        paths.EndpointJSONPath,
			ManageEndpointState: true,
			BootID:              reimportTestNewBoot,
		}, BootPolicy{})
		require.ErrorIs(t, err, ErrLegacyImportSource)
		assert.False(t, changed)
		assert.Equal(t, before, requireValidSnapshot(t, db))
	})

	t.Run("commit fault", func(t *testing.T) {
		db, _ := openPopulatedExportDB(t)
		paths := exportPaths(t)
		changed, err := db.ExportLegacy(context.Background(), paths)
		require.NoError(t, err)
		require.True(t, changed)
		before := requireValidSnapshot(t, db)
		changed, err = db.reimportLegacyWithCommitHook(
			context.Background(),
			ImportOptions{
				CNSPath:             paths.CNSJSONPath,
				EndpointPath:        paths.EndpointJSONPath,
				ManageEndpointState: true,
				BootID:              reimportTestNewBoot,
			},
			BootPolicy{},
			os.ReadFile,
			func() error { return errInjectedReimport },
		)
		require.ErrorIs(t, err, errInjectedReimport)
		assert.False(t, changed)
		assert.Equal(t, before, requireValidSnapshot(t, db))
	})

	t.Run("Bolt authoritative", func(t *testing.T) {
		db, _ := openPopulatedExportDB(t)
		changed, err := db.ReimportLegacy(context.Background(), ImportOptions{
			CNSPath:             "unused-cns.json",
			EndpointPath:        "unused-endpoints.json",
			ManageEndpointState: true,
			BootID:              reimportTestNewBoot,
		}, BootPolicy{})
		require.ErrorIs(t, err, ErrLegacyReimportState)
		assert.False(t, changed)
	})

	t.Run("canceled", func(t *testing.T) {
		db, _ := openPopulatedExportDB(t)
		paths := exportPaths(t)
		changed, err := db.ExportLegacy(context.Background(), paths)
		require.NoError(t, err)
		require.True(t, changed)
		before := requireValidSnapshot(t, db)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		changed, err = db.ReimportLegacy(ctx, ImportOptions{
			CNSPath:             paths.CNSJSONPath,
			EndpointPath:        paths.EndpointJSONPath,
			ManageEndpointState: true,
			BootID:              "new-boot",
		}, BootPolicy{})
		require.ErrorIs(t, err, context.Canceled)
		assert.False(t, changed)
		assert.Equal(t, before, requireValidSnapshot(t, db))
	})
}
