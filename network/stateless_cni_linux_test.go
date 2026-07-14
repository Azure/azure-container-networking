//go:build linux
// +build linux

package network

import (
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/stretchr/testify/require"
)

// TestStatelessCNI_Delete_Linux_TransparentMode tests DELETE flow on Linux in transparent mode
func TestStatelessCNI_Delete_Linux_TransparentMode(t *testing.T) {
	// Create mock netlink - this will be used by DeleteEndpointStateless
	mockNetlink := netlink.NewMockNetlink(false, "")

	// Track if DeleteLink was called (for veth deletion)
	var deletedLinks []string
	mockNetlink.DeleteLinkFn = func(name string) error {
		deletedLinks = append(deletedLinks, name)
		return nil
	}

	// Create network manager with mock netlink
	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
		netlink:            mockNetlink,
		plClient:           platform.NewMockExecClient(false),
		netio:              &netio.MockNetIO{},
	}

	// Set up endpoint info for transparent mode on Linux
	containerID := "test-stateless-container-linux"
	epInfo := &EndpointInfo{
		EndpointID:   containerID,
		ContainerID:  containerID,
		Data:         make(map[string]interface{}),
		IfName:       "eth0",
		NICType:      cns.InfraNIC,
		HostIfName:   "veth-host-1", // Linux uses HostVethName
		HNSNetworkID: "",            // No HNS on Linux
		MacAddress:   net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		IPAddresses: []net.IPNet{
			{IP: net.ParseIP("10.0.0.10"), Mask: net.CIDRMask(24, 32)},
		},
	}

	// Execute DeleteEndpointStateless
	err := nm.DeleteEndpointStateless("azure-test-network", epInfo, opModeTransparent)
	require.NoError(t, err)

	// TransparentEndpointClient.DeleteEndpoints() intentionally does not delete the veth -
	// the CRI removes the network namespace which automatically cleans up the veth pair.
	// Verify no links were deleted by the endpoint client.
	require.Empty(t, deletedLinks, "TransparentEndpointClient should not delete links directly (CRI handles cleanup)")
}

// Tests for stateless CNI DELETE operations on Linux using MockNetlink and MockNamespaceClient.

// TestStatelessCNI_Delete_Linux_FrontendNIC tests DELETE for SwiftV2 FrontendNIC on Linux
func TestStatelessCNI_Delete_Linux_FrontendNIC(t *testing.T) {
	mockNetlink := netlink.NewMockNetlink(false, "")

	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
		netlink:            mockNetlink,
		plClient:           platform.NewMockExecClient(false),
		netio:              &netio.MockNetIO{},
		nsClient:           NewMockNamespaceClient(),
	}

	// Set up endpoint info for SwiftV2 FrontendNIC
	containerID := "swiftv2-frontend-container"
	epInfo := &EndpointInfo{
		EndpointID:   containerID + "-eth1",
		ContainerID:  containerID,
		Data:         make(map[string]interface{}),
		IfName:       "eth1",
		NICType:      cns.NodeNetworkInterfaceFrontendNIC,
		HostIfName:   "veth-frontend",
		NetNsPath:    "/var/run/netns/test-ns", // MockNamespaceClient will handle this
		HNSNetworkID: "",
		MacAddress:   net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
		IPAddresses: []net.IPNet{
			{IP: net.ParseIP("10.1.0.10"), Mask: net.CIDRMask(24, 32)},
		},
	}

	// Execute DeleteEndpointStateless
	// SecondaryEndpointClient is created internally and handles the FrontendNIC
	err := nm.DeleteEndpointStateless("azure-frontend-network", epInfo, opModeTransparent)
	require.NoError(t, err)
}

// TestStatelessCNI_Delete_Linux_BackendNIC tests that BackendNIC is skipped on Linux
func TestStatelessCNI_Delete_Linux_BackendNIC(t *testing.T) {
	mockNetlink := netlink.NewMockNetlink(false, "")

	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
		netlink:            mockNetlink,
		plClient:           platform.NewMockExecClient(false),
		netio:              &netio.MockNetIO{},
	}

	containerID := "backend-nic-container-linux"
	epInfo := &EndpointInfo{
		EndpointID:  containerID + "-eth2",
		ContainerID: containerID,
		Data:        make(map[string]interface{}),
		IfName:      "eth2",
		NICType:     cns.BackendNIC,
		HostIfName:  "veth-backend",
		MacAddress:  net.HardwareAddr{0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00},
	}

	// Delete endpoint - BackendNIC should be skipped (same as Windows)
	err := nm.DeleteEndpointStateless("azure-backend-network", epInfo, opModeTransparent)
	require.NoError(t, err)

	// BackendNIC endpoint deletion is intentionally skipped in deleteEndpointImpl
	// (see endpoint_linux.go - "endpoint deletion is not required for IB")
}

