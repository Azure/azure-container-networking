// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolterrors "go.etcd.io/bbolt/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

var errMetricRegistration = errors.New("register failure")

const metricTestDelta = 1e-9

func TestNewMetricsRegistration(t *testing.T) {
	t.Run("descriptors", func(t *testing.T) {
		registry := prometheus.NewRegistry()
		metrics, err := NewMetrics(registry)
		require.NoError(t, err)
		require.NoError(t, metrics.ObserveTransaction(TransactionView, ResultSuccess, 0))
		require.NoError(t, metrics.ObserveLifecycle(LifecycleStartup, ResultSuccess, 0))
		require.NoError(t, metrics.ObserveInvariant(InvariantStructural))
		require.NoError(t, metrics.Refresh(Status{
			Backend: BackendBolt, Authority: AuthorityBolt, SchemaVersion: SchemaVersion,
		}))

		families, err := registry.Gather()
		require.NoError(t, err)
		wantHelp := map[string]string{
			"cns_persistent_state_transactions_total":           "Total number of persistent state database transactions by operation and result.",
			"cns_persistent_state_transaction_duration_seconds": "Persistent state database transaction duration in seconds by operation and result.",
			"cns_persistent_state_lifecycle_total":              "Total number of persistent state lifecycle operations by operation and result.",
			"cns_persistent_state_lifecycle_duration_seconds":   "Persistent state lifecycle operation duration in seconds by operation and result.",
			"cns_persistent_state_invariant_failures_total":     "Total number of bounded persistent state invariant failures.",
			"cns_persistent_state_info":                         "Persistent state backend, authority, and schema information.",
			"cns_persistent_state_generation":                   "Current committed persistent state generation.",
			"cns_persistent_state_storage_present":              "Whether persistent state storage is present and readable.",
			"cns_persistent_state_database_bytes":               "Current persistent state database size in bytes.",
			"cns_persistent_state_records":                      "Current persistent state record count by bounded record type.",
		}
		wantLabels := map[string][]string{
			"cns_persistent_state_transactions_total":           {metricLabelOperation, metricLabelResult},
			"cns_persistent_state_transaction_duration_seconds": {metricLabelOperation, metricLabelResult},
			"cns_persistent_state_lifecycle_total":              {metricLabelOperation, metricLabelResult},
			"cns_persistent_state_lifecycle_duration_seconds":   {metricLabelOperation, metricLabelResult},
			"cns_persistent_state_invariant_failures_total":     {metricLabelInvariant},
			"cns_persistent_state_info":                         {metricLabelBackend, metricLabelAuthority, metricLabelSchema},
			"cns_persistent_state_generation":                   {},
			"cns_persistent_state_storage_present":              {},
			"cns_persistent_state_database_bytes":               {},
			"cns_persistent_state_records":                      {metricLabelRecordType},
		}
		for name, help := range wantHelp {
			family := findMetricFamily(t, families, name)
			assert.Equal(t, help, family.GetHelp())
			require.NotEmpty(t, family.GetMetric())
			var labels []string
			for _, label := range family.GetMetric()[0].GetLabel() {
				labels = append(labels, label.GetName())
			}
			assert.ElementsMatch(t, wantLabels[name], labels)
		}
	})

	t.Run("nil and duplicate registerer", func(t *testing.T) {
		_, err := NewMetrics(nil)
		require.Error(t, err)

		registry := prometheus.NewRegistry()
		metrics, err := NewMetrics(registry)
		require.NoError(t, err)
		require.NoError(t, metrics.ObserveTransaction(TransactionView, ResultSuccess, 0))
		require.NoError(t, metrics.ObserveLifecycle(LifecycleStartup, ResultSuccess, 0))
		require.NoError(t, metrics.ObserveInvariant(InvariantStructural))
		require.NoError(t, metrics.Refresh(Status{
			Backend: BackendBolt, Authority: AuthorityBolt, SchemaVersion: SchemaVersion,
		}))
		_, err = NewMetrics(registry)
		require.Error(t, err)
		families, gatherErr := registry.Gather()
		require.NoError(t, gatherErr)
		assert.Len(t, families, 10)
	})

	t.Run("partial registration rolls back", func(t *testing.T) {
		registerer := &failingRegisterer{failAt: 3}
		_, err := NewMetrics(registerer)
		require.Error(t, err)
		assert.Equal(t, 2, registerer.unregistered)
	})

	t.Run("partial registration reports incomplete rollback", func(t *testing.T) {
		registerer := &failingRegisterer{failAt: 2, failUnregister: true}
		_, err := NewMetrics(registerer)
		require.ErrorContains(t, err, "collector rollback incomplete")
	})
}

