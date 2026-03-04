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
}

func TestIPv6PrefixClampWindows(t *testing.T) {
	tests := []struct {
		name            string
		ipv6PrefixClamp int
		ipAssignment    string
		wantIPCount     int
	}{
		{
			name:            "IPv6 /112 clamped to /120 produces 256 IPs",
			ipv6PrefixClamp: 120,
			ipAssignment:    "fd00:abcd:1234:5678::/112",
			wantIPCount:     256, // /120 = 2^8
		},
		{
			name:            "IPv6 /124 not clamped (narrower than clamp) produces 16 IPs",
			ipv6PrefixClamp: 120,
			ipAssignment:    "fd00:abcd:1234:5678::/124",
			wantIPCount:     16, // /124 = 2^4, narrower than /120
		},
		{
			name:            "IPv4 /24 not affected by IPv6 clamp",
			ipv6PrefixClamp: 120,
			ipAssignment:    "10.0.0.0/24",
			wantIPCount:     256, // /24 = 2^8, IPv4 not clamped
		},
		{
			name:            "Clamp disabled (0) allows full IPv6 /112",
			ipv6PrefixClamp: 0,
			ipAssignment:    "fd00:abcd:1234:5678::/112",
			wantIPCount:     65536, // 2^16
		},
		{
			name:            "Custom clamp /124 clamps /112 to 16 IPs",
			ipv6PrefixClamp: 124,
			ipAssignment:    "fd00:abcd:1234:5678::/112",
			wantIPCount:     16, // /124 = 2^4
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nc := v1alpha.NetworkContainer{
				ID:                 ncID,
				PrimaryIP:          "10.0.0.0/30",
				NodeIP:             "10.0.0.1",
				Type:               v1alpha.VNETBlock,
				SubnetAddressSpace: "10.0.0.0/24",
				DefaultGateway:     "10.0.0.1",
				Version:            1,
				IPAssignments: []v1alpha.IPAssignment{
					{Name: "test-block", IP: tt.ipAssignment},
				},
			}

			got, err := CreateNCRequestFromStaticNC(nc, true, tt.ipv6PrefixClamp) // swiftV2=true to skip primary prefix IPs
			require.NoError(t, err)
			// Windows deletes lastAddr, so the count is one less than the raw CIDR size
			// when VNETBlock IPs are the only source (swiftV2=true skips primary prefix).
			expectedCount := tt.wantIPCount - 1
			assert.Len(t, got.SecondaryIPConfigs, expectedCount,
				"expected %d IPs from CIDR %s with clamp %d (minus 1 for lastAddr delete)",
				expectedCount, tt.ipAssignment, tt.ipv6PrefixClamp)
		})
	}
}
