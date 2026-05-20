// Copyright 2024 Microsoft. All rights reserved.
// MIT License

// Package metric publishes CNS bootstrap and lifecycle metrics for
// Prometheus scraping. The exported "Record*" setters are
// idempotent: callers can invoke them at known startup boundaries
// and only the first call records a value.
//
// The metrics in this file are intended for production dashboards
// and fleet-wide startup SLO tracking; they expose enough phase
// boundaries that consumers can compute per-node bootstrap time
// directly from /metrics rather than reconstructing it from logs.
package metric

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/internal/fs"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// BootStateFile is the persistent location where CNS records the
// current kernel boot_id so that subsequent CNS starts can detect
// whether the kernel has restarted (reboot) or only the CNS
// process has restarted (restart). The file is small (~36 bytes)
// and lives alongside the main CNS state.
//
// Variable rather than const so tests can override it.
var BootStateFile = "/var/lib/azure-network/.cns_boot_id"

// Boot state label values for cns_boot_state.
const (
	BootStateFresh   = "fresh"   // no prior CNS run on this node
	BootStateReboot  = "reboot"  // kernel restarted since CNS last ran
	BootStateRestart = "restart" // CNS process restart, kernel intact
	BootStateUnknown = "unknown" // could not determine (e.g., Windows v1)
)

// Event labels for cns_time_to_event_seconds.
const (
	EventStateRestored          = "state_restored"
	EventFirstNNCReceived       = "first_nnc_received"
	EventInitialIPAMReconciled  = "initial_ipam_reconciled"
	EventFirstNCProgrammed      = "first_nc_programmed"
	EventHTTPListenerReady      = "http_listener_ready"
	EventConflistWritten        = "conflist_written"
	EventReadyToAssign          = "ready_to_assign"
)

// allEvents is iterated at registration time so every label
// combination has a series with sample count 0, giving dashboards
// the full set up-front.
var allEvents = []string{
	EventStateRestored,
	EventFirstNNCReceived,
	EventInitialIPAMReconciled,
	EventFirstNCProgrammed,
	EventHTTPListenerReady,
	EventConflistWritten,
	EventReadyToAssign,
}