func TestMetricsValidateBoundedInputs(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := NewMetrics(registry)
	require.NoError(t, err)

	require.Error(t, metrics.ObserveTransaction("dynamic", ResultSuccess, time.Second))
	require.Error(t, metrics.ObserveTransaction(TransactionView, "dynamic", time.Second))
	require.Error(t, metrics.ObserveTransaction(TransactionView, ResultSuccess, -time.Second))
	require.Error(t, metrics.ObserveLifecycle("dynamic", ResultSuccess, time.Second))
	require.Error(t, metrics.ObserveLifecycle(LifecycleBoot, ResultSuccess, -time.Second))
	require.Error(t, metrics.ObserveInvariant("dynamic"))
	require.NoError(t, (*Metrics)(nil).ObserveTransaction(TransactionView, ResultSuccess, 0))
	require.NoError(t, (*Metrics)(nil).ObserveLifecycle(LifecycleBoot, ResultSuccess, 0))
	require.NoError(t, (*Metrics)(nil).ObserveInvariant(InvariantStructural))
	require.NoError(t, (*Metrics)(nil).Refresh(Status{}))
	require.Error(t, metrics.Refresh(Status{Backend: "dynamic", Authority: AuthorityBolt, SchemaVersion: SchemaVersion}))
	require.Error(t, metrics.Refresh(Status{Backend: BackendBolt, Authority: "dynamic", SchemaVersion: SchemaVersion}))
	require.Error(t, metrics.Refresh(Status{Backend: BackendBolt, Authority: AuthorityBolt, SchemaVersion: SchemaVersion + 1}))
	require.NoError(t, metrics.Refresh(Status{InvariantStatus: InvariantFailed}))
	assert.Zero(t, Status{}.RecordCount("dynamic"))
}

func TestTransactionMetricsClassifyResults(t *testing.T) {
	db, registry, metrics := openObservedTestDB(t, nil)
	base := time.Date(2026, time.July, 24, 1, 0, 0, 0, time.UTC)
	var clockMu sync.Mutex
	metrics.now = func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		base = base.Add(250 * time.Millisecond)
		return base
	}

	require.NoError(t, db.View(context.Background(), func(*ReadTx) error { return nil }))
	require.ErrorIs(t, db.View(context.Background(), func(*ReadTx) error { return errAbort }), errAbort)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, db.View(canceled, func(*ReadTx) error { return nil }), context.Canceled)

	require.NoError(t, db.Update(context.Background(), func(tx *WriteTx) error {
		return tx.PutMetadata(Metadata{Initialized: true})
	}))
	require.ErrorIs(t, db.Update(context.Background(), func(*WriteTx) error { return errAbort }), errAbort)
	changed, err := db.update(context.Background(), func(*WriteTx) (bool, error) { return false, nil })
	require.NoError(t, err)
	assert.False(t, changed)
	_, err = db.ReplaceDurableState(context.Background(), 99, NewDurableState())
	require.ErrorIs(t, err, ErrStaleGeneration)
	require.ErrorIs(t, db.Update(canceled, func(*WriteTx) error { return nil }), context.Canceled)

	families, err := registry.Gather()
	require.NoError(t, err)
	assert.InDelta(t, float64(1), counterValue(t, families, "cns_persistent_state_transactions_total", map[string]string{
		metricLabelOperation: string(TransactionUpdate), metricLabelResult: string(ResultSuccess),
	}), metricTestDelta)
	for _, result := range []OperationResult{ResultError, ResultNoop, ResultConflict, ResultCanceled} {
		assert.InDelta(t, float64(1), counterValue(t, families, "cns_persistent_state_transactions_total", map[string]string{
			metricLabelOperation: string(TransactionUpdate), metricLabelResult: string(result),
		}), metricTestDelta)
	}
	assert.Equal(t, uint64(1), histogramCount(t, families, "cns_persistent_state_transaction_duration_seconds", map[string]string{
		metricLabelOperation: string(TransactionUpdate), metricLabelResult: string(ResultSuccess),
	}))
	assert.InDelta(t, 0.25, histogramSum(t, families, "cns_persistent_state_transaction_duration_seconds", map[string]string{
		metricLabelOperation: string(TransactionUpdate), metricLabelResult: string(ResultSuccess),
	}), metricTestDelta)
}