// TestStatelessCNI_Delete_Linux_NetlinkErrorIgnored verifies that netlink errors during
// FrontendNIC deletion are intentionally swallowed (deleteEndpointImpl ignores DeleteEndpoints errors).
// This ensures cleanup continues even when network namespace operations fail.
func TestStatelessCNI_Delete_Linux_NetlinkErrorIgnored(t *testing.T) {
	// Create mock netlink that returns errors - this will fail SetLinkNetNs
	mockNetlink := netlink.NewMockNetlink(true, "simulated netlink failure")

	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
		netlink:            mockNetlink,
		plClient:           platform.NewMockExecClient(false),
		netio:              &netio.MockNetIO{},
		nsClient:           NewMockNamespaceClient(), // Required for SecondaryEndpointClient
	}

	containerID := "error-test-container"
	epInfo := &EndpointInfo{
		EndpointID:  containerID + "-eth1",
		ContainerID: containerID,
		Data:        make(map[string]interface{}),
		IfName:      "eth1",
		NICType:     cns.NodeNetworkInterfaceFrontendNIC, // FrontendNIC uses SecondaryEndpointClient which calls netlink
		HostIfName:  "veth-error",
		NetNsPath:   "/var/run/netns/test-ns", // Required for namespace operations
		MacAddress:  net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
	}

	// Execute - even though netlink.SetLinkNetNs fails, deleteEndpointImpl intentionally
	// ignores errors from epClient.DeleteEndpoints (see //nolint:errcheck comment in endpoint_linux.go)
	// This is by design: cleanup should continue even if namespace operations fail
	err := nm.DeleteEndpointStateless("azure-error-network", epInfo, opModeTransparent)
	require.NoError(t, err)
}

// =============================================================================
// Tests for REAL networkManager using CNSClient interface
// =============================================================================

// TestNetworkManager_GetEndpointState tests the real networkManager.GetEndpointState
// using the CNSClient interface with MockCNSEndpointClient injected.
func TestNetworkManager_GetEndpointState(t *testing.T) {
	tests := []struct {
		name             string
		containerID      string
		setupMock        func(*MockCNSEndpointClient)
		expectedCount    int
		expectedErr      error
		expectedNICTypes []cns.NICType
	}{
		{
			name:        "Success - InfraNIC endpoint",
			containerID: "infra-container",
			setupMock: func(mockCNS *MockCNSEndpointClient) {
				mockCNS.SetEndpointStateWithIPInfo("infra-container", "test-pod", "test-ns", map[string]*restserver.IPInfo{
					"eth0": CreateMockIPInfo(cns.InfraNIC, "10.0.0.5/24", "", "", "veth-host", ""),
				})
			},
			expectedCount:    1,
			expectedErr:      nil,
			expectedNICTypes: []cns.NICType{cns.InfraNIC},
		},
		{
			name:        "Success - MultiNIC (InfraNIC + FrontendNIC)",
			containerID: "multi-nic-container",
			setupMock: func(mockCNS *MockCNSEndpointClient) {
				mockCNS.SetEndpointStateWithIPInfo("multi-nic-container", "test-pod", "test-ns", map[string]*restserver.IPInfo{
					"eth0": CreateMockIPInfo(cns.InfraNIC, "10.0.0.5/24", "", "", "veth-infra", ""),
					"eth1": CreateMockIPInfo(cns.NodeNetworkInterfaceFrontendNIC, "20.20.20.20/32", "", "", "", "aa:bb:cc:dd:ee:ff"),
				})
			},
			expectedCount:    2,
			expectedErr:      nil,
			expectedNICTypes: []cns.NICType{cns.InfraNIC, cns.NodeNetworkInterfaceFrontendNIC},
		},
		{
			name:        "Endpoint not found",
			containerID: "nonexistent-container",
			setupMock: func(_ *MockCNSEndpointClient) {
				// Don't set up any state - endpoint will not be found
			},
			expectedCount: 0,
			expectedErr:   ErrEndpointStateNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock CNS client
			mockCNS := NewMockCNSEndpointClient()
			tt.setupMock(mockCNS)

			// Create REAL networkManager with mock CNS client injected
			nm := &networkManager{
				statelessCniMode: true,
				CnsClient:        mockCNS, // Inject mock via CNSClient interface
			}

			// Call GetEndpointState on the REAL networkManager
			epInfos, err := nm.GetEndpointState("", tt.containerID, "test-netns")

			// Verify error
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
			} else {
				require.NoError(t, err)
			}

			// Verify endpoint count
			require.Len(t, epInfos, tt.expectedCount)

			// Verify NIC types if we have endpoints
			if tt.expectedCount > 0 {
				foundNICTypes := make(map[cns.NICType]bool)
				for _, ep := range epInfos {
					foundNICTypes[ep.NICType] = true
				}
				for _, expectedType := range tt.expectedNICTypes {
					require.True(t, foundNICTypes[expectedType], "Expected NIC type %v not found", expectedType)
				}
			}

			// Verify GetEndpoint was called on the mock
			require.Contains(t, mockCNS.GetEndpointCalls, tt.containerID)
		})
	}
}