var (
	cnsBuildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cns_build_info",
			Help: "Constant 1 with build identity labels for CNS.",
		},
		[]string{"version", "goversion", "os"},
	)

	cnsStartTime = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "cns_start_time_seconds",
			Help: "Unix timestamp (sub-second precision) of when CNS main() was entered.",
		},
	)

	cnsModeInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cns_mode_info",
			Help: "Constant 1 with the active CNS mode labels (channel mode, IPAM v2, SwiftV2, manage-endpoint-state, dual-stack).",
		},
		[]string{"channel_mode", "ipam_v2", "swift_v2", "manage_endpoint_state", "dual_stack"},
	)

	cnsBootState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cns_boot_state",
			Help: "Boot-state classification: fresh = no prior CNS run on this node; reboot = kernel restarted since CNS last ran; restart = CNS process restart on a running kernel; unknown = could not classify. Exactly one label value is set to 1 per process.",
		},
		[]string{"state"},
	)

	cnsStateRestoredSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cns_state_restored_seconds",
		Help: "Unix timestamp at which CNS state was restored from disk (or determined absent).",
	})

	cnsFirstNNCReceivedSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cns_first_nnc_received_seconds",
		Help: "Unix timestamp of the first NodeNetworkConfig reconcile, regardless of init success.",
	})

	cnsInitialIPAMReconciledSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cns_initial_ipam_reconciled_seconds",
		Help: "Unix timestamp at which the initial IPAM reconcile from the first NNC completed successfully.",
	})

	cnsFirstNCProgrammedSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cns_first_nc_programmed_seconds",
		Help: "Unix timestamp at which CNS first observed an NC reach its desired version via NMAgent.",
	})

	cnsHTTPListenerReadySeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cns_http_listener_ready_seconds",
		Help: "Unix timestamp at which the CNS HTTP REST listener bound successfully.",
	})

	cnsConflistWrittenSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cns_conflist_written_seconds",
		Help: "Unix timestamp at which the CNI conflist was written (after atomic rename).",
	})

	cnsReadyToAssignSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cns_ready_to_assign_seconds",
		Help: "Unix timestamp at which CNS first satisfied the mode-aware ready-to-assign predicate (HTTP listener up AND mode-specific IP availability).",
	})

	// Pair with cns_first_nnc_received_seconds: that gauge answers
	// "did NNC ever arrive?", these gauges answer "is it still
	// arriving?" and "is it still being processed successfully?"
	// Suitable for staleness alerting (e.g. "NNC last reconciled >
	// 10 minutes ago" -> page).
	cnsNNCLastReceivedSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cns_nnc_last_received_seconds",
		Help: "Unix timestamp of the most recent NodeNetworkConfig reconcile (regardless of outcome). Pair with cns_first_nnc_received_seconds for end-to-end staleness alerting.",
	})

	cnsNNCLastSuccessfulReconcileSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cns_nnc_last_successful_reconcile_seconds",
		Help: "Unix timestamp of the most recent NodeNetworkConfig reconcile that returned without error.",
	})

	// Latency histogram and result counter for the NNC reconciler.
	// Detects stuck or hot-looping reconciliation in production.
	// Bounded label set: result enum has 3 values.
	cnsNNCReconcileDurationSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "cns_nnc_reconcile_duration_seconds",
		Help: "Wall-clock duration of NodeNetworkConfig reconciler invocations.",
		// 1ms .. ~16s, same shape as the existing CNS HTTP-latency
		// histogram so dashboards can correlate.
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), //nolint:gomnd // standard bucket geometry
	})

	cnsNNCReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cns_nnc_reconcile_total",
			Help: "Count of NodeNetworkConfig reconcile invocations by terminal result.",
		},
		[]string{"result"},
	)

	// Histogram of process-relative time to each bootstrap event.
	// Records exactly one observation per event per process. The
	// per-event gauges (cns_*_seconds) provide the per-node "when"
	// view; this histogram provides the fleet-wide "how long" view
	// so dashboards can compute histogram_quantile() across nodes
	// without external recording rules.
	cnsTimeToEventSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "cns_time_to_event_seconds",
			Help: "Process-relative time (cns_start_time_seconds -> event timestamp) for each bootstrap event, one observation per event per process.",
			Buckets: []float64{
				0.5, 1, 2, 5, 10, 20, 30, 60, 120, 300,
			},
		},
		[]string{"event"},
	)
)

// NNC reconcile result label values for cns_nnc_reconcile_total.
const (
	NNCReconcileResultSuccess = "success"
	NNCReconcileResultError   = "error"
	NNCReconcileResultRequeue = "requeue"
)

func init() {
	metrics.Registry.MustRegister(
		cnsBuildInfo,
		cnsStartTime,
		cnsModeInfo,
		cnsBootState,
		cnsStateRestoredSeconds,
		cnsFirstNNCReceivedSeconds,
		cnsInitialIPAMReconciledSeconds,
		cnsFirstNCProgrammedSeconds,
		cnsHTTPListenerReadySeconds,
		cnsConflistWrittenSeconds,
		cnsReadyToAssignSeconds,
		cnsNNCLastReceivedSeconds,
		cnsNNCLastSuccessfulReconcileSeconds,
		cnsNNCReconcileDurationSeconds,
		cnsNNCReconcileTotal,
		cnsTimeToEventSeconds,
	)
	// Pre-register every boot_state series with value 0 so dashboards
	// can rely on the full label set being present. Same pattern as
	// upstream kube_pod_status_phase. ClassifyAndRecordBootState sets
	// the active value to 1 once classification completes.
	for _, s := range []string{BootStateFresh, BootStateReboot, BootStateRestart, BootStateUnknown} {
		cnsBootState.WithLabelValues(s).Set(0)
	}
	// Pre-create the per-event histogram series so the full label
	// set is visible in /metrics from process start. Each Observe()
	// adds to the appropriate series; pre-creating them at zero
	// just makes the empty buckets visible.
	for _, ev := range allEvents {
		cnsTimeToEventSeconds.WithLabelValues(ev)
	}
	// Same for the NNC reconcile result counter.
	for _, r := range []string{NNCReconcileResultSuccess, NNCReconcileResultError, NNCReconcileResultRequeue} {
		cnsNNCReconcileTotal.WithLabelValues(r)
	}
}

