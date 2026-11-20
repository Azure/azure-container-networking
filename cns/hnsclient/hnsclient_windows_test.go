package hnsclient

import (
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdhocAdjustIPConfig(t *testing.T) {
	tests := []struct {
		name     string
		ipConfig cns.IPConfiguration
		expected cns.IPConfiguration
	}{
		{
			name:     "expect no change when gw address is not 169.254.128.1",
			ipConfig: cns.IPConfiguration{GatewayIPAddress: "169.254.128.3"},
			expected: cns.IPConfiguration{GatewayIPAddress: "169.254.128.3"},
		},
		{
			name:     "expect default gw address is set when gw address is 169.254.128.1",
			ipConfig: cns.IPConfiguration{GatewayIPAddress: "169.254.128.1"},
			expected: cns.IPConfiguration{GatewayIPAddress: "169.254.128.2"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			updateGwForLocalIPConfiguration(&tt.ipConfig)
			assert.Equal(t, tt.expected.GatewayIPAddress, tt.ipConfig.GatewayIPAddress)
		})
	}
}

func TestConfigureHostNCApipaEndpointDefaultRoute(t *testing.T) {
	tests := []struct {
		name              string
		skipDefaultRoutes bool
		wantNextHop       string
	}{
		{
			name:        "default route via the host loopback adapter",
			wantNextHop: hnsLoopbackAdapterIPAddress,
		},
		{
			name:              "network container that skips default routes gets only the dummy default route",
			skipDefaultRoutes: true,
			wantNextHop:       "0.0.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			localIPConfig := cns.IPConfiguration{
				IPSubnet:         cns.IPSubnet{IPAddress: "169.254.128.4", PrefixLength: 17},
				GatewayIPAddress: "169.254.128.2",
			}
			endpoint, err := configureHostNCApipaEndpoint("ep", "network", localIPConfig, true, true, tt.skipDefaultRoutes, nil)
			require.NoError(t, err)
			require.Len(t, endpoint.Routes, 1)
			assert.Equal(t, "0.0.0.0/0", endpoint.Routes[0].DestinationPrefix)
			assert.Equal(t, tt.wantNextHop, endpoint.Routes[0].NextHop)
		})
	}
}
