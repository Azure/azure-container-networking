package v1alpha1

// IsReady checks if all the required fields in the MTPNC status are populated
func (m *MultitenantPodNetworkConfig) IsReady() bool {
	if m.Status.PrimaryIP == "" || m.Status.MacAddress == "" ||
		m.Status.NCID == "" || m.Status.GatewayIP == "" {
		return false
	}

	// Check if InterfaceInfos slice is not empty and each NICType is not empty
	if len(m.Status.InterfaceInfos) == 0 {
		return false
	}

	for _, interfaceInfo := range m.Status.InterfaceInfos {
		if interfaceInfo.NICType == "" {
			return false
		}
	}

	return true
}
