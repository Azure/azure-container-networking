// Copyright 2024 Microsoft. All rights reserved.
// MIT License

package metric

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func TestClassifyBootState(t *testing.T) {
	tests := []struct {
		name              string
		writePrevBootID   string
		setBootStateFile  bool
		want              string
		wantPersistedBoot string
	}{
		{
			name:              "fresh: no previous boot file",
			setBootStateFile:  false,
			want:              BootStateFresh,
			wantPersistedBoot: "test-boot-id",
		},
		{
			name:              "restart: previous boot id matches current",
			setBootStateFile:  true,
			writePrevBootID:   "test-boot-id",
			want:              BootStateRestart,
			wantPersistedBoot: "test-boot-id",
		},
		{
			name:              "reboot: previous boot id differs",
			setBootStateFile:  true,
			writePrevBootID:   "previous-boot-id",
			want:              BootStateReboot,
			wantPersistedBoot: "test-boot-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			restoreBootFile := overrideBootStateFile(t, filepath.Join(dir, ".cns_boot_id"))
			defer restoreBootFile()
			restoreReadBootID := overrideReadBootID(t, func() string { return "test-boot-id" })
			defer restoreReadBootID()

			if tt.setBootStateFile {
				require.NoError(t, os.WriteFile(BootStateFile, []byte(tt.writePrevBootID), 0o600))
			}

			got := classifyBootState()
			assert.Equal(t, tt.want, got)

			persisted, err := os.ReadFile(BootStateFile)
			require.NoError(t, err)
			assert.Equal(t, tt.wantPersistedBoot, string(persisted))
		})
	}
}

func TestClassifyBootStateUnknownWhenReadFails(t *testing.T) {
	restoreReadBootID := overrideReadBootID(t, func() string { return "" })
	defer restoreReadBootID()

	assert.Equal(t, BootStateUnknown, classifyBootState())
}

func TestReadyToAssignCRDPredicate(t *testing.T) {
	tests := []struct {
		name             string
		dualStack        bool
		listener         bool
		availableV4      uint64
		availableV6      uint64
		wantFire         bool
	}{
		{name: "no listener, has v4", listener: false, availableV4: 1, wantFire: false},
		{name: "listener, no IPs", listener: true, wantFire: false},
		{name: "listener, v4 only, single-stack", listener: true, availableV4: 1, wantFire: true},
		{name: "listener, v4 only, dual-stack", listener: true, dualStack: true, availableV4: 1, wantFire: false},
		{name: "listener, both, dual-stack", listener: true, dualStack: true, availableV4: 1, availableV6: 1, wantFire: true},
		{name: "listener, v6 only, single-stack", listener: true, availableV4: 0, availableV6: 1, wantFire: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetReadyToAssignForTest(t)
			SetReadyToAssignMode(ReadyToAssignModeCRD, tt.dualStack)
			NotifyAvailableIPCount(tt.availableV4, tt.availableV6)
			if tt.listener {
				RecordHTTPListenerReady()
			}

			got := readyAssignTimestamp()
			if tt.wantFire {
				assert.NotZero(t, got, "ready_to_assign should have fired")
			} else {
				assert.Zero(t, got, "ready_to_assign should not have fired")
			}
		})
	}
}

func TestReadyToAssignNodeSubnetPredicate(t *testing.T) {
	resetReadyToAssignForTest(t)
	SetReadyToAssignMode(ReadyToAssignModeNodeSubnet, false)

	// Listener alone is not enough.
	RecordHTTPListenerReady()
	assert.Zero(t, readyAssignTimestamp())

	// NodeSubnet ready triggers it.
	NotifyNodeSubnetReady()
	assert.NotZero(t, readyAssignTimestamp())
}

func TestReadyToAssignFiresOnceOnly(t *testing.T) {
	resetReadyToAssignForTest(t)
	SetReadyToAssignMode(ReadyToAssignModeCRD, false)
	RecordHTTPListenerReady()
	NotifyAvailableIPCount(1, 0)
	first := readyAssignTimestamp()
	require.NotZero(t, first)

	// Subsequent state changes must not change the recorded value.
	NotifyAvailableIPCount(5, 5)
	assert.Equal(t, first, readyAssignTimestamp())
}

func TestSetBuildInfoAndMode(t *testing.T) {
	// These are smoke tests that ensure label values are accepted
	// and the metric is registered.
	SetBuildInfo("v1.2.3-test")
	SetMode("CRD", true, false, true, false)
}

func TestAllBootstrapMetricsRegistered(t *testing.T) {
	// Confirm every bootstrap-metric we ship is reachable through
	// the controller-runtime metrics.Registry that healthserver
	// scrapes via /metrics. A drift here would silently lose the
	// signal in production even if the setter worked.

	// GaugeVec metrics only become visible in the registry once at
	// least one label combination has been instantiated. Trigger
	// the setters that materialize them; the underlying metric
	// objects are still the same registered instances.
	SetBuildInfo("test")
	SetMode("CRD", false, false, false, false)

	want := []string{
		"cns_build_info",
		"cns_start_time_seconds",
		"cns_mode_info",
		"cns_boot_state",
		"cns_state_restored_seconds",
		"cns_first_nnc_received_seconds",
		"cns_initial_ipam_reconciled_seconds",
		"cns_first_nc_programmed_seconds",
		"cns_http_listener_ready_seconds",
		"cns_conflist_written_seconds",
		"cns_ready_to_assign_seconds",
		"cns_nnc_last_received_seconds",
		"cns_nnc_last_successful_reconcile_seconds",
		"cns_nnc_reconcile_duration_seconds",
		"cns_nnc_reconcile_total",
		"cns_time_to_event_seconds",
	}

	mfs, err := ctrlmetrics.Registry.Gather()
	require.NoError(t, err)

	got := map[string]bool{}
	for _, mf := range mfs {
		got[mf.GetName()] = true
	}
	for _, name := range want {
		assert.True(t, got[name], "metric %s not registered", name)
	}
}