// nowSeconds returns the current Unix time as a float64 with
// sub-second precision.
func nowSeconds() float64 {
	return float64(time.Now().UnixNano()) / 1e9
}

// idempotent setters

var (
	startTimeOnce            sync.Once
	bootStateOnce            sync.Once
	stateRestoredOnce        sync.Once
	firstNNCReceivedOnce     sync.Once
	initialIPAMReconciledOnce sync.Once
	firstNCProgrammedOnce    sync.Once
	httpListenerReadyOnce    sync.Once
	conflistWrittenOnce      sync.Once
	readyToAssignOnce        sync.Once
)

// SetBuildInfo records the build identity. Safe to call multiple
// times; the gauge is idempotent in shape (same labels = same series).
func SetBuildInfo(version string) {
	cnsBuildInfo.WithLabelValues(version, runtime.Version(), runtime.GOOS).Set(1)
}

// SetMode records the active CNS mode. Bool labels are encoded as
// "true"/"false" strings.
func SetMode(channelMode string, ipamV2, swiftV2, manageEndpointState, dualStack bool) {
	cnsModeInfo.WithLabelValues(
		channelMode,
		strconv.FormatBool(ipamV2),
		strconv.FormatBool(swiftV2),
		strconv.FormatBool(manageEndpointState),
		strconv.FormatBool(dualStack),
	).Set(1)
}

// RecordStartTime sets cns_start_time_seconds to now. Idempotent.
// Should be called as early as possible in main().
func RecordStartTime() {
	startTimeOnce.Do(func() {
		cnsStartTime.Set(nowSeconds())
	})
}

// ClassifyAndRecordBootState reads the current kernel boot_id,
// compares it to the value persisted at BootStateFile, and sets
// cns_boot_state{state} accordingly. The current boot_id is then
// atomically written back to BootStateFile so that subsequent
// CNS starts can compare against it.
//
// On unsupported platforms (e.g., Windows v1), readBootID returns
// "" and the state is recorded as "unknown".
//
// Returns the recorded state for callers that want to log or
// otherwise act on the classification. Idempotent: only the first
// call performs the classification and write.
func ClassifyAndRecordBootState() string {
	var classified string
	bootStateOnce.Do(func() {
		classified = classifyBootState()
		cnsBootState.WithLabelValues(classified).Set(1)
	})
	return classified
}

// classifyBootState performs the boot-state classification logic
// without the once-guard.
func classifyBootState() string {
	current := readBootIDFn()
	if current == "" {
		return BootStateUnknown
	}

	previous, err := os.ReadFile(BootStateFile)
	switch {
	case os.IsNotExist(err):
		writeBootID(current)
		return BootStateFresh
	case err != nil:
		// Treat unreadable boot file as unknown rather than misclassify.
		return BootStateUnknown
	}

	state := BootStateRestart
	if string(previous) != current {
		state = BootStateReboot
	}
	writeBootID(current)
	return state
}

// readBootIDFn is the function used to read the kernel's boot id.
// Indirected through a variable so tests can override it. Defaults
// to the per-OS readBootID implementation.
var readBootIDFn = readBootID

// writeBootID atomically writes the current boot_id to
// BootStateFile. Errors are silent: failure to persist boot_id
// only causes the next CNS start to misclassify, which is not
// catastrophic.
func writeBootID(bootID string) {
	dir := filepath.Dir(BootStateFile)
	// Ensure the directory exists; this matches the other CNS
	// state files (azure-cns.json, azure-endpoints.json) which
	// rely on the directory being created by the daemonset
	// hostPath mount, but creating it here makes the function
	// robust on test rigs and non-AKS deployments.
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gomnd // standard 0755
		return
	}
	w, err := fs.NewAtomicWriter(BootStateFile)
	if err != nil {
		return
	}
	defer w.Close() //nolint:errcheck // best-effort persistence
	_, _ = w.Write([]byte(bootID))
}

// observeTimeToEvent records a sample on cns_time_to_event_seconds
// for the named event. Computed against cns_start_time_seconds; if
// start time hasn't been recorded yet (impossible in practice but
// possible in tests), the observation is skipped to avoid emitting
// a meaningless negative value.
func observeTimeToEvent(event string, eventSeconds float64) {
	startVal := readGaugeValueByPtr(cnsStartTime)
	if startVal == 0 {
		return
	}
	delta := eventSeconds - startVal
	if delta < 0 {
		delta = 0
	}
	cnsTimeToEventSeconds.WithLabelValues(event).Observe(delta)
}

