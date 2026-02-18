// Copyright 2017 Microsoft. All rights reserved.
// MIT License

//go:build windows
// +build windows

package network

import (
	"net"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/stretchr/testify/require"
)

// TestStatelessCNI_Delete_Windows_WithHNSEndpointID tests DELETE flow on Windows
// where HNS endpoint ID is retrieved from CNS endpoint state
func TestStatelessCNI_Delete_Windows_WithHNSEndpointID(t *testing.T) {
	// Set up mock HNS wrapper
	hnsFake := hnswrapper.NewHnsv2wrapperFake()
	Hnsv2 = hnswrapper.Hnsv2wrapperwithtimeout{
		Hnsv2:          hnsFake,
		HnsCallTimeout: 5 * time.Second,
	}

	// Create HNS network
	hnsNetworkID := "test-hns-network-stateless"
	hnsNetwork := &hcn.HostComputeNetwork{
		Id:   hnsNetworkID,
		Name: "azure-stateless",
	}
	_, err := Hnsv2.CreateNetwork(hnsNetwork)
	require.NoError(t, err)

	// Create HNS endpoint
	hnsEndpointID := "test-hns-endpoint-stateless"
	hnsEndpoint := &hcn.HostComputeEndpoint{
		Id:                 hnsEndpointID,
		Name:               hnsEndpointID,
		HostComputeNetwork: hnsNetworkID,
		MacAddress:         "00:11:22:33:44:55",
	}
	_, err = Hnsv2.CreateEndpoint(hnsEndpoint)
	require.NoError(t, err)

	// Verify endpoint was created
	endpoints := hnsFake.Cache.GetEndpoints()
	require.Len(t, endpoints, 1, "HNS endpoint should be created")

	// Set up mock CNS client with endpoint state containing HNS IDs
	mockCNSClient := NewMockCNSEndpointClient()
	containerID := "test-stateless-container-windows"
	ipInfo := CreateMockIPInfo(cns.InfraNIC, "10.0.0.10/24", hnsEndpointID, hnsNetworkID, "", "00:11:22:33:44:55")
	mockCNSClient.SetEndpointStateWithIPInfo(containerID, "test-pod", "test-ns", map[string]*restserver.IPInfo{
		"eth0": ipInfo,
	})

	// Create network manager in stateless mode
	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
	}

	// Create endpoint info from CNS state
	epInfo := &EndpointInfo{
		EndpointID:    containerID,
		ContainerID:   containerID,
		Data:          make(map[string]interface{}),
		IfName:        "eth0",
		NICType:       cns.InfraNIC,
		HNSEndpointID: hnsEndpointID,
		HNSNetworkID:  hnsNetworkID,
		MacAddress:    net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
	}

	// Execute DeleteEndpointStateless - should delete the HNS endpoint
	err = nm.DeleteEndpointStateless(hnsNetworkID, epInfo, "")
	require.NoError(t, err)

	// Verify HNS endpoint was deleted
	endpoints = hnsFake.Cache.GetEndpoints()
	require.Empty(t, endpoints, "HNS endpoint should be deleted")
}

