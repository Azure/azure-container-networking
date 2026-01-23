// Copyright 2025 Microsoft. All rights reserved.
// MIT License

//go:build windows
// +build windows

package restserver

import (
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/imds"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessIMDSData_EmptyInterfaces(t *testing.T) {
	service := &HTTPRestService{}

	result, err := service.processIMDSData([]imds.NetworkInterface{})
	require.NoError(t, err)

	if len(result) != 0 {
		t.Errorf("Expected empty result for empty input, got %d NCs", len(result))
	}
}

func TestProcessIMDSData_InfraNICOnly(t *testing.T) {
	infraMacAddr, _ := net.ParseMAC("00:15:5D:01:02:FF")

	service := &HTTPRestService{
		state: &httpRestServiceState{
			ContainerStatus: map[string]containerstatus{},
		},
	}

	// Only infra NIC interface (empty NC ID)
	interfaces := []imds.NetworkInterface{
		{
			MacAddress:             imds.HardwareAddr(infraMacAddr),
			InterfaceCompartmentID: "",
		},
	}

	result, err := service.processIMDSData(interfaces)

	if err == nil {
		if len(result) != 0 {
			t.Errorf("Expected empty result for infra NIC only, got %d NCs", len(result))
		}
	}
}

func TestIsSwiftV2PrefixOnNicEnabled_NCExists_Enabled(t *testing.T) {
	ncID := "test-nc-id"
	service := &HTTPRestService{
		state: &httpRestServiceState{
			ContainerStatus: map[string]containerstatus{
				ncID: {
					ID: ncID,
					CreateNetworkContainerRequest: cns.CreateNetworkContainerRequest{
						SwiftV2PrefixOnNic: true,
					},
				},
			},
		},
	}

	result := service.isSwiftV2PrefixOnNicEnabled(ncID)
	assert.True(t, result, "Expected SwiftV2PrefixOnNic to be enabled")
}

func TestIsSwiftV2PrefixOnNicEnabled_NCExists_Disabled(t *testing.T) {
	ncID := "test-nc-id"
	service := &HTTPRestService{
		state: &httpRestServiceState{
			ContainerStatus: map[string]containerstatus{
				ncID: {
					ID: ncID,
					CreateNetworkContainerRequest: cns.CreateNetworkContainerRequest{
						SwiftV2PrefixOnNic: false,
					},
				},
			},
		},
	}

	result := service.isSwiftV2PrefixOnNicEnabled(ncID)
	assert.False(t, result, "Expected SwiftV2PrefixOnNic to be disabled")
}

func TestIsSwiftV2PrefixOnNicEnabled_NCDoesNotExist(t *testing.T) {
	service := &HTTPRestService{
		state: &httpRestServiceState{
			ContainerStatus: map[string]containerstatus{},
		},
	}

	result := service.isSwiftV2PrefixOnNicEnabled("non-existent-nc")
	assert.False(t, result, "Expected false when NC does not exist")
}

func TestIsSwiftV2PrefixOnNicEnabled_NilState(t *testing.T) {
	service := &HTTPRestService{}

	// This should not panic even with nil state
	result := service.isSwiftV2PrefixOnNicEnabled("any-nc-id")
	assert.False(t, result, "Expected false when state is nil")
}

func TestGetInterfaceNameFromMAC_InvalidMACFormats(t *testing.T) {
	service := &HTTPRestService{}

	tests := []struct {
		name        string
		macAddress  string
		expectError bool
		errContains string
	}{
		{
			name:        "invalid hex characters",
			macAddress:  "GGHHIIJJKKLL",
			expectError: true,
			errContains: "failed to parse MAC address",
		},
		{
			name:        "invalid hex with valid format",
			macAddress:  "GG:HH:II:JJ:KK:LL",
			expectError: true,
			errContains: "failed to parse MAC address",
		},
		{
			name:        "empty string",
			macAddress:  "",
			expectError: true,
			errContains: "empty MAC address",
		},
		{
			name:        "valid format but with Invalid MAC",
			macAddress:  "001122334455",
			expectError: true,
			errContains: "failed to find interface",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.getInterfaceNameFromMAC(tt.macAddress)

			if tt.expectError {
				require.Error(t, err, "Expected error for MAC: %s", tt.macAddress)
				assert.Contains(t, err.Error(), tt.errContains, "Error should contain expected message")
				assert.Empty(t, result, "Result should be empty on error")
			} else {
				require.NoError(t, err, "Did not expect error for MAC: %s", tt.macAddress)
				assert.NotEmpty(t, result, "Result should not be empty on success")
			}
		})
	}
}

func TestGetInterfaceNameFromMAC_HappyPath(t *testing.T) {
	service := &HTTPRestService{}
	macAddress := "00:15:5D:01:02:03"

	result, err := service.getInterfaceNameFromMAC(macAddress)

	if err != nil {
		assert.Contains(t, err.Error(), "failed to find interface", "Should fail on lookup, not parsing")
	} else {
		assert.NotEmpty(t, result, "Result should not be empty on success")
	}
}