// readGaugeValueByPtr extracts a gauge's current value. Internal
// helper to avoid pulling in a test dependency.
func readGaugeValueByPtr(g prometheus.Gauge) float64 {
	var m dto.Metric
	if err := g.Write(&m); err != nil {
		return 0
	}
	if m.Gauge == nil || m.Gauge.Value == nil {
		return 0
	}
	return *m.Gauge.Value
}

// RecordStateRestored marks the moment restoreState() returned.
func RecordStateRestored() {
	stateRestoredOnce.Do(func() {
		now := nowSeconds()
		cnsStateRestoredSeconds.Set(now)
		observeTimeToEvent(EventStateRestored, now)
	})
}

// RecordFirstNNCReceived marks the moment of the first NNC reconcile.
func RecordFirstNNCReceived() {
	firstNNCReceivedOnce.Do(func() {
		now := nowSeconds()
		cnsFirstNNCReceivedSeconds.Set(now)
		observeTimeToEvent(EventFirstNNCReceived, now)
	})
}

// RecordInitialIPAMReconciled marks the moment the initial IPAM
// reconcile from the first NNC completes successfully.
func RecordInitialIPAMReconciled() {
	initialIPAMReconciledOnce.Do(func() {
		now := nowSeconds()
		cnsInitialIPAMReconciledSeconds.Set(now)
		observeTimeToEvent(EventInitialIPAMReconciled, now)
	})
}

// RecordFirstNCProgrammed marks the moment CNS first observed an
// NC reach its desired version via NMAgent.
func RecordFirstNCProgrammed() {
	firstNCProgrammedOnce.Do(func() {
		now := nowSeconds()
		cnsFirstNCProgrammedSeconds.Set(now)
		observeTimeToEvent(EventFirstNCProgrammed, now)
	})
}

// RecordHTTPListenerReady marks the moment the HTTP REST listener
// bound successfully. Also signals the ready-to-assign recorder.
func RecordHTTPListenerReady() {
	httpListenerReadyOnce.Do(func() {
		now := nowSeconds()
		cnsHTTPListenerReadySeconds.Set(now)
		observeTimeToEvent(EventHTTPListenerReady, now)
		markListenerReady()
	})
}

// RecordConflistWritten marks the moment the CNI conflist was
// atomically renamed into place. Must only be called after both
// the writer's Generate() and Close() have succeeded.
func RecordConflistWritten() {
	conflistWrittenOnce.Do(func() {
		now := nowSeconds()
		cnsConflistWrittenSeconds.Set(now)
		observeTimeToEvent(EventConflistWritten, now)
	})
}

// RecordReadyToAssign marks the moment the mode-aware
// ready-to-assign predicate was first satisfied. The predicate
// itself is owned by the central recorder (see readyToAssign.go);
// callers should normally use NotifyAvailableIPCount and
// NotifyNodeSubnetReady to feed signals to the recorder rather
// than calling this directly.
func RecordReadyToAssign() {
	readyToAssignOnce.Do(func() {
		now := nowSeconds()
		cnsReadyToAssignSeconds.Set(now)
		observeTimeToEvent(EventReadyToAssign, now)
	})
}

// ObserveNNCReconcile records one NNC reconcile invocation: its
// duration, its terminal result, and (if non-error) the timestamp
// of the most recent successful reconcile. The cns_nnc_last_received_seconds
// gauge is updated on every call regardless of result so it
// reflects "is the NNC stream alive?" while
// cns_nnc_last_successful_reconcile_seconds reflects "is the
// reconciler actually progressing?".
func ObserveNNCReconcile(duration time.Duration, result string) {
	cnsNNCReconcileDurationSeconds.Observe(duration.Seconds())
	cnsNNCReconcileTotal.WithLabelValues(result).Inc()
	now := nowSeconds()
	cnsNNCLastReceivedSeconds.Set(now)
	if result == NNCReconcileResultSuccess {
		cnsNNCLastSuccessfulReconcileSeconds.Set(now)
	}
}
