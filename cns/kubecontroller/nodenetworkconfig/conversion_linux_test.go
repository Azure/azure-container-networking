package nodenetworkconfig

import (
	"strconv"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var validOverlayRequest = &cns.CreateNetworkContainerRequest{
	HostPrimaryIP: validOverlayNC.NodeIP,
	Version:       strconv.FormatInt(0, 10),
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

var validVNETBlockRequest = &cns.CreateNetworkContainerRequest{
	Version:       strconv.FormatInt(version, 10),
	HostPrimaryIP: vnetBlockNodeIP,
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
		"10.224.0.4": {
			IPAddress: "10.224.0.4",
			NCVersion: version,
		},
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
		"10.224.0.15": {
			IPAddress: "10.224.0.15",
			NCVersion: version,
		},
	},
}

func TestCreateNCRequestFromStaticNCWithConfig(t *testing.T) {
	tests := []struct {
		name      string
		input     v1alpha.NetworkContainer
		isSwiftV2 bool
		want      *cns.CreateNetworkContainerRequest
		wantErr   bool
	}{
		{
			name: "SwiftV2 enabled with VNETBlock - should NOT process all IPs in prefix",
			input: v1alpha.NetworkContainer{
				ID:                 ncID,
				PrimaryIP:          "10.0.0.0/32",
				NodeIP:             "10.0.0.1",
				Type:               v1alpha.VNETBlock,
				SubnetAddressSpace: "10.0.0.0/24",
				DefaultGateway:     "10.0.0.1",
				Version:            1,
				Status:             "Available",
			},
			isSwiftV2: true,
			want: &cns.CreateNetworkContainerRequest{
				NetworkContainerid:   ncID,
				NetworkContainerType: cns.Docker,
				Version:              "1",
				HostPrimaryIP:        "10.0.0.1",
				IPConfiguration: cns.IPConfiguration{
					IPSubnet: cns.IPSubnet{
						IPAddress:    "10.0.0.1",
						PrefixLength: 24,
					},
					GatewayIPAddress: "10.0.0.1",
				},
				SecondaryIPConfigs: map[string]cns.SecondaryIPConfig{
					// No IPs from primary prefix
				},
				NCStatus: "Available",
			},
			wantErr: false,
		},
		{
			name: "SwiftV2 disabled with VNETBlock - should process all IP in prefix",
			input: v1alpha.NetworkContainer{
				ID:                 ncID,
				PrimaryIP:          "10.0.0.0/32",
				NodeIP:             "10.0.0.1",
				Type:               v1alpha.VNETBlock,
				SubnetAddressSpace: "10.0.0.0/24",
				DefaultGateway:     "10.0.0.1",
				Version:            1,
				Status:             "Available",
				IPAssignments: []v1alpha.IPAssignment{
					{
						Name: "test-ip",
						IP:   "10.0.0.10/32",
					},
				},
			},
			isSwiftV2: false,
			want: &cns.CreateNetworkContainerRequest{
				NetworkContainerid:   ncID,
				NetworkContainerType: cns.Docker,
				Version:              "1",
				HostPrimaryIP:        "10.0.0.1",
				IPConfiguration: cns.IPConfiguration{
					IPSubnet: cns.IPSubnet{
						IPAddress:    "10.0.0.1",
						PrefixLength: 24,
					},
					GatewayIPAddress: "10.0.0.1",
				},
				SecondaryIPConfigs: map[string]cns.SecondaryIPConfig{
					"10.0.0.0": {IPAddress: "10.0.0.0", NCVersion: 1},
					// IP assignments
					"10.0.0.10": {IPAddress: "10.0.0.10", NCVersion: 1},
				},
				NCStatus: "Available",
			},
			wantErr: false,
		},
		{
			name: "SwiftV2 disabled with non-VNETBlock type - should process IP in prefix",
			input: v1alpha.NetworkContainer{
				ID:                 ncID,
				PrimaryIP:          "10.0.0.0/32",
				NodeIP:             "10.0.0.1",
				Type:               v1alpha.Overlay,
				SubnetAddressSpace: "10.0.0.0/24",
				DefaultGateway:     "10.0.0.1",
				Version:            1,
				Status:             "Available",
				IPAssignments: []v1alpha.IPAssignment{
					{
						Name: "test-ip",
						IP:   "10.0.0.10/32",
					},
				},
			},
			isSwiftV2: false,
			want: &cns.CreateNetworkContainerRequest{
				NetworkContainerid:   ncID,
				NetworkContainerType: cns.Docker,
				Version:              "0",
				HostPrimaryIP:        "10.0.0.1",
				IPConfiguration: cns.IPConfiguration{
					IPSubnet: cns.IPSubnet{
						IPAddress:    "10.0.0.0",
						PrefixLength: 24,
					},
					GatewayIPAddress: "10.0.0.1",
				},
				SecondaryIPConfigs: map[string]cns.SecondaryIPConfig{
					"10.0.0.0": {IPAddress: "10.0.0.0", NCVersion: 0},
				},
				NCStatus: "Available",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CreateNCRequestFromStaticNC(tt.input, tt.isSwiftV2)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.EqualValues(t, tt.want, got)
		})
	}
}
