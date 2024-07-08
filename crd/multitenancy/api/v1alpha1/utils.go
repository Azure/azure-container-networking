package v1alpha1

import (
	"reflect"
)

// IsReady checks if all the required fields in the MTPNC status are populated
func (m *MultitenantPodNetworkConfig) IsReady() bool {
	// Check if InterfaceInfos slice is not empty
	return !reflect.DeepEqual(m.Status, MultitenantPodNetworkConfigStatus{})
}
