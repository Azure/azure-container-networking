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