func TestLifecycleMetricsAndLogs(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	db, registry, _ := openObservedTestDB(t, zap.New(core))

	changed, err := db.ApplyBoot(context.Background(), "boot-1", BootPolicy{})
	require.NoError(t, err)
	assert.True(t, changed)
	changed, err = db.ApplyBoot(context.Background(), "boot-1", BootPolicy{})
	require.NoError(t, err)
	assert.False(t, changed)
	_, err = db.ApplyBoot(context.Background(), "", BootPolicy{})
	require.Error(t, err)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = db.ApplyBoot(canceled, "boot-2", BootPolicy{})
	require.ErrorIs(t, err, context.Canceled)

	cnsData, endpointData := completeLegacyImportData(t)
	importDB, importRegistry, _ := openObservedTestDB(t, nil)
	importOpts := writeLegacyImportFiles(t, cnsData, endpointData, true)
	changed, err = importDB.ImportLegacy(context.Background(), importOpts)
	require.NoError(t, err)
	assert.True(t, changed)
	changed, err = importDB.ImportLegacy(context.Background(), importOpts)
	require.NoError(t, err)
	assert.False(t, changed)
	_, err = importDB.ImportLegacy(context.Background(), ImportOptions{})
	require.Error(t, err)
	exportOpts := exportPaths(t)
	changed, err = importDB.ExportLegacy(context.Background(), exportOpts)
	require.NoError(t, err)
	assert.True(t, changed)
	changed, err = importDB.ExportLegacy(context.Background(), exportOpts)
	require.NoError(t, err)
	assert.False(t, changed)
	_, err = importDB.ExportLegacy(context.Background(), ExportOptions{})
	require.Error(t, err)

	families, err := registry.Gather()
	require.NoError(t, err)
	assert.InDelta(t, float64(1), counterValue(t, families, "cns_persistent_state_lifecycle_total", map[string]string{
		metricLabelOperation: string(LifecycleStartup), metricLabelResult: string(ResultSuccess),
	}), metricTestDelta)
	assert.InDelta(t, float64(1), counterValue(t, families, "cns_persistent_state_lifecycle_total", map[string]string{
		metricLabelOperation: string(LifecycleBoot), metricLabelResult: string(ResultSuccess),
	}), metricTestDelta)
	assert.InDelta(t, float64(1), counterValue(t, families, "cns_persistent_state_lifecycle_total", map[string]string{
		metricLabelOperation: string(LifecycleBoot), metricLabelResult: string(ResultNoop),
	}), metricTestDelta)
	assert.InDelta(t, float64(1), counterValue(t, families, "cns_persistent_state_lifecycle_total", map[string]string{
		metricLabelOperation: string(LifecycleBoot), metricLabelResult: string(ResultError),
	}), metricTestDelta)
	assert.InDelta(t, float64(1), counterValue(t, families, "cns_persistent_state_lifecycle_total", map[string]string{
		metricLabelOperation: string(LifecycleBoot), metricLabelResult: string(ResultCanceled),
	}), metricTestDelta)
	assert.Equal(t, uint64(1), histogramCount(t, families, "cns_persistent_state_lifecycle_duration_seconds", map[string]string{
		metricLabelOperation: string(LifecycleBoot), metricLabelResult: string(ResultSuccess),
	}))

	importFamilies, err := importRegistry.Gather()
	require.NoError(t, err)
	for _, operation := range []LifecycleOperation{LifecycleImport, LifecycleRollback} {
		for _, result := range []OperationResult{ResultSuccess, ResultNoop} {
			assert.InDelta(t, float64(1), counterValue(t, importFamilies, "cns_persistent_state_lifecycle_total", map[string]string{
				metricLabelOperation: string(operation), metricLabelResult: string(result),
			}), metricTestDelta)
		}
		assert.InDelta(t, float64(1), counterValue(t, importFamilies, "cns_persistent_state_lifecycle_total", map[string]string{
			metricLabelOperation: string(operation), metricLabelResult: string(ResultError),
		}), metricTestDelta)
	}

	messages := logs.All()
	require.Len(t, messages, 3)
	assert.Equal(t, "persistent state opened", messages[0].Message)
	assert.Equal(t, "persistent state boot applied", messages[1].Message)
	assert.Equal(t, "persistent state boot unchanged", messages[2].Message)
	fields := messages[1].ContextMap()
	assert.Equal(t, BackendBolt, fields["backend"])
	assert.Equal(t, string(AuthorityBolt), fields["authority"])
	assert.EqualValues(t, SchemaVersion, fields["schema"])
	assert.EqualValues(t, 1, fields["generation"])
	assert.Equal(t, string(LifecycleBoot), fields[metricLabelOperation])
	assert.Equal(t, string(ResultSuccess), fields[metricLabelResult])
	assert.NotContains(t, fields, "path")
	assert.NotContains(t, fields, "error")
}