// TestNetworkManager_DeleteState tests the real networkManager.DeleteState
// using the CNSClient interface with MockCNSEndpointClient injected.
func TestNetworkManager_DeleteState(t *testing.T) {
	tests := []struct {
		name          string
		epInfos       []*EndpointInfo
		setupMock     func(*MockCNSEndpointClient)
		validateAfter func(*testing.T, *MockCNSEndpointClient)
		expectErr     bool
	}{
		{
			name: "With InfraNIC - state already deleted by IPAM (no CNS call)",
			epInfos: []*EndpointInfo{
				{EndpointID: "infra-ep", NICType: cns.InfraNIC},
				{EndpointID: "frontend-ep", NICType: cns.NodeNetworkInterfaceFrontendNIC},
			},
			setupMock: func(_ *MockCNSEndpointClient) {
				// State doesn't matter - when InfraNIC is present, DeleteState returns early
			},
			validateAfter: func(t *testing.T, mockCNS *MockCNSEndpointClient) {
				// When InfraNIC is present, DeleteState returns early without calling CNS
				// because IPAM invoker already deleted the state
				require.Empty(t, mockCNS.DeleteEndpointStateCalls, "DeleteEndpointState should not be called when InfraNIC present")
			},
			expectErr: false,
		},
		{
			name: "FrontendNIC only - calls DeleteEndpointState",
			epInfos: []*EndpointInfo{
				{EndpointID: "frontend-ep", NICType: cns.NodeNetworkInterfaceFrontendNIC},
			},
			setupMock: func(mockCNS *MockCNSEndpointClient) {
				mockCNS.EndpointState["frontend-ep"] = &restserver.EndpointInfo{}
			},
			validateAfter: func(t *testing.T, mockCNS *MockCNSEndpointClient) {
				// Without InfraNIC, DeleteState should call DeleteEndpointState on CNS
				require.Contains(t, mockCNS.DeleteEndpointStateCalls, "frontend-ep",
					"DeleteEndpointState should be called for standalone FrontendNIC")
			},
			expectErr: false,
		},
		{
			name: "AccelnetFrontendNIC - calls DeleteEndpointState",
			epInfos: []*EndpointInfo{
				{EndpointID: "accelnet-ep", NICType: cns.NodeNetworkInterfaceAccelnetFrontendNIC},
			},
			setupMock: func(mockCNS *MockCNSEndpointClient) {
				mockCNS.EndpointState["accelnet-ep"] = &restserver.EndpointInfo{}
			},
			validateAfter: func(t *testing.T, mockCNS *MockCNSEndpointClient) {
				require.Contains(t, mockCNS.DeleteEndpointStateCalls, "accelnet-ep",
					"DeleteEndpointState should be called for AccelnetFrontendNIC")
			},
			expectErr: false,
		},
		{
			name: "BackendNIC only - no CNS call (not FrontendNIC type)",
			epInfos: []*EndpointInfo{
				{EndpointID: "backend-ep", NICType: cns.BackendNIC},
			},
			setupMock: func(_ *MockCNSEndpointClient) {},
			validateAfter: func(t *testing.T, mockCNS *MockCNSEndpointClient) {
				// BackendNIC doesn't trigger DeleteEndpointState (only FrontendNIC types do)
				require.Empty(t, mockCNS.DeleteEndpointStateCalls)
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCNS := NewMockCNSEndpointClient()
			tt.setupMock(mockCNS)

			// Create REAL networkManager with mock CNS client injected
			nm := &networkManager{
				statelessCniMode: true,
				CnsClient:        mockCNS, // Inject mock via CNSClient interface
			}

			// Call DeleteState on the REAL networkManager
			err := nm.DeleteState(tt.epInfos)

			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			tt.validateAfter(t, mockCNS)
		})
	}
}

// TestNetworkManager_UpdateEndpointState tests the real networkManager.UpdateEndpointState
// (called via SaveState) using the CNSClient interface.
func TestNetworkManager_UpdateEndpointState(t *testing.T) {
	mockCNS := NewMockCNSEndpointClient()

	nm := &networkManager{
		statelessCniMode: true,
		CnsClient:        mockCNS,
	}

	// Create test endpoints
	eps := []*endpoint{
		{
			ContainerID:        "test-container",
			IfName:             "eth0",
			NICType:            cns.InfraNIC,
			HnsId:              "hns-endpoint-123",
			HNSNetworkID:       "hns-network-456",
			HostIfName:         "veth-host",
			MacAddress:         net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
			NetworkContainerID: "nc-789",
		},
	}

	// Call SaveState which internally calls UpdateEndpointState
	err := nm.SaveState(eps)
	require.NoError(t, err)

	// Verify UpdateEndpoint was called with correct data
	require.Len(t, mockCNS.UpdateEndpointCalls, 1)
	require.Equal(t, "test-container", mockCNS.UpdateEndpointCalls[0].EndpointID)
	require.Contains(t, mockCNS.UpdateEndpointCalls[0].IPInfo, "eth0")

	ipInfo := mockCNS.UpdateEndpointCalls[0].IPInfo["eth0"]
	require.Equal(t, cns.InfraNIC, ipInfo.NICType)
	require.Equal(t, "hns-endpoint-123", ipInfo.HnsEndpointID)
	require.Equal(t, "veth-host", ipInfo.HostVethName)
}
