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
		GatewayIPAddress: "10.0.0.1",
	},
	NetworkContainerid:   ncID,
	NetworkContainerType: cns.Docker,
	SecondaryIPConfigs: map[string]cns.SecondaryIPConfig{
		"10.0.0.2": {
			IPAddress: "10.0.0.2",
			NCVersion: 0,
		},
	},
	SwiftV2PrefixOnNic: false,
}

var validVNETBlockRequest = &cns.CreateNetworkContainerRequest{
	Version: strconv.FormatInt(version, 10),
	IPConfiguration: cns.IPConfiguration{
		GatewayIPAddress: vnetBlockDefaultGateway,
		IPSubnet: cns.IPSubnet{
			PrefixLength: uint8(vnetBlockSubnetPrefixLen),
			IPAddress:    vnetBlockNodeIP,
		},
	},
	NetworkContainerid:   ncID,
	NetworkContainerType: cns.Docker,
	// Ignore first IP in first CIDR Block, i.e. 10.224.0.4
	SecondaryIPConfigs: map[string]cns.SecondaryIPConfig{
		"10.224.0.5": {
			IPAddress: "10.224.0.5",
			NCVersion: version,
		},
		"10.224.0.6": {
			IPAddress: "10.224.0.6",
			NCVersion: version,
		},
		"10.224.0.7": {
			IPAddress: "10.224.0.7",
			NCVersion: version,
		},
		"10.224.0.8": {
			IPAddress: "10.224.0.8",
			NCVersion: version,
		},
		"10.224.0.9": {
			IPAddress: "10.224.0.9",
			NCVersion: version,
		},
		"10.224.0.10": {
			IPAddress: "10.224.0.10",
			NCVersion: version,
		},
		"10.224.0.11": {
			IPAddress: "10.224.0.11",
			NCVersion: version,
		},
		"10.224.0.12": {
			IPAddress: "10.224.0.12",
			NCVersion: version,
		},
		"10.224.0.13": {
			IPAddress: "10.224.0.13",
			NCVersion: version,
		},
		"10.224.0.14": {
			IPAddress: "10.224.0.14",
			NCVersion: version,
		},
	},
	SwiftV2PrefixOnNic: false,
}

var validVNETBlockRequestSwiftV2 = &cns.CreateNetworkContainerRequest{
	Version: strconv.FormatInt(version, 10),
	IPConfiguration: cns.IPConfiguration{
		GatewayIPAddress: vnetBlockDefaultGateway,
		IPSubnet: cns.IPSubnet{
			PrefixLength: uint8(vnetBlockSubnetPrefixLen),
			IPAddress:    vnetBlockNodeIP,
		},
	},
	NetworkContainerid:   ncID,
	NetworkContainerType: cns.Docker,
	SecondaryIPConfigs: map[string]cns.SecondaryIPConfig{
		"10.224.0.8": {
			IPAddress: "10.224.0.8",
			NCVersion: version,
		},
		"10.224.0.9": {
			IPAddress: "10.224.0.9",
			NCVersion: version,
		},
		"10.224.0.10": {
			IPAddress: "10.224.0.10",
			NCVersion: version,
		},
		"10.224.0.11": {
			IPAddress: "10.224.0.11",
			NCVersion: version,
		},
		"10.224.0.12": {
			IPAddress: "10.224.0.12",
			NCVersion: version,
		},
		"10.224.0.13": {
			IPAddress: "10.224.0.13",
			NCVersion: version,
		},
		"10.224.0.14": {
			IPAddress: "10.224.0.14",
			NCVersion: version,
		},
	},
	SwiftV2PrefixOnNic: true,
}

var swiftV2EnabledVNETBlockRequest = &cns.CreateNetworkContainerRequest{
	NetworkContainerid:   ncID,
	NetworkContainerType: cns.Docker,
	Version:              "1",
	IPConfiguration: cns.IPConfiguration{
		IPSubnet: cns.IPSubnet{
			IPAddress:    "10.0.0.1",
			PrefixLength: 24,
		},
		GatewayIPAddress: "10.0.0.1",
	},
	SecondaryIPConfigs: map[string]cns.SecondaryIPConfig{},
	SwiftV2PrefixOnNic: true,
	NCStatus:           "Available",
}

var swiftV2DisabledVNETBlockRequest = &cns.CreateNetworkContainerRequest{
	NetworkContainerid:   ncID,
	NetworkContainerType: cns.Docker,
	Version:              "1",
	IPConfiguration: cns.IPConfiguration{
		IPSubnet: cns.IPSubnet{
			IPAddress:    "10.0.0.1",
			PrefixLength: 24,
		},
		GatewayIPAddress: "10.0.0.1",
	},
	SecondaryIPConfigs: map[string]cns.SecondaryIPConfig{},
	NCStatus:           "Available",
}

var swiftV2DisabledNonVNETBlockRequest = &cns.CreateNetworkContainerRequest{
	NetworkContainerid:   ncID,
	NetworkContainerType: cns.Docker,
	Version:              "0",
	IPConfiguration: cns.IPConfiguration{
		IPSubnet: cns.IPSubnet{
			IPAddress:    "10.0.0.0",
			PrefixLength: 24,
		},
		GatewayIPAddress: "10.0.0.1",
	},
	SecondaryIPConfigs: map[string]cns.SecondaryIPConfig{},
	NCStatus:           "Available",
}
