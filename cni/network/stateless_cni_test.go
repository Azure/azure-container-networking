package network

import (
	"fmt"
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/common"
	acnnetwork "github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/nns"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	"github.com/stretchr/testify/require"
)

const (
	testNamespace = "test-ns"
)

// GetStatelessTestResources creates a test NetPlugin configured for stateless CNI mode
func GetStatelessTestResources(t *testing.T) (*NetPlugin, *acnnetwork.MockNetworkManager) {
	pluginName := "testplugin"
	isIPv6 := false
	config := &common.PluginConfig{}
	grpcClient := &nns.MockGrpcClient{}
	plugin, err := NewPlugin(pluginName, config, grpcClient, &Multitenancy{})
	require.NoError(t, err, "NewPlugin should not fail")

	// Create mock network manager with stateless mode enabled
	mockNetworkManager := acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(nil))
	err = mockNetworkManager.SetStatelessCNIMode()
	require.NoError(t, err, "SetStatelessCNIMode should not fail")

	plugin.nm = mockNetworkManager
	plugin.ipamInvoker = NewMockIpamInvoker(isIPv6, false, false, false, false)

	return plugin, mockNetworkManager
}

// createStatelessTestConfig creates a network config for stateless CNI tests
func createStatelessTestConfig() cni.NetworkConfig {
	return cni.NetworkConfig{
		Name:              "test-stateless-nwcfg",
		CNIVersion:        "0.3.0",
		Type:              "azure-vnet",
		Mode:              OpModeTransparent,
		Master:            "eth0",
		IPsToRouteViaHost: []string{"169.254.20.10"},
		IPAM: struct {
			Mode          string `json:"mode,omitempty"`
			Type          string `json:"type"`
			Environment   string `json:"environment,omitempty"`
			AddrSpace     string `json:"addressSpace,omitempty"`
			Subnet        string `json:"subnet,omitempty"`
			Address       string `json:"ipAddress,omitempty"`
			QueryInterval string `json:"queryInterval,omitempty"`
		}{
			Type: "azure-cns",
		},
	}
}