func TestStartupErrorMetricIsReturnedWithoutLogging(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := NewMetrics(registry)
	require.NoError(t, err)
	core, logs := observer.New(zap.InfoLevel)
	_, err = Open(filepath.Join(t.TempDir(), "missing", "azure-cns.db"), Options{
		Metrics: metrics,
		Logger:  zap.New(core),
	})
	require.Error(t, err)
	assert.Empty(t, logs.All())

	families, gatherErr := registry.Gather()
	require.NoError(t, gatherErr)
	assert.InDelta(t, float64(1), counterValue(t, families, "cns_persistent_state_lifecycle_total", map[string]string{
		metricLabelOperation: string(LifecycleStartup), metricLabelResult: string(ResultError),
	}), metricTestDelta)
}

func TestStatusAndGaugeRefresh(t *testing.T) {
	db, registry, _ := openObservedTestDB(t, nil)
	snapshot := completeSnapshot()
	snapshot.Metadata.BootID = "sensitive-boot"
	snapshot.Metadata.NodeID = "sensitive-node"
	writeSnapshot(t, db, snapshot)
	require.NoError(t, db.Update(context.Background(), func(tx *WriteTx) error {
		return tx.PutMetadata(snapshot.Metadata)
	}))

	status, err := db.RefreshMetrics(context.Background())
	require.NoError(t, err)
	assert.Equal(t, BackendBolt, status.Backend)
	assert.Equal(t, AuthorityBolt, status.Authority)
	assert.True(t, status.BootPresent)
	assert.True(t, status.StoragePresent)
	assert.Positive(t, status.DatabaseBytes)
	assert.Equal(t, InvariantHealthy, status.InvariantStatus)
	assert.Equal(t, RecordCounts{
		NetworkContainers: len(snapshot.NetworkContainers),
		IPs:               len(snapshot.IPs),
		Networks:          len(snapshot.Networks),
		Endpoints:         len(snapshot.Endpoints),
		Assignments:       len(snapshot.Assignments),
		Owners:            len(snapshot.IPOwners),
		DeleteIntents:     len(snapshot.DeleteIntents),
	}, status.Records)

	data, err := json.Marshal(status)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "sensitive-boot")
	assert.NotContains(t, string(data), "sensitive-node")
	assert.NotContains(t, string(data), "path")

	families, err := registry.Gather()
	require.NoError(t, err)
	assert.InDelta(t, float64(status.Generation), gaugeValue(t, families, "cns_persistent_state_generation", nil), metricTestDelta)
	assert.InDelta(t, float64(status.DatabaseBytes), gaugeValue(t, families, "cns_persistent_state_database_bytes", nil), metricTestDelta)
	assert.InDelta(t, float64(status.Records.IPs), gaugeValue(t, families, "cns_persistent_state_records", map[string]string{
		metricLabelRecordType: string(RecordIP),
	}), metricTestDelta)
	assert.InDelta(t, float64(1), gaugeValue(t, families, "cns_persistent_state_info", map[string]string{
		metricLabelBackend:   BackendBolt,
		metricLabelAuthority: string(AuthorityBolt),
		metricLabelSchema:    "1",
	}), metricTestDelta)
	assertBoundedMetricLabels(t, families)
}

