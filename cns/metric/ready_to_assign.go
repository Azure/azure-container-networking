// Copyright 2024 Microsoft. All rights reserved.
// MIT License

package metric

import (
	"sync"
)

// ReadyToAssignMode names the high-level CNS mode that determines
// the ready-to-assign predicate. The predicate is "the next
// normal-pod RequestIPConfigs would succeed" and is mode-specific.
type ReadyToAssignMode int

const (
	// ReadyToAssignModeUnset means no mode has been configured yet;
	// the recorder will not fire until SetReadyToAssignMode is called.
	ReadyToAssignModeUnset ReadyToAssignMode = iota

	// ReadyToAssignModeCRD covers Swift / Overlay (and SwiftV2 in
	// v1, as a best-effort approximation). Predicate: HTTP listener
	// up AND >=1 IP per required family in Available state.
	ReadyToAssignModeCRD

	// ReadyToAssignModeNodeSubnet covers AzureHost / NodeSubnet.
	// Predicate: HTTP listener up AND InitializeNodeSubnet returned
	// successfully (signaled via NotifyNodeSubnetReady).
	ReadyToAssignModeNodeSubnet
)

// readyToAssign tracks the inputs to the mode-aware predicate.
// All fields are protected by mu. The recorder fires
// cns_ready_to_assign_seconds at most once when the predicate
// transitions from false to true.
type readyToAssignRecorder struct {
	mu sync.Mutex

	mode             ReadyToAssignMode
	requireDualStack bool

	listenerReady bool

	availableV4Count uint64
	availableV6Count uint64

	nodeSubnetReady bool
}

var readyRecorder readyToAssignRecorder

// SetReadyToAssignMode configures which predicate the recorder
// will use to determine ready-to-assign. requireDualStack is only
// meaningful for ReadyToAssignModeCRD (in which case the predicate
// requires both an IPv4 AND an IPv6 Available IP).
//
// Should be called once during CNS initialization, after the
// channel mode and dual-stack config are known. Subsequent calls
// overwrite the mode (but the once-only firing is preserved).
func SetReadyToAssignMode(mode ReadyToAssignMode, requireDualStack bool) {
	readyRecorder.mu.Lock()
	readyRecorder.mode = mode
	readyRecorder.requireDualStack = requireDualStack
	readyRecorder.mu.Unlock()
	notifyReadyToAssignWatcher()
}

// NotifyAvailableIPCount feeds the recorder the current count of
// IPs in Available state per family. CRD-mode predicate fires
// when at least one IP is available per required family.
//
// Callers in CNS's IPAM hot path should invoke this whenever the
// per-family Available count changes (or, more pragmatically,
// after each batch of state transitions). Cheap calls are fine —
// the recorder short-circuits once it has fired.
func NotifyAvailableIPCount(availableV4, availableV6 uint64) {
	readyRecorder.mu.Lock()
	readyRecorder.availableV4Count = availableV4
	readyRecorder.availableV6Count = availableV6
	readyRecorder.mu.Unlock()
	notifyReadyToAssignWatcher()
}

// NotifyNodeSubnetReady marks the NodeSubnet path's
// InitializeNodeSubnet as complete.
func NotifyNodeSubnetReady() {
	readyRecorder.mu.Lock()
	readyRecorder.nodeSubnetReady = true
	readyRecorder.mu.Unlock()
	notifyReadyToAssignWatcher()
}

// notifyReadyToAssignWatcher checks whether the predicate is now
// satisfied and, if so, records cns_ready_to_assign_seconds. The
// underlying RecordReadyToAssign is once-guarded, so duplicate
// invocations are safe. Also folds in HTTP listener readiness
// because RecordHTTPListenerReady calls into here.
func notifyReadyToAssignWatcher() {
	readyRecorder.mu.Lock()
	// Listener readiness is sourced from the once-set gauge, but
	// reading the gauge value back from prometheus is awkward; we
	// instead track a local flag set by the same call site.
	listenerReady := readyRecorder.listenerReady
	mode := readyRecorder.mode
	dualStack := readyRecorder.requireDualStack
	v4 := readyRecorder.availableV4Count
	v6 := readyRecorder.availableV6Count
	nodeSubnetReady := readyRecorder.nodeSubnetReady
	readyRecorder.mu.Unlock()

	if !listenerReady {
		return
	}

	switch mode {
	case ReadyToAssignModeCRD:
		if v4 == 0 {
			return
		}
		if dualStack && v6 == 0 {
			return
		}
		RecordReadyToAssign()
	case ReadyToAssignModeNodeSubnet:
		if !nodeSubnetReady {
			return
		}
		RecordReadyToAssign()
	default:
		// Unset / unknown mode: do not fire.
	}
}

// markListenerReady is the internal hook invoked by
// RecordHTTPListenerReady. It updates the recorder's
// listenerReady flag and re-evaluates the predicate.
func markListenerReady() {
	readyRecorder.mu.Lock()
	readyRecorder.listenerReady = true
	readyRecorder.mu.Unlock()
	notifyReadyToAssignWatcher()
}
