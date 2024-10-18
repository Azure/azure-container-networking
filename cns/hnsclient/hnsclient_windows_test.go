package hnsclient

import (
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/stretchr/testify/assert"
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