func TestStatusInvalidClosedAndCanceled(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *DB)
		want   InvariantName
	}{
		{
			name: "logical structure",
			mutate: func(t *testing.T, db *DB) {
				snapshot := completeSnapshot()
				snapshot.IPOwners["ip-v4"] = "missing-assignment"
				writeSnapshot(t, db, snapshot)
			},
			want: InvariantStructural,
		},
		{
			name: "schema",
			mutate: func(t *testing.T, db *DB) {
				putRaw(t, db, bucketMetadata, metaKeySchemaVersion, encodeUint32(SchemaVersion+1))
			},
			want: InvariantSchema,
		},
		{
			name: "authority",
			mutate: func(t *testing.T, db *DB) {
				putRaw(t, db, bucketMetadata, metaKeyAuthority, []byte("dynamic"))
			},
			want: InvariantAuthority,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, registry, _ := openObservedTestDB(t, nil)
			tt.mutate(t, db)
			status, err := db.Status(context.Background())
			require.NoError(t, err)
			assert.Equal(t, InvariantFailed, status.InvariantStatus)
			assert.Equal(t, tt.want, status.FailedInvariant)
			families, gatherErr := registry.Gather()
			require.NoError(t, gatherErr)
			assert.InDelta(t, float64(1), counterValue(t, families, "cns_persistent_state_invariant_failures_total", map[string]string{
				metricLabelInvariant: string(tt.want),
			}), metricTestDelta)
		})
	}

	db, _, _ := openObservedTestDB(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := db.Status(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, db.Close())
	_, err = db.Status(context.Background())
	require.ErrorIs(t, err, bolterrors.ErrDatabaseNotOpen)
}

func TestMetricsConcurrentUse(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := NewMetrics(registry)
	require.NoError(t, err)
	status := Status{Backend: BackendBolt, Authority: AuthorityBolt, SchemaVersion: SchemaVersion}

	const workers = 32
	errs := make(chan error, workers*4)
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			errs <- metrics.ObserveTransaction(TransactionView, ResultSuccess, time.Millisecond)
			errs <- metrics.ObserveLifecycle(LifecycleBoot, ResultNoop, time.Millisecond)
			errs <- metrics.ObserveInvariant(InvariantStructural)
			errs <- metrics.Refresh(status)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	families, err := registry.Gather()
	require.NoError(t, err)
	assert.InDelta(t, float64(workers), counterValue(t, families, "cns_persistent_state_transactions_total", map[string]string{
		metricLabelOperation: string(TransactionView), metricLabelResult: string(ResultSuccess),
	}), metricTestDelta)
}

