// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"context"
	"net"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/cns/types"
)

// MockCNSEndpointClient is a mock implementation of CNSEndpointClient for testing stateless CNI
type MockCNSEndpointClient struct {
	// EndpointState stores the mock endpoint state, keyed by containerID
	EndpointState map[string]*restserver.EndpointInfo

	// Error injection fields
	GetEndpointErr         error
	GetEndpointReturnCode  types.ResponseCode
	UpdateEndpointErr      error
	DeleteEndpointStateErr error

	// Track method calls for verification
	GetEndpointCalls         []string
	UpdateEndpointCalls      []UpdateEndpointCall
	DeleteEndpointStateCalls []string
}

// UpdateEndpointCall stores the arguments passed to UpdateEndpoint
type UpdateEndpointCall struct {
	EndpointID string
	IPInfo     map[string]*restserver.IPInfo
}

// NewMockCNSEndpointClient creates a new MockCNSEndpointClient with empty state
func NewMockCNSEndpointClient() *MockCNSEndpointClient {
	return &MockCNSEndpointClient{
		EndpointState:            make(map[string]*restserver.EndpointInfo),
		GetEndpointCalls:         []string{},
		UpdateEndpointCalls:      []UpdateEndpointCall{},
		DeleteEndpointStateCalls: []string{},
	}
}

// GetEndpoint retrieves the endpoint state from the mock store
func (m *MockCNSEndpointClient) GetEndpoint(_ context.Context, containerID string) (*restserver.GetEndpointResponse, error) {
	m.GetEndpointCalls = append(m.GetEndpointCalls, containerID)

	// Return error if configured
	if m.GetEndpointErr != nil {
		return &restserver.GetEndpointResponse{
			Response: restserver.Response{
				ReturnCode: m.GetEndpointReturnCode,
				Message:    m.GetEndpointErr.Error(),
			},
		}, m.GetEndpointErr
	}

	// Check if endpoint exists in mock state
	endpointInfo, exists := m.EndpointState[containerID]
	if !exists {
		return &restserver.GetEndpointResponse{
			Response: restserver.Response{
				ReturnCode: types.NotFound,
				Message:    "endpoint not found",
			},
		}, ErrEndpointStateNotFound
	}

	return &restserver.GetEndpointResponse{
		Response: restserver.Response{
			ReturnCode: types.Success,
			Message:    "success",
		},
		EndpointInfo: *endpointInfo,
	}, nil
}

// UpdateEndpoint updates the endpoint state in the mock store
func (m *MockCNSEndpointClient) UpdateEndpoint(_ context.Context, endpointID string, ipInfo map[string]*restserver.IPInfo) (*cns.Response, error) {
	m.UpdateEndpointCalls = append(m.UpdateEndpointCalls, UpdateEndpointCall{
		EndpointID: endpointID,
		IPInfo:     ipInfo,
	})

	// Return error if configured
	if m.UpdateEndpointErr != nil {
		return nil, m.UpdateEndpointErr
	}

	// Create or update endpoint state
	if _, exists := m.EndpointState[endpointID]; !exists {
		m.EndpointState[endpointID] = &restserver.EndpointInfo{
			IfnameToIPMap: make(map[string]*restserver.IPInfo),
		}
	}

	// Merge IPInfo into existing state
	for ifName, info := range ipInfo {
		m.EndpointState[endpointID].IfnameToIPMap[ifName] = info
	}

	return &cns.Response{
		ReturnCode: types.Success,
		Message:    "success",
	}, nil
}

// DeleteEndpointState deletes the endpoint state from the mock store
func (m *MockCNSEndpointClient) DeleteEndpointState(_ context.Context, endpointID string) (*cns.Response, error) {
	m.DeleteEndpointStateCalls = append(m.DeleteEndpointStateCalls, endpointID)

	// Return error if configured
	if m.DeleteEndpointStateErr != nil {
		return nil, m.DeleteEndpointStateErr
	}

	// Delete endpoint state
	delete(m.EndpointState, endpointID)

	return &cns.Response{
		ReturnCode: types.Success,
		Message:    "success",
	}, nil
}

// SetEndpointState is a helper to set up mock endpoint state for testing
func (m *MockCNSEndpointClient) SetEndpointState(containerID string, endpointInfo *restserver.EndpointInfo) {
	m.EndpointState[containerID] = endpointInfo
}

// SetEndpointStateWithIPInfo is a helper to set up mock endpoint state with specific IP info
func (m *MockCNSEndpointClient) SetEndpointStateWithIPInfo(containerID, podName, podNamespace string, ifnameToIPMap map[string]*restserver.IPInfo) {
	m.EndpointState[containerID] = &restserver.EndpointInfo{
		PodName:       podName,
		PodNamespace:  podNamespace,
		IfnameToIPMap: ifnameToIPMap,
	}
}

// CreateMockIPInfo is a helper to create IPInfo for testing
func CreateMockIPInfo(nicType cns.NICType, ipv4 string, hnsEndpointID, hnsNetworkID, hostVethName, macAddress string) *restserver.IPInfo {
	ipInfo := &restserver.IPInfo{
		NICType:       nicType,
		HnsEndpointID: hnsEndpointID,
		HnsNetworkID:  hnsNetworkID,
		HostVethName:  hostVethName,
		MacAddress:    macAddress,
	}

	if ipv4 != "" {
		ip, ipNet, _ := net.ParseCIDR(ipv4)
		if ipNet != nil {
			// Use the original IP with the network mask (not the network address)
			ipNet.IP = ip
			ipInfo.IPv4 = []net.IPNet{*ipNet}
		}
	}

	return ipInfo
}

// GetEndpointState returns the endpoint state in EndpointInfo format for MockNetworkManager
// This converts the CNS EndpointInfo to the network package's EndpointInfo format
func (m *MockCNSEndpointClient) GetEndpointState(containerID, ifName string) ([]*EndpointInfo, error) {
	// Return error if configured
	if m.GetEndpointErr != nil {
		return nil, m.GetEndpointErr
	}

	endpointInfos := make([]*EndpointInfo, 0)

	// Check if endpoint state exists for this containerID
	epInfo, exists := m.EndpointState[containerID]
	if !exists || epInfo == nil {
		return endpointInfos, nil
	}

	// Convert CNS endpoint state to EndpointInfo for each interface
	for ifname, ipInfo := range epInfo.IfnameToIPMap {
		// If specific ifName is requested, filter
		if ifName != "" && ifname != ifName {
			continue
		}

		endpointInfo := &EndpointInfo{
			ContainerID:   containerID,
			IfName:        ifname,
			NICType:       ipInfo.NICType,
			HNSEndpointID: ipInfo.HnsEndpointID,
			HNSNetworkID:  ipInfo.HnsNetworkID,
			HostIfName:    ipInfo.HostVethName,
			MacAddress:    net.HardwareAddr{},
		}

		// Parse MAC address if provided
		if ipInfo.MacAddress != "" {
			mac, err := net.ParseMAC(ipInfo.MacAddress)
			if err == nil {
				endpointInfo.MacAddress = mac
			}
		}

		// Convert IP addresses
		for _, ipNet := range ipInfo.IPv4 {
			ip := ipNet
			endpointInfo.IPAddresses = append(endpointInfo.IPAddresses, ip)
		}
		for _, ipNet := range ipInfo.IPv6 {
			ip := ipNet
			endpointInfo.IPAddresses = append(endpointInfo.IPAddresses, ip)
		}

		endpointInfos = append(endpointInfos, endpointInfo)
	}

	return endpointInfos, nil
}
