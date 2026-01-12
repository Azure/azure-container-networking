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
)

func TestProcessIMDSData_EmptyInterfaces(t *testing.T) {
	service := &HTTPRestService{}

	result := service.processIMDSData([]imds.NetworkInterface{})

	if len(result) != 0 {
		t.Errorf("Expected empty result for empty input, got %d NCs", len(result))
	}
}

func TestProcessIMDSData_SwiftV2PrefixOnNicEnabled(t *testing.T) {
	ncID := "nc-id-1"
	delegatedMacAddr, _ := net.ParseMAC("00:15:5D:01:02:03")
	infraMacAddr, _ := net.ParseMAC("00:15:5D:01:02:FF")

	service := &HTTPRestService{}

	interfaces := []imds.NetworkInterface{
		{
			MacAddress:             imds.HardwareAddr(delegatedMacAddr),
			InterfaceCompartmentID: ncID,
		},
		{
			MacAddress:             imds.HardwareAddr(infraMacAddr),
			InterfaceCompartmentID: "",
		},
	}

	result := service.processIMDSData(interfaces)

	assert.Len(t, result, 1, "Expected one NC in result")
	assert.Equal(t, PrefixOnNicNCVersion, result[ncID], "NC should have expected version")
}

func TestProcessIMDSData_InfraNICOnly(t *testing.T) {
	infraMacAddr, _ := net.ParseMAC("00:15:5D:01:02:FF")

	service := &HTTPRestService{}

	// Only infra NIC interface (empty NC ID)
	interfaces := []imds.NetworkInterface{
		{
			MacAddress:             imds.HardwareAddr(infraMacAddr),
			InterfaceCompartmentID: "",
		},
	}

	result := service.processIMDSData(interfaces)

	if len(result) != 0 {
		t.Errorf("Expected empty result for infra NIC only, got %d NCs", len(result))
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
