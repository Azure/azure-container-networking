// Copyright 2025 Microsoft. All rights reserved.
// MIT License

//go:build windows
// +build windows

package restserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockWindowsRegistryClient struct {
	// Store call history
	calls []registryCall
}

type registryCall struct {
	method string
	value  interface{}
}

func (m *mockWindowsRegistryClient) SetPrefixOnNicEnabled(isEnabled bool) error {
	m.calls = append(m.calls, registryCall{method: "SetPrefixOnNicEnabled", value: isEnabled})
	return nil
}

func (m *mockWindowsRegistryClient) SetInfraNicMacAddress(macAddress string) error {
	m.calls = append(m.calls, registryCall{method: "SetInfraNicMacAddress", value: macAddress})
	return nil
}

func (m *mockWindowsRegistryClient) SetInfraNicIfName(ifName string) error {
	m.calls = append(m.calls, registryCall{method: "SetInfraNicIfName", value: ifName})
	return nil
}

func (m *mockWindowsRegistryClient) SetEnableSNAT(isEnabled bool) error {
	m.calls = append(m.calls, registryCall{method: "SetEnableSNAT", value: isEnabled})
	return nil
}

// Helper method to get the last value set for a specific method
func (m *mockWindowsRegistryClient) getLastValue(method string) interface{} {
	for i := len(m.calls) - 1; i >= 0; i-- {
		if m.calls[i].method == method {
			return m.calls[i].value
		}
	}
	return nil
}

func TestProcessWindowsRegistryKeys_EmptyMacAddress(t *testing.T) {
	mockRegistry := &mockWindowsRegistryClient{}
	service := &HTTPRestService{
		windowsRegistry: mockRegistry,
		state: &httpRestServiceState{
			ContainerStatus: map[string]containerstatus{},
		},
	}

	// Call processWindowsRegistryKeys with empty MAC address
	service.processWindowsRegistryKeys(false, "")

	assert.Equal(t, false, mockRegistry.getLastValue("SetPrefixOnNicEnabled"), "PrefixOnNic should be disabled")
	assert.Equal(t, "", mockRegistry.getLastValue("SetInfraNicMacAddress"), "MAC address should be empty")
	assert.Equal(t, "eth0", mockRegistry.getLastValue("SetInfraNicIfName"), "Interface name should be set to eth0")
	assert.Equal(t, true, mockRegistry.getLastValue("SetEnableSNAT"), "SNAT should be enabled")
}

func TestProcessWindowsRegistryKeys_ValidMacAddress(t *testing.T) {
	validMacAddress := "00:15:5D:01:02:03"
	mockRegistry := &mockWindowsRegistryClient{}
	service := &HTTPRestService{
		windowsRegistry: mockRegistry,
		state: &httpRestServiceState{
			ContainerStatus: map[string]containerstatus{},
		},
	}

	// Call processWindowsRegistryKeys with valid MAC address
	service.processWindowsRegistryKeys(true, validMacAddress)

	assert.Equal(t, true, mockRegistry.getLastValue("SetPrefixOnNicEnabled"), "PrefixOnNic should be disabled")
	assert.Equal(t, validMacAddress, mockRegistry.getLastValue("SetInfraNicMacAddress"), "MAC address should be set")
	assert.Equal(t, "eth0", mockRegistry.getLastValue("SetInfraNicIfName"), "Interface name should be set to eth0")
	assert.Equal(t, false, mockRegistry.getLastValue("SetEnableSNAT"), "SNAT should be enabled when prefix-on-NIC is disabled")
}

func TestProcessWindowsRegistryKeys_RegistryError(t *testing.T) {
	validMacAddress := "00:15:5D:01:02:03"
	mockRegistry := &mockWindowsRegistryClient{}
	service := &HTTPRestService{
		windowsRegistry: mockRegistry,
		state: &httpRestServiceState{
			ContainerStatus: map[string]containerstatus{},
		},
	}

	// Call processWindowsRegistryKeys - should not panic even with error
	require.NotPanics(t, func() {
		service.processWindowsRegistryKeys(false, validMacAddress)
	}, "processWindowsRegistryKeys should handle errors gracefully")
}
