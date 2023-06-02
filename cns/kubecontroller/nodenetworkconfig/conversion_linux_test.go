package nodenetworkconfig

import (
	"strconv"

	"github.com/Azure/azure-container-networking/cns"
)

var validOverlayRequest = &cns.CreateNetworkContainerRequest{
	Version: strconv.FormatInt(0, 10),
	IPConfiguration: cns.IPConfiguration{
		IPSubnet: cns.IPSubnet{
			PrefixLength: uint8(subnetPrefixLen),
			IPAddress:    primaryIP,
		},
	},
	NetworkContainerid:   ncID,
	NetworkContainerType: cns.Docker,
	SecondaryIPConfigs: map[string]cns.SecondaryIPConfig{
		"10.0.0.0": {
			IPAddress: "10.0.0.0",
			NCVersion: 0,
		},
		"10.0.0.1": {
			IPAddress: "10.0.0.1",
			NCVersion: 0,
		},
		"10.0.0.2": {
			IPAddress: "10.0.0.2",
			NCVersion: 0,
		},
		"10.0.0.3": {
			IPAddress: "10.0.0.3",
			NCVersion: 0,
		},
	},
}
