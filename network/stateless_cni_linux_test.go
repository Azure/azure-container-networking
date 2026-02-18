//go:build linux
// +build linux

package network

import (
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/netio"
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

	// Note: TransparentEndpointClient.DeleteEndpoints() returns nil without deleting
	// This is expected behavior - the CRI removes the network namespace which cleans up the veth
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

// TestStatelessCNI_Delete_Linux_NetlinkError tests error handling when netlink fails
func TestStatelessCNI_Delete_Linux_NetlinkError(t *testing.T) {
	// Create mock netlink that returns errors
	mockNetlink := netlink.NewMockNetlink(true, "simulated netlink failure")

	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
		netlink:            mockNetlink,
		plClient:           platform.NewMockExecClient(false),
		netio:              &netio.MockNetIO{},
	}

	containerID := "error-test-container"
	epInfo := &EndpointInfo{
		EndpointID:  containerID,
		ContainerID: containerID,
		Data:        make(map[string]interface{}),
		IfName:      "eth0",
		NICType:     cns.InfraNIC,
		HostIfName:  "veth-error",
		MacAddress:  net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
	}

	// Execute - transparent mode DeleteEndpoints returns nil regardless of netlink state
	// so this should still succeed
	err := nm.DeleteEndpointStateless("azure-error-network", epInfo, opModeTransparent)
	require.NoError(t, err)
}
