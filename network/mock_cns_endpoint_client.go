// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"context"
	"errors"
	"net"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/cns/types"
)

// Compile-time check that MockCNSEndpointClient implements CNSClient interface
var _ CNSClient = (*MockCNSEndpointClient)(nil)

// MockCNSEndpointClient is a mock implementation of the CNSClient interface for testing.
// It can be injected into networkManager.CnsClient to unit test GetEndpointState, DeleteState, etc.
//
// USAGE IN TESTS:
//
// For testing real networkManager methods (GetEndpointState, DeleteState, UpdateEndpointState):
//
//	mockCNS := NewMockCNSEndpointClient()
//	mockCNS.SetEndpointStateWithIPInfo("container123", "pod", "ns", map[string]*restserver.IPInfo{
//	    "eth0": CreateMockIPInfo(cns.InfraNIC, "10.0.0.5/24", "", "", "veth-host", ""),
//	})
//	nm := &networkManager{
//	    statelessCniMode: true,
//	    CnsClient:        mockCNS,  // Inject mock
//	}
//	epInfos, err := nm.GetEndpointState("", "container123", "netns")
//
// FEATURES:
//   - Method call tracking (GetEndpointCalls, DeleteEndpointStateCalls, etc.)
//   - Error injection (GetEndpointErr, DeleteEndpointStateErr)
//   - State storage in CNS format (restserver.IPInfo with NICType, IPv4, etc.)
//   - Uses real cnsEndpointInfotoCNIEpInfos for format conversion testing
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
	GetEndpointStateCalls    []string
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
		GetEndpointStateCalls:    []string{},
		UpdateEndpointCalls:      []UpdateEndpointCall{},
		DeleteEndpointStateCalls: []string{},
	}
}

// GetEndpoint retrieves the endpoint state from the mock store
func (m *MockCNSEndpointClient) GetEndpoint(_ context.Context, containerID string) (*restserver.GetEndpointResponse, error) {
	m.GetEndpointCalls = append(m.GetEndpointCalls, containerID)

	// Return error if configured
	if m.GetEndpointErr != nil {
		returnCode := m.GetEndpointReturnCode
		// If ReturnCode wasn't explicitly set (defaults to 0/Success), infer from error type
		// to match production behavior where errors always have appropriate return codes.
		if returnCode == types.Success {
			returnCode = m.inferReturnCodeFromError(m.GetEndpointErr)
		}
		return &restserver.GetEndpointResponse{
			Response: restserver.Response{
				ReturnCode: returnCode,
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

// inferReturnCodeFromError maps common errors to their appropriate ReturnCode values
// to match production behavior where errors always have specific return codes.
func (m *MockCNSEndpointClient) inferReturnCodeFromError(err error) types.ResponseCode {
	if err == nil {
		return types.Success
	}
	switch {
	case errors.Is(err, ErrEndpointStateNotFound):
		return types.NotFound
	case errors.Is(err, ErrConnectionFailure):
		return types.ConnectionError
	default:
		// For unknown errors, use a generic failure code
		return types.UnexpectedError
	}
}

// SetNotFoundError configures the mock to return a NotFound error with the appropriate ReturnCode.
// This helper ensures tests match production behavior where GetEndpoint returns types.NotFound
// when an endpoint doesn't exist.
func (m *MockCNSEndpointClient) SetNotFoundError() {
	m.GetEndpointErr = ErrEndpointStateNotFound
	m.GetEndpointReturnCode = types.NotFound
}

// SetConnectionError configures the mock to return a ConnectionError with the appropriate ReturnCode.
// This helper ensures tests match production behavior where GetEndpoint returns types.ConnectionError
// when CNS connectivity fails.
func (m *MockCNSEndpointClient) SetConnectionError() {
	m.GetEndpointErr = ErrConnectionFailure
	m.GetEndpointReturnCode = types.ConnectionError
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
func CreateMockIPInfo(nicType cns.NICType, ipv4, hnsEndpointID, hnsNetworkID, hostVethName, macAddress string) *restserver.IPInfo {
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

// GetEndpointState returns the endpoint state in EndpointInfo format for MockNetworkManager.
// This uses the production cnsEndpointInfotoCNIEpInfos function to ensure consistent behavior
// with production code, including legacy/unmigrated case handling where IfnameToIPMap key is "".
// Matches the production networkManager.GetEndpointState behavior - returns ErrEndpointStateNotFound when not found.
func (m *MockCNSEndpointClient) GetEndpointState(containerID, netns string) ([]*EndpointInfo, error) {
	// Track method calls for verification
	m.GetEndpointStateCalls = append(m.GetEndpointStateCalls, containerID)

	// Return error if configured
	if m.GetEndpointErr != nil {
		return nil, m.GetEndpointErr
	}

	// Check if endpoint state exists for this containerID
	// Production behavior: returns ErrEndpointStateNotFound when endpoint is not in state
	epInfo, exists := m.EndpointState[containerID]
	if !exists || epInfo == nil {
		return []*EndpointInfo{}, ErrEndpointStateNotFound
	}

	// Use the production conversion function to ensure consistent behavior with production code.
	// This handles legacy cases (empty ifName mapped to InfraInterfaceName with NICType=InfraNIC)
	// and populates all fields that production does (IfIndex, NetworkContainerID, etc.)
	return cnsEndpointInfotoCNIEpInfos(*epInfo, containerID, netns), nil
}
