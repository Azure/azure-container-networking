package v1alpha1

// IsReady checks if all the required fields in the MTPNC status are populated
func (m *MultitenantPodNetworkConfig) IsReady() bool {
	if m.Status.PrimaryIP == "" ||
		m.Status.MacAddress == "" ||
		m.Status.NCID == "" ||
		m.Status.GatewayIP == "" {
		return false
	}

	// Check if InterfaceInfos slice is not empty
	if len(m.Status.InterfaceInfos) == 0 {
		return false
	}

	// Check if each InterfaceInfo has all required fields populated
	for _, interfaceInfo := range m.Status.InterfaceInfos {
		if interfaceInfo.NCID == "" ||
			interfaceInfo.PrimaryIP == "" ||
			interfaceInfo.MacAddress == "" ||
			interfaceInfo.GatewayIP == "" ||
			interfaceInfo.DeviceType == "" ||
			interfaceInfo.NICType == "" {
			return false
		}
	}

	return true
}