func TestObserveNNCReconcileSuccessUpdatesBothStalenessGauges(t *testing.T) {
	// Prime the success gauge to zero so we can detect that a
	// success advances it.
	cnsNNCLastReceivedSeconds.Set(0)
	cnsNNCLastSuccessfulReconcileSeconds.Set(0)

	ObserveNNCReconcile(150*time.Millisecond, NNCReconcileResultSuccess)

	rcvd := readGaugeValue(cnsNNCLastReceivedSeconds)
	succ := readGaugeValue(cnsNNCLastSuccessfulReconcileSeconds)
	assert.NotZero(t, rcvd)
	assert.NotZero(t, succ)
	assert.InDelta(t, rcvd, succ, 0.01, "received and successful should be set to the same instant")
}

func TestObserveNNCReconcileErrorAdvancesOnlyReceived(t *testing.T) {
	cnsNNCLastReceivedSeconds.Set(0)
	cnsNNCLastSuccessfulReconcileSeconds.Set(0)

	ObserveNNCReconcile(50*time.Millisecond, NNCReconcileResultError)

	rcvd := readGaugeValue(cnsNNCLastReceivedSeconds)
	succ := readGaugeValue(cnsNNCLastSuccessfulReconcileSeconds)
	assert.NotZero(t, rcvd, "error reconcile must still update last_received")
	assert.Zero(t, succ, "error reconcile must NOT update last_successful")
}

func TestObserveTimeToEventSkipsWhenStartTimeUnset(t *testing.T) {
	// Reset start-time gauge to zero. observeTimeToEvent should
	// no-op rather than emit a giant negative observation.
	cnsStartTime.Set(0)
	// Read counter before / after; should be unchanged.
	before := readHistogramSampleCount(cnsTimeToEventSeconds.WithLabelValues(EventStateRestored))
	observeTimeToEvent(EventStateRestored, 1700000005)
	after := readHistogramSampleCount(cnsTimeToEventSeconds.WithLabelValues(EventStateRestored))
	assert.Equal(t, before, after, "no observation should be recorded with start time unset")
}

func TestObserveTimeToEventClampsNegativeToZero(t *testing.T) {
	cnsStartTime.Set(1700000010)
	before := readHistogramSampleCount(cnsTimeToEventSeconds.WithLabelValues(EventConflistWritten))
	// Event timestamp earlier than start time -- shouldn't happen
	// in practice but guard against negative observation.
	observeTimeToEvent(EventConflistWritten, 1700000000)
	after := readHistogramSampleCount(cnsTimeToEventSeconds.WithLabelValues(EventConflistWritten))
	assert.Equal(t, before+1, after, "observation should be recorded (clamped to 0)")
}

func TestRecordStartTimeIsIdempotent(t *testing.T) {
	startTimeOnce = sync.Once{}
	RecordStartTime()
	first := startTimestamp()
	require.NotZero(t, first)

	RecordStartTime()
	assert.Equal(t, first, startTimestamp(), "start time must not advance on second call")
}

// --- test helpers ---

// overrideBootStateFile temporarily redirects BootStateFile to
// the given path. The returned function restores the original.
func overrideBootStateFile(t *testing.T, path string) func() {
	t.Helper()
	prev := BootStateFile
	BootStateFile = path
	bootStateOnce = sync.Once{}
	return func() { BootStateFile = prev }
}

// overrideReadBootID temporarily replaces the readBootID function
// for the duration of a test. The returned function restores it.
func overrideReadBootID(t *testing.T, fn func() string) func() {
	t.Helper()
	prev := readBootIDFn
	readBootIDFn = fn
	return func() { readBootIDFn = prev }
}

// resetReadyToAssignForTest wipes the recorder state and the
// once-guard so a test can drive the predicate from scratch.
func resetReadyToAssignForTest(t *testing.T) {
	t.Helper()
	readyRecorder = readyToAssignRecorder{}
	readyToAssignOnce = sync.Once{}
	cnsReadyToAssignSeconds.Set(0)
	httpListenerReadyOnce = sync.Once{}
	cnsHTTPListenerReadySeconds.Set(0)
}

// readyAssignTimestamp reads the current value of the
// cns_ready_to_assign_seconds gauge.
func readyAssignTimestamp() float64 {
	return readGaugeValue(cnsReadyToAssignSeconds)
}

// startTimestamp reads the current value of cns_start_time_seconds.
func startTimestamp() float64 {
	return readGaugeValue(cnsStartTime)
}

// readGaugeValue extracts the float64 value from a prometheus gauge.
func readGaugeValue(g prometheus.Gauge) float64 {
	var m dto.Metric
	if err := g.Write(&m); err != nil {
		return 0
	}
	if m.Gauge == nil || m.Gauge.Value == nil {
		return 0
	}
	return *m.Gauge.Value
}

// readHistogramSampleCount returns the cumulative observation count
// of a Histogram observer.
func readHistogramSampleCount(o prometheus.Observer) uint64 {
	h, ok := o.(prometheus.Histogram)
	if !ok {
		return 0
	}
	var m dto.Metric
	if err := h.Write(&m); err != nil {
		return 0
	}
	if m.Histogram == nil || m.Histogram.SampleCount == nil {
		return 0
	}
	return *m.Histogram.SampleCount
}