// TestStatelessCNI_Delete_Windows_SwiftV2_MultipleNICs tests DELETE for SwiftV2 Windows with multiple NICs.
// In SwiftV2 Windows: each NIC has its own SEPARATE endpoint entry in CNS keyed by EndpointID.
// - InfraNIC: keyed by containerID
// - FrontendNIC: keyed by containerID-ifName (e.g., "containerID-eth1")
// Uses HNS IDs (no veth names on Windows).
func TestStatelessCNI_Delete_Windows_SwiftV2_MultipleNICs(t *testing.T) {
	hnsFake := hnswrapper.NewHnsv2wrapperFake()
	Hnsv2 = hnswrapper.Hnsv2wrapperwithtimeout{
		Hnsv2:          hnsFake,
		HnsCallTimeout: 5 * time.Second,
	}

	// Create network for InfraNIC
	infraNetworkID := "azure-infra-net"
	_, err := Hnsv2.CreateNetwork(&hcn.HostComputeNetwork{
		Id:   infraNetworkID,
		Name: "azure-infra",
	})
	require.NoError(t, err)

	// Create network for FrontendNIC (SwiftV2)
	frontendNetworkID := "azure-frontend-net"
	_, err = Hnsv2.CreateNetwork(&hcn.HostComputeNetwork{
		Id:   frontendNetworkID,
		Name: "azure-frontend",
		Type: hcn.Transparent,
	})
	require.NoError(t, err)

	// Create InfraNIC endpoint
	infraEndpointID := "infra-endpoint-123"
	_, err = Hnsv2.CreateEndpoint(&hcn.HostComputeEndpoint{
		Id:                 infraEndpointID,
		Name:               infraEndpointID,
		HostComputeNetwork: infraNetworkID,
	})
	require.NoError(t, err)

	// Create FrontendNIC endpoint (SwiftV2)
	frontendEndpointID := "frontend-endpoint-456"
	_, err = Hnsv2.CreateEndpoint(&hcn.HostComputeEndpoint{
		Id:                 frontendEndpointID,
		Name:               frontendEndpointID,
		HostComputeNetwork: frontendNetworkID,
		MacAddress:         "aa:bb:cc:dd:ee:ff",
	})
	require.NoError(t, err)

	// Verify both endpoints created
	require.Len(t, hnsFake.Cache.GetEndpoints(), 2)

	// SwiftV2 Windows: SEPARATE endpoint entries per NIC in CNS
	mockCNSClient := NewMockCNSEndpointClient()
	containerID := "multi-nic-container-windows"

	// InfraNIC entry keyed by containerID
	mockCNSClient.SetEndpointStateWithIPInfo(containerID, "multi-nic-pod", "default", map[string]*restserver.IPInfo{
		"eth0": CreateMockIPInfo(cns.InfraNIC, "10.0.0.20/24", infraEndpointID, infraNetworkID, "", "00:11:22:33:44:55"),
	})

	// FrontendNIC entry keyed by containerID-eth1 (separate entry)
	frontendEntryID := containerID + "-eth1"
	mockCNSClient.SetEndpointStateWithIPInfo(frontendEntryID, "multi-nic-pod", "default", map[string]*restserver.IPInfo{
		"eth1": CreateMockIPInfo(cns.NodeNetworkInterfaceFrontendNIC, "10.1.0.20/24", frontendEndpointID, frontendNetworkID, "", "aa:bb:cc:dd:ee:ff"),
	})

	// Verify CNS has TWO separate endpoint entries
	require.Len(t, mockCNSClient.EndpointState, 2, "SwiftV2 Windows should have separate endpoint entries per NIC")

	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
	}

	// Delete FrontendNIC endpoint first
	frontendEpInfo := &EndpointInfo{
		EndpointID:    frontendEntryID, // containerID-eth1
		ContainerID:   containerID,
		Data:          make(map[string]interface{}),
		IfName:        "eth1",
		NICType:       cns.NodeNetworkInterfaceFrontendNIC,
		HNSEndpointID: frontendEndpointID,
		HNSNetworkID:  frontendNetworkID,
		MacAddress:    net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
	}
	err = nm.DeleteEndpointStateless(frontendNetworkID, frontendEpInfo, "")
	require.NoError(t, err)

	// Verify frontend endpoint deleted, infra endpoint still exists
	endpoints := hnsFake.Cache.GetEndpoints()
	require.Len(t, endpoints, 1, "Should have 1 endpoint remaining (InfraNIC)")

	// Delete InfraNIC endpoint
	infraEpInfo := &EndpointInfo{
		EndpointID:    containerID, // InfraNIC uses containerID as EndpointID
		ContainerID:   containerID,
		Data:          make(map[string]interface{}),
		IfName:        "eth0",
		NICType:       cns.InfraNIC,
		HNSEndpointID: infraEndpointID,
		HNSNetworkID:  infraNetworkID,
		MacAddress:    net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
	}
	err = nm.DeleteEndpointStateless(infraNetworkID, infraEpInfo, "")
	require.NoError(t, err)

	// Verify all endpoints deleted
	endpoints = hnsFake.Cache.GetEndpoints()
	require.Empty(t, endpoints, "All HNS endpoints should be deleted")
}

