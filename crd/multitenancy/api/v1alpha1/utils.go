package v1alpha1

import (
	"reflect"
)

// IsReady checks if all the required fields in the MTPNC status are populated
func (m *MultitenantPodNetworkConfig) IsReady() bool {
	// Check if InterfaceInfos slice is not empty
	return !reflect.DeepEqual(m.Status, MultitenantPodNetworkConfigStatus{})
}

// IsDeleting returns true if the MultitenantPodNetworkConfig resource has been marked for deletion.
// A resource is considered to be deleting when its DeletionTimestamp field is set.
func (m *MultitenantPodNetworkConfig) IsDeleting() bool {
	return !m.DeletionTimestamp.IsZero()
}

// IsDRAScheduled reports whether this pod was scheduled with Dynamic Resource
// Allocation (DRA). It is derived from Status.ResourceClaims: a non-empty list
// means a DRA driver owns dataplane programming for the pod (via NRI), and CNS
// should skip CNI pod-info delivery. Callers must use this helper rather than
// inspecting the field directly, so the contract has a single source of truth.
func (m *MultitenantPodNetworkConfig) IsDRAScheduled() bool {
	return len(m.Status.ResourceClaims) > 0
}