// TestStatelessCNI_Delete_CNSGetEndpointError tests DELETE when CNS returns errors
func TestStatelessCNI_Delete_CNSGetEndpointError(t *testing.T) {
	plugin, mockNM := GetStatelessTestResources(t)
	nwCfgStateless := createStatelessTestConfig()

	tests := []struct {
		name            string
		setupError      func()
		wantErr         bool
		wantErrContains string
	}{
		{
			name: "CNS returns EndpointStateNotFound - should succeed",
			setupError: func() {
				mockNM.MockCNSClient.GetEndpointErr = acnnetwork.ErrEndpointStateNotFound
			},
			wantErr: false, // EndpointStateNotFound is handled gracefully
		},
		{
			name: "CNS returns ConnectionFailure - should succeed (async IP release)",
			setupError: func() {
				mockNM.MockCNSClient.GetEndpointErr = acnnetwork.ErrConnectionFailure
			},
			wantErr: false, // Connection failure handled with async release
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock
			mockNM.MockCNSClient = acnnetwork.NewMockCNSEndpointClient()

			// Set up error condition
			tt.setupError()

			cmdArgs := &cniSkel.CmdArgs{
				StdinData:   nwCfgStateless.Serialize(),
				ContainerID: "error-test-container",
				Netns:       "error-test-container",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "error-pod", testNamespace),
				IfName:      "eth0",
			}

			err := plugin.Delete(cmdArgs)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrContains != "" {
					require.Contains(t, err.Error(), tt.wantErrContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestStatelessCNI_Delete_HappyPath tests DELETE when CNS returns valid endpoint state
func TestStatelessCNI_Delete_HappyPath(t *testing.T) {
	nwCfgStateless := createStatelessTestConfig()
	containerID := "happy-path-container"
	podName := "test-pod"
	podNamespace := testNamespace

	tests := []struct {
		name          string
		setupState    func(*acnnetwork.MockCNSEndpointClient, *MockIpamInvoker)
		validateAfter func(*testing.T, *MockIpamInvoker)
		wantErr       bool
		description   string
	}{
		{
			name: "Delete InfraNIC endpoint - IP released",
			setupState: func(mockCNS *acnnetwork.MockCNSEndpointClient, mockIpam *MockIpamInvoker) {
				// Set up InfraNIC endpoint in CNS
				mockCNS.SetEndpointStateWithIPInfo(containerID, podName, podNamespace, map[string]*restserver.IPInfo{
					"eth0": acnnetwork.CreateMockIPInfo(cns.InfraNIC, "10.240.0.5/24", "", "", "veth-host", ""),
				})
				// Pre-populate ipam invoker with the IP so Delete validates it
				mockIpam.ipMap["10.240.0.5/24"] = true
			},
			validateAfter: func(t *testing.T, mockIpam *MockIpamInvoker) {
				// Verify IP was released from IPAM
				_, exists := mockIpam.ipMap["10.240.0.5/24"]
				require.False(t, exists, "InfraNIC IP should be released from IPAM")
			},
			wantErr:     false,
			description: "InfraNIC endpoint should be deleted and IP released via ipamInvoker.Delete",
		},
		{
			name: "Delete FrontendNIC endpoint - IP NOT released (delegated)",
			setupState: func(mockCNS *acnnetwork.MockCNSEndpointClient, _ *MockIpamInvoker) {
				// Set up FrontendNIC (delegated) endpoint in CNS
				mockCNS.SetEndpointStateWithIPInfo(containerID, podName, podNamespace, map[string]*restserver.IPInfo{
					"eth1": acnnetwork.CreateMockIPInfo(cns.NodeNetworkInterfaceFrontendNIC, "20.20.20.20/32", "", "", "", "aa:bb:cc:dd:ee:ff"),
				})
				// Do NOT add to ipam invoker - delegated IPs should not be released
			},
			validateAfter: func(t *testing.T, mockIpam *MockIpamInvoker) {
				// Verify ipMap is unchanged (delegated NICs don't release IPs via IPAM)
				require.Empty(t, mockIpam.ipMap, "FrontendNIC should not trigger IPAM release")
			},
			wantErr:     false,
			description: "FrontendNIC (delegated) endpoint should be deleted but IP NOT released",
		},
		{
			name: "Delete BackendNIC endpoint - IP NOT released",
			setupState: func(mockCNS *acnnetwork.MockCNSEndpointClient, _ *MockIpamInvoker) {
				// Set up BackendNIC endpoint in CNS
				mockCNS.SetEndpointStateWithIPInfo(containerID, podName, podNamespace, map[string]*restserver.IPInfo{
					"ib1": acnnetwork.CreateMockIPInfo(cns.BackendNIC, "", "", "", "", ""),
				})
				// BackendNIC has no IP to release
			},
			validateAfter: func(t *testing.T, mockIpam *MockIpamInvoker) {
				// Verify ipMap is unchanged (BackendNIC has no IPs)
				require.Empty(t, mockIpam.ipMap, "BackendNIC should not trigger IPAM release")
			},
			wantErr:     false,
			description: "BackendNIC endpoint should be deleted (no IP release needed)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh plugin and mocks for each test
			plugin, mockNM := GetStatelessTestResources(t)
			mockIpam := NewMockIpamInvoker(false, false, false, false, false)
			plugin.ipamInvoker = mockIpam

			// Set up endpoint state
			tt.setupState(mockNM.MockCNSClient, mockIpam)

			cmdArgs := &cniSkel.CmdArgs{
				StdinData:   nwCfgStateless.Serialize(),
				ContainerID: containerID,
				Netns:       containerID,
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", podName, podNamespace),
				IfName:      "eth0",
			}

			err := plugin.Delete(cmdArgs)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			// Validate side effects
			if tt.validateAfter != nil {
				tt.validateAfter(t, mockIpam)
			}
		})
	}
}

// TestStatelessCNI_Delete_MultiNIC tests DELETE with multiple NICs (SwiftV2 scenario)
func TestStatelessCNI_Delete_MultiNIC(t *testing.T) {
	nwCfgStateless := createStatelessTestConfig()
	containerID := "multi-nic-container"
	podName := "multi-nic-pod"
	podNamespace := testNamespace

	tests := []struct {
		name          string
		setupState    func(*acnnetwork.MockCNSEndpointClient, *MockIpamInvoker)
		validateAfter func(*testing.T, *MockIpamInvoker)
		wantErr       bool
		description   string
	}{
		{
			name: "Delete InfraNIC + FrontendNIC - only InfraNIC IP released",
			setupState: func(mockCNS *acnnetwork.MockCNSEndpointClient, mockIpam *MockIpamInvoker) {
				// Set up both InfraNIC and FrontendNIC (SwiftV2 scenario)
				mockCNS.SetEndpointStateWithIPInfo(containerID, podName, podNamespace, map[string]*restserver.IPInfo{
					"eth0": acnnetwork.CreateMockIPInfo(cns.InfraNIC, "10.240.0.5/24", "", "", "veth-infra", ""),
					"eth1": acnnetwork.CreateMockIPInfo(cns.NodeNetworkInterfaceFrontendNIC, "20.20.20.20/32", "", "", "", "aa:bb:cc:dd:ee:ff"),
				})
				// Only InfraNIC IP should be released
				mockIpam.ipMap["10.240.0.5/24"] = true
			},
			validateAfter: func(t *testing.T, mockIpam *MockIpamInvoker) {
				// Verify InfraNIC IP was released
				_, exists := mockIpam.ipMap["10.240.0.5/24"]
				require.False(t, exists, "InfraNIC IP should be released from IPAM")
			},
			wantErr:     false,
			description: "Both endpoints deleted, only InfraNIC IP released via ipamInvoker.Delete",
		},
		{
			name: "Delete InfraNIC + BackendNIC",
			setupState: func(mockCNS *acnnetwork.MockCNSEndpointClient, mockIpam *MockIpamInvoker) {
				// Set up both InfraNIC and BackendNIC
				mockCNS.SetEndpointStateWithIPInfo(containerID, podName, podNamespace, map[string]*restserver.IPInfo{
					"eth0": acnnetwork.CreateMockIPInfo(cns.InfraNIC, "10.240.0.5/24", "", "", "veth-infra", ""),
					"ib1":  acnnetwork.CreateMockIPInfo(cns.BackendNIC, "", "", "", "", ""),
				})
				// Only InfraNIC IP should be released
				mockIpam.ipMap["10.240.0.5/24"] = true
			},
			validateAfter: func(t *testing.T, mockIpam *MockIpamInvoker) {
				// Verify InfraNIC IP was released
				_, exists := mockIpam.ipMap["10.240.0.5/24"]
				require.False(t, exists, "InfraNIC IP should be released from IPAM")
			},
			wantErr:     false,
			description: "Both endpoints deleted, only InfraNIC IP released",
		},
		{
			name: "Delete two FrontendNICs - no IP released",
			setupState: func(mockCNS *acnnetwork.MockCNSEndpointClient, _ *MockIpamInvoker) {
				// Set up two FrontendNICs (standalone SwiftV2)
				mockCNS.SetEndpointStateWithIPInfo(containerID, podName, podNamespace, map[string]*restserver.IPInfo{
					"eth1": acnnetwork.CreateMockIPInfo(cns.NodeNetworkInterfaceFrontendNIC, "20.20.20.20/32", "", "", "", "aa:bb:cc:dd:ee:f1"),
					"eth2": acnnetwork.CreateMockIPInfo(cns.NodeNetworkInterfaceFrontendNIC, "20.20.20.21/32", "", "", "", "aa:bb:cc:dd:ee:f2"),
				})
				// Delegated IPs not released - ipMap stays empty
			},
			validateAfter: func(t *testing.T, mockIpam *MockIpamInvoker) {
				// Verify ipMap is unchanged (delegated NICs don't release IPs via IPAM)
				require.Empty(t, mockIpam.ipMap, "FrontendNICs should not trigger IPAM release")
			},
			wantErr:     false,
			description: "Both FrontendNIC endpoints deleted, no IPs released (delegated)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh plugin and mocks for each test
			plugin, mockNM := GetStatelessTestResources(t)
			mockIpam := NewMockIpamInvoker(false, false, false, false, false)
			plugin.ipamInvoker = mockIpam

			// Set up endpoint state
			tt.setupState(mockNM.MockCNSClient, mockIpam)

			cmdArgs := &cniSkel.CmdArgs{
				StdinData:   nwCfgStateless.Serialize(),
				ContainerID: containerID,
				Netns:       containerID,
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", podName, podNamespace),
				IfName:      "eth0",
			}

			err := plugin.Delete(cmdArgs)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			// Validate side effects
			if tt.validateAfter != nil {
				tt.validateAfter(t, mockIpam)
			}
		})
	}
}

// TestStatelessCNI_Delete_DualStack tests DELETE with IPv4+IPv6 addresses
// This verifies that the Delete path handles endpoints with multiple IPs
func TestStatelessCNI_Delete_DualStack(t *testing.T) {
	nwCfgStateless := createStatelessTestConfig()
	containerID := "dualstack-container"
	podName := "dualstack-pod"
	podNamespace := testNamespace

	// Create fresh plugin and mocks
	plugin, mockNM := GetStatelessTestResources(t)
	mockIpam := NewMockIpamInvoker(false, false, false, false, false)
	plugin.ipamInvoker = mockIpam

	// Use CreateMockIPInfo helper which handles IP/mask formats correctly
	ipInfo := acnnetwork.CreateMockIPInfo(cns.InfraNIC, "10.240.0.5/24", "", "", "veth-host", "")
	// Add IPv6 address
	_, ipv6Net, _ := net.ParseCIDR("fc00::5/128")
	ipv6Net.IP = net.ParseIP("fc00::5")
	ipInfo.IPv6 = []net.IPNet{*ipv6Net}

	mockNM.MockCNSClient.SetEndpointStateWithIPInfo(containerID, podName, podNamespace, map[string]*restserver.IPInfo{
		"eth0": ipInfo,
	})

	// Pre-populate ipam invoker with both IPs so Delete validates them
	mockIpam.ipMap["10.240.0.5/24"] = true
	mockIpam.ipMap["fc00::5/128"] = true

	cmdArgs := &cniSkel.CmdArgs{
		StdinData:   nwCfgStateless.Serialize(),
		ContainerID: containerID,
		Netns:       containerID,
		Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", podName, podNamespace),
		IfName:      "eth0",
	}

	// Verify Delete succeeds with dual-stack endpoint
	err := plugin.Delete(cmdArgs)
	require.NoError(t, err)

	// Verify both IPv4 and IPv6 addresses were released
	_, v4Exists := mockIpam.ipMap["10.240.0.5/24"]
	require.False(t, v4Exists, "IPv4 address should be released from IPAM")
	_, v6Exists := mockIpam.ipMap["fc00::5/128"]
	require.False(t, v6Exists, "IPv6 address should be released from IPAM")
}

// TestStatelessCNI_Delete_IpamDeleteFails tests DELETE when ipamInvoker.Delete fails
func TestStatelessCNI_Delete_IpamDeleteFails(t *testing.T) {
	nwCfgStateless := createStatelessTestConfig()
	containerID := "ipam-fail-container"
	podName := "ipam-fail-pod"
	podNamespace := testNamespace

	// Create fresh plugin and mocks
	plugin, mockNM := GetStatelessTestResources(t)
	mockIpam := NewMockIpamInvoker(false, true, false, false, false) // v4Fail=true to trigger delete failure
	plugin.ipamInvoker = mockIpam

	// Set up InfraNIC endpoint in CNS
	mockNM.MockCNSClient.SetEndpointStateWithIPInfo(containerID, podName, podNamespace, map[string]*restserver.IPInfo{
		"eth0": acnnetwork.CreateMockIPInfo(cns.InfraNIC, "10.240.0.5/24", "", "", "veth-host", ""),
	})
	// DO NOT add to ipMap to trigger delete failure

	cmdArgs := &cniSkel.CmdArgs{
		StdinData:   nwCfgStateless.Serialize(),
		ContainerID: containerID,
		Netns:       containerID,
		Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", podName, podNamespace),
		IfName:      "eth0",
	}

	err := plugin.Delete(cmdArgs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to release address")
}
