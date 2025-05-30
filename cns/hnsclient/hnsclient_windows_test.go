//go:build windows
// +build windows

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

func TestShouldCreateLoopbackAdapter(t *testing.T) {
	tests := []struct {
		name                     string
		hostNCExists             bool
		vEthernetHostNCExists    bool
		expectedShouldCreate     bool
		expectedLogMessagePrefix string
	}{
		{
			name:                     "should create when neither interface exists",
			hostNCExists:             false,
			vEthernetHostNCExists:    false,
			expectedShouldCreate:     true,
			expectedLogMessagePrefix: "Creating loopback adapter",
		},
		{
			name:                     "should not create when hostNCLoopbackAdapterName exists",
			hostNCExists:             true,
			vEthernetHostNCExists:    false,
			expectedShouldCreate:     false,
			expectedLogMessagePrefix: "LoopbackAdapterHostNCConnectivity already created",
		},
		{
			name:                     "should not create when vEthernethostNCLoopbackAdapterName exists",
			hostNCExists:             false,
			vEthernetHostNCExists:    true,
			expectedShouldCreate:     false,
			expectedLogMessagePrefix: "vEthernet (LoopbackAdapterHostNCConnectivity) already created",
		},
		{
			name:                     "should not create when both interfaces exist - prioritizes hostNCLoopbackAdapterName",
			hostNCExists:             true,
			vEthernetHostNCExists:    true,
			expectedShouldCreate:     false,
			expectedLogMessagePrefix: "LoopbackAdapterHostNCConnectivity already created",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// Create mock interface exists function
			mockInterfaceExists := func(interfaceName string) (bool, error) {
				switch interfaceName {
				case hostNCLoopbackAdapterName:
					return tt.hostNCExists, nil
				case vEthernethostNCLoopbackAdapterName:
					return tt.vEthernetHostNCExists, nil
				default:
					return false, nil
				}
			}

			shouldCreate, logMessage := shouldCreateLoopbackAdapter(mockInterfaceExists)
			
			assert.Equal(t, tt.expectedShouldCreate, shouldCreate)
			assert.Contains(t, logMessage, tt.expectedLogMessagePrefix)
		})
	}
}

func TestConstants(t *testing.T) {
	// Test that the vEthernet constant is constructed correctly
	expectedVEthernetName := "vEthernet (LoopbackAdapterHostNCConnectivity)"
	assert.Equal(t, expectedVEthernetName, vEthernethostNCLoopbackAdapterName)
	
	// Test that the hostNCLoopbackAdapterName constant is as expected
	assert.Equal(t, "LoopbackAdapterHostNCConnectivity", hostNCLoopbackAdapterName)
}