// TestStatelessCNI_Delete_Windows_HNSNotFound tests DELETE when HNS endpoint doesn't exist
func TestStatelessCNI_Delete_Windows_HNSNotFound(t *testing.T) {
	hnsFake := hnswrapper.NewHnsv2wrapperFake()
	Hnsv2 = hnswrapper.Hnsv2wrapperwithtimeout{
		Hnsv2:          hnsFake,
		HnsCallTimeout: 5 * time.Second,
	}

	// Create network but NOT the endpoint
	hnsNetworkID := "test-network-no-endpoint"
	_, err := Hnsv2.CreateNetwork(&hcn.HostComputeNetwork{
		Id:   hnsNetworkID,
		Name: "azure-no-ep",
	})
	require.NoError(t, err)

	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
	}

	// CNS has endpoint state but HNS endpoint doesn't exist
	epInfo := &EndpointInfo{
		EndpointID:    "container-no-hns",
		ContainerID:   "container-no-hns",
		Data:          make(map[string]interface{}),
		IfName:        "eth0",
		NICType:       cns.InfraNIC,
		HNSEndpointID: "non-existent-hns-endpoint",
		HNSNetworkID:  hnsNetworkID,
	}

	// Should not error - DELETE should be idempotent
	err = nm.DeleteEndpointStateless(hnsNetworkID, epInfo, "")
	require.NoError(t, err)
}

// TestStatelessCNI_Delete_Windows_BackendNIC tests DELETE for backend NIC type
func TestStatelessCNI_Delete_Windows_BackendNIC(t *testing.T) {
	hnsFake := hnswrapper.NewHnsv2wrapperFake()
	Hnsv2 = hnswrapper.Hnsv2wrapperwithtimeout{
		Hnsv2:          hnsFake,
		HnsCallTimeout: 5 * time.Second,
	}

	// Create backend network
	backendNetworkID := "backend-network-123"
	_, err := Hnsv2.CreateNetwork(&hcn.HostComputeNetwork{
		Id:   backendNetworkID,
		Name: "azure-backend",
		Type: hcn.Transparent,
	})
	require.NoError(t, err)

	// Create backend endpoint
	backendEndpointID := "backend-endpoint-789"
	_, err = Hnsv2.CreateEndpoint(&hcn.HostComputeEndpoint{
		Id:                 backendEndpointID,
		Name:               backendEndpointID,
		HostComputeNetwork: backendNetworkID,
		MacAddress:         "bb:cc:dd:ee:ff:00",
	})
	require.NoError(t, err)

	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
	}

	// Set up endpoint info for backend NIC
	containerID := "backend-nic-container"
	epInfo := &EndpointInfo{
		EndpointID:    containerID + "-eth1",
		ContainerID:   containerID,
		Data:          make(map[string]interface{}),
		IfName:        "eth1",
		NICType:       cns.BackendNIC,
		HNSEndpointID: backendEndpointID,
		HNSNetworkID:  backendNetworkID,
		MacAddress:    net.HardwareAddr{0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00},
	}

	// Delete endpoint - BackendNIC deletion is intentionally skipped (see endpoint_windows.go)
	// "endpoint deletion is not required for IB"
	err = nm.DeleteEndpointStateless(backendNetworkID, epInfo, "")
	require.NoError(t, err)

	// Verify endpoint is NOT deleted - BackendNIC endpoints are skipped
	endpoints := hnsFake.Cache.GetEndpoints()
	require.Len(t, endpoints, 1, "BackendNIC endpoint should NOT be deleted")
}
