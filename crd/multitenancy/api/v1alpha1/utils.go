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

// IsScheduledWithDRA reports whether the pod was scheduled with Dynamic
// Resource Allocation for the multitenancy DRA driver. ResourceClaims is
// populated by the MTPNC writer with only the claims created for the
// multitenancy DRA driver, so a non-empty slice means DRA scheduling.
func (m *MultitenantPodNetworkConfig) IsScheduledWithDRA() bool {
	return len(m.Spec.ResourceClaims) > 0
}