func openObservedTestDB(t *testing.T, logger *zap.Logger) (*DB, *prometheus.Registry, *Metrics) {
	t.Helper()
	registry := prometheus.NewRegistry()
	metrics, err := NewMetrics(registry)
	require.NoError(t, err)
	db, err := Open(filepath.Join(t.TempDir(), "azure-cns.db"), Options{Metrics: metrics, Logger: logger})
	require.NoError(t, err)
	t.Cleanup(func() {
		err := db.Close()
		require.True(t, err == nil || errors.Is(err, bolterrors.ErrDatabaseNotOpen))
	})
	return db, registry, metrics
}

type failingRegisterer struct {
	calls          int
	failAt         int
	unregistered   int
	failUnregister bool
}

func (r *failingRegisterer) Register(prometheus.Collector) error {
	r.calls++
	if r.calls == r.failAt {
		return errMetricRegistration
	}
	return nil
}

func (r *failingRegisterer) MustRegister(...prometheus.Collector) {}

func (r *failingRegisterer) Unregister(prometheus.Collector) bool {
	r.unregistered++
	return !r.failUnregister
}

func findMetricFamily(t *testing.T, families []*dto.MetricFamily, name string) *dto.MetricFamily {
	t.Helper()
	for _, family := range families {
		if family.GetName() == name {
			return family
		}
	}
	t.Fatalf("metric family %q not found", name)
	return nil
}

func findMetric(t *testing.T, family *dto.MetricFamily, labels map[string]string) *dto.Metric {
	t.Helper()
	for _, metric := range family.GetMetric() {
		got := map[string]string{}
		for _, label := range metric.GetLabel() {
			got[label.GetName()] = label.GetValue()
		}
		if (len(labels) == 0 && len(got) == 0) || assert.ObjectsAreEqual(labels, got) {
			return metric
		}
	}
	t.Fatalf("metric %q labels %v not found", family.GetName(), labels)
	return nil
}

func counterValue(t *testing.T, families []*dto.MetricFamily, name string, labels map[string]string) float64 {
	t.Helper()
	return findMetric(t, findMetricFamily(t, families, name), labels).GetCounter().GetValue()
}

func gaugeValue(t *testing.T, families []*dto.MetricFamily, name string, labels map[string]string) float64 {
	t.Helper()
	return findMetric(t, findMetricFamily(t, families, name), labels).GetGauge().GetValue()
}

func histogramCount(t *testing.T, families []*dto.MetricFamily, name string, labels map[string]string) uint64 {
	t.Helper()
	return findMetric(t, findMetricFamily(t, families, name), labels).GetHistogram().GetSampleCount()
}

func histogramSum(t *testing.T, families []*dto.MetricFamily, name string, labels map[string]string) float64 {
	t.Helper()
	return findMetric(t, findMetricFamily(t, families, name), labels).GetHistogram().GetSampleSum()
}

func assertBoundedMetricLabels(t *testing.T, families []*dto.MetricFamily) {
	t.Helper()
	allowedNames := map[string]struct{}{
		metricLabelOperation:  {},
		metricLabelResult:     {},
		metricLabelInvariant:  {},
		metricLabelBackend:    {},
		metricLabelAuthority:  {},
		metricLabelSchema:     {},
		metricLabelRecordType: {},
	}
	for _, family := range families {
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				_, ok := allowedNames[label.GetName()]
				assert.True(t, ok, "%s has unexpected label %q", family.GetName(), label.GetName())
				for _, prohibited := range []string{"node-", "pod-", "10.", "fd00", "/", "secret"} {
					assert.NotContains(t, label.GetValue(), prohibited)
				}
			}
		}
	}
}
