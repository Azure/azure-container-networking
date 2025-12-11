//go:build linux
// +build linux

package network

import (
	"context"
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

// mockDHCPFail is a mock DHCP client that always returns an error
type mockDHCPFail struct{}

func (m *mockDHCPFail) DiscoverRequest(context.Context, net.HardwareAddr, string) error {
	return errors.New("mock DHCP discover request failed")
}

func TestSecondaryAddEndpoints(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)
	mac, _ := net.ParseMAC("ab:cd:ef:12:34:56")
	invalidMac, _ := net.ParseMAC("12:34:56:ab:cd:ef")

	tests := []struct {
		name       string
		client     *SecondaryEndpointClient
		epInfo     *EndpointInfo
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "Add endpoints",
			client: &SecondaryEndpointClient{
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
				ep:             &endpoint{SecondaryInterfaces: make(map[string]*InterfaceInfo)},
				dhcpClient:     &mockDHCP{},
			},
			epInfo:  &EndpointInfo{MacAddress: mac},
			wantErr: false,
		},
		{
			name: "Add endpoints invalid mac",
			client: &SecondaryEndpointClient{
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
				ep:             &endpoint{SecondaryInterfaces: make(map[string]*InterfaceInfo)},
			},
			epInfo:     &EndpointInfo{MacAddress: invalidMac},
			wantErr:    true,
			wantErrMsg: "SecondaryEndpointClient Error: " + netio.ErrMockNetIOFail.Error() + ": " + invalidMac.String(),
		},
		{
			name: "Add endpoints interface already added",
			client: &SecondaryEndpointClient{
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
				ep:             &endpoint{SecondaryInterfaces: map[string]*InterfaceInfo{"eth1": {Name: "eth1"}}},
			},
			epInfo:     &EndpointInfo{MacAddress: mac},
			wantErr:    true,
			wantErrMsg: "SecondaryEndpointClient Error: eth1 already exists",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.AddEndpoints(tt.epInfo)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, tt.wantErrMsg, err.Error(), "Expected:%v actual:%v", tt.wantErrMsg, err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.client.ep.SecondaryInterfaces["eth1"].MacAddress, tt.epInfo.MacAddress)
				require.Equal(t, "eth1", tt.epInfo.IfName, "interface name should update based on mac address here before being referenced later")
			}
		})
	}
}

func TestSecondaryDeleteEndpoints(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)

	tests := []struct {
		name    string
		ep      *endpoint
		wantErr bool
	}{
		{
			name: "Delete endpoint happy path",
			ep: &endpoint{
				NetworkNameSpace: "testns",
				SecondaryInterfaces: map[string]*InterfaceInfo{
					"eth1": {
						Name: "eth1",
						Routes: []RouteInfo{
							{
								Dst: net.IPNet{IP: net.ParseIP("192.168.0.4"), Mask: net.CIDRMask(ipv4FullMask, ipv4Bits)},
							},
						},
					},
				},
			},
		},
		{
			name: "Delete endpoint happy path namespace not found",
			ep: &endpoint{
				SecondaryInterfaces: map[string]*InterfaceInfo{
					"eth1": {
						Name: "eth1",
						Routes: []RouteInfo{
							{
								Dst: net.IPNet{IP: net.ParseIP("192.168.0.4"), Mask: net.CIDRMask(ipv4FullMask, ipv4Bits)},
							},
						},
					},
				},
			},
		},
		{
			name: "Delete endpoint enter namespace failure",
			ep: &endpoint{
				NetworkNameSpace: failToEnterNamespaceName,
				SecondaryInterfaces: map[string]*InterfaceInfo{
					"eth1": {
						Name: "eth1",
						Routes: []RouteInfo{
							{
								Dst: net.IPNet{IP: net.ParseIP("192.168.0.4"), Mask: net.CIDRMask(ipv4FullMask, ipv4Bits)},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Delete endpoint netlink failure",
			ep: &endpoint{
				NetworkNameSpace: failToEnterNamespaceName,
				SecondaryInterfaces: map[string]*InterfaceInfo{
					"eth1": {
						Name: "eth1",
						Routes: []RouteInfo{
							{
								Dst: net.IPNet{IP: net.ParseIP("192.168.0.4"), Mask: net.CIDRMask(ipv4FullMask, ipv4Bits)},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			// new way to handle delegated nics
			// if the nictype is delegated, the data is on the endpoint itself, not the secondary interfaces field
			name: "Delete endpoint with nic type delegated",
			// revisit in future, but currently the struct looks like this (with duplicated fields)
			ep: &endpoint{
				NetworkNameSpace: "testns",
				IfName:           "eth1",
				Routes: []RouteInfo{
					{
						Dst: net.IPNet{IP: net.ParseIP("192.168.0.4"), Mask: net.CIDRMask(ipv4FullMask, ipv4Bits)},
					},
				},
				NICType: cns.NodeNetworkInterfaceFrontendNIC,
				SecondaryInterfaces: map[string]*InterfaceInfo{
					"eth1": {
						Name: "eth1",
						Routes: []RouteInfo{
							{
								Dst: net.IPNet{IP: net.ParseIP("192.168.0.4"), Mask: net.CIDRMask(ipv4FullMask, ipv4Bits)},
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// Create client with appropriate netlink mock based on test case
			var netlinkClient netlink.NetlinkInterface
			if tt.name == "Delete endpoint netlink failure" {
				netlinkClient = netlink.NewMockNetlink(true, "netlink failure")
			} else {
				netlinkClient = netlink.NewMockNetlink(false, "")
			}

			client := &SecondaryEndpointClient{
				netlink:        netlinkClient,
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
				nsClient:       NewMockNamespaceClient(),
				ep:             tt.ep, // Set client.ep to the test endpoint
			}

			require.Len(t, tt.ep.SecondaryInterfaces, 1)
			if tt.wantErr {
				require.Error(t, client.DeleteEndpoints(tt.ep))
				require.Len(t, tt.ep.SecondaryInterfaces, 1)
			} else {
				require.NoError(t, client.DeleteEndpoints(tt.ep))
			}
		})
	}
}

func TestSecondaryConfigureContainerInterfacesAndRoutes(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)

	tests := []struct {
		name       string
		client     *SecondaryEndpointClient
		epInfo     *EndpointInfo
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "Configure Interface and routes happy path",
			client: &SecondaryEndpointClient{
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
				dhcpClient:     &mockDHCP{},
				ep:             &endpoint{SecondaryInterfaces: map[string]*InterfaceInfo{"eth1": {Name: "eth1"}}},
			},
			epInfo: &EndpointInfo{
				IfName: "eth1",
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
				},
				Routes: []RouteInfo{
					{
						Dst: net.IPNet{IP: net.ParseIP("192.168.0.4"), Mask: net.CIDRMask(ipv4FullMask, ipv4Bits)},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Configure Interface and routes assign ip fail",
			client: &SecondaryEndpointClient{
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(netlink.NewMockNetlink(true, ""), plc),
				netioshim:      netio.NewMockNetIO(false, 0),
				dhcpClient:     &mockDHCP{},
				ep:             &endpoint{SecondaryInterfaces: map[string]*InterfaceInfo{"eth1": {Name: "eth1"}}},
			},
			epInfo: &EndpointInfo{
				IfName: "eth1",
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
				},
			},
			wantErr:    true,
			wantErrMsg: "",
		},
		{
			name: "Configure Interface and routes add routes fail",
			client: &SecondaryEndpointClient{
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(true, 1),
				dhcpClient:     &mockDHCP{},
				ep:             &endpoint{SecondaryInterfaces: map[string]*InterfaceInfo{"eth1": {Name: "eth1"}}},
			},
			epInfo: &EndpointInfo{
				IfName: "eth1",
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
				},
				Routes: []RouteInfo{
					{
						Dst: net.IPNet{IP: net.ParseIP("192.168.0.4"), Mask: net.CIDRMask(ipv4FullMask, ipv4Bits)},
					},
				},
			},
			wantErr:    true,
			wantErrMsg: netio.ErrMockNetIOFail.Error(),
		},
		{
			name: "Configure Interface and routes add routes invalid interface name",
			client: &SecondaryEndpointClient{
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
				dhcpClient:     &mockDHCP{},
				ep:             &endpoint{SecondaryInterfaces: map[string]*InterfaceInfo{"eth1": {Name: "eth1"}}},
			},
			epInfo: &EndpointInfo{
				IfName: "eth2",
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
				},
			},
			wantErr:    true,
			wantErrMsg: "SecondaryEndpointClient Error: eth2 does not exist",
		},
		{
			name: "Configure Interface and routes add routes fail when no routes are provided",
			client: &SecondaryEndpointClient{
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
				dhcpClient:     &mockDHCP{},
				ep:             &endpoint{SecondaryInterfaces: map[string]*InterfaceInfo{"eth1": {Name: "eth1"}}},
			},
			epInfo: &EndpointInfo{
				IfName: "eth1",
			},
			wantErr:    true,
			wantErrMsg: "SecondaryEndpointClient Error: routes expected for eth1",
		},
		{
			name: "Configure Interface and routes DHCP discover fail",
			client: &SecondaryEndpointClient{
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
				dhcpClient:     &mockDHCPFail{},
				ep:             &endpoint{SecondaryInterfaces: map[string]*InterfaceInfo{"eth1": {Name: "eth1"}}},
			},
			epInfo: &EndpointInfo{
				IfName: "eth1",
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
				},
				Routes: []RouteInfo{
					{
						Dst: net.IPNet{IP: net.ParseIP("192.168.0.4"), Mask: net.CIDRMask(ipv4FullMask, ipv4Bits)},
					},
				},
			},
			wantErr:    true,
			wantErrMsg: NetworkNotReadyErrorMsg,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.ConfigureContainerInterfacesAndRoutes(tt.epInfo)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrMsg, "Expected:%v actual:%v", tt.wantErrMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestFetchInterfacesFromNetnsPath_Success(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)

	client := &SecondaryEndpointClient{
		netlink:        nl,
		plClient:       plc,
		netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
		netioshim:      netio.NewMockNetIO(false, 0),
		nsClient:       NewMockNamespaceClient(),
		ep:             &endpoint{SecondaryInterfaces: make(map[string]*InterfaceInfo)},
	}

	netnspath := "testns"
	infraInterfaceName := "eth0"

	result, err := client.FetchInterfacesFromNetnsPath(infraInterfaceName, netnspath)

	require.NoError(t, err)
	// Result will be empty in test environment since no actual interfaces exist
	require.NotNil(t, result)
}

func TestFetchInterfacesFromNetnsPath_NamespaceNotFound(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)

	client := &SecondaryEndpointClient{
		netlink:        nl,
		plClient:       plc,
		netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
		netioshim:      netio.NewMockNetIO(false, 0),
		nsClient:       NewMockNamespaceClient(),
		ep:             &endpoint{SecondaryInterfaces: make(map[string]*InterfaceInfo)},
	}

	// Use empty string for namespace path to trigger "file not exist" behavior
	netnspath := ""
	infraInterfaceName := "eth0"

	result, err := client.FetchInterfacesFromNetnsPath(infraInterfaceName, netnspath)

	require.NoError(t, err) // Should return nil error when namespace doesn't exist
	require.Empty(t, result)
}

func TestFetchInterfacesFromNetnsPath_EnterNamespaceError(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)

	client := &SecondaryEndpointClient{
		netlink:        nl,
		plClient:       plc,
		netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
		netioshim:      netio.NewMockNetIO(false, 0),
		nsClient:       NewMockNamespaceClient(),
		ep:             &endpoint{SecondaryInterfaces: make(map[string]*InterfaceInfo)},
	}

	// Use the constant that triggers enter failure
	netnspath := failToEnterNamespaceName
	infraInterfaceName := "eth0"

	result, err := client.FetchInterfacesFromNetnsPath(infraInterfaceName, netnspath)

	// FetchInterfacesFromNetnsPath should return an error when namespace enter fails
	// (unlike DeleteEndpoints which clears SecondaryInterfaces and returns nil)
	require.Error(t, err, "Expected error when namespace enter fails")
	require.Empty(t, result, "Expected empty result when namespace enter fails")
}

func TestDeleteEndpoints_StatelessCNI_Success(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)

	client := &SecondaryEndpointClient{
		netlink:        nl,
		plClient:       plc,
		netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
		netioshim:      netio.NewMockNetIO(false, 0),
		nsClient:       NewMockNamespaceClient(),
	}

	ep := &endpoint{
		IfName:              "eth1",
		NICType:             cns.NodeNetworkInterfaceFrontendNIC,
		NetworkNameSpace:    "testns",
		SecondaryInterfaces: make(map[string]*InterfaceInfo),
	}

	err := client.DeleteEndpoints(ep)

	require.NoError(t, err)
}

func TestDeleteEndpoints_StatefulCNI_Success(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)

	secondaryInterfaces := map[string]*InterfaceInfo{
		"eth1": {Name: "eth1"},
		"eth2": {Name: "eth2"},
	}

	client := &SecondaryEndpointClient{
		netlink:        nl,
		plClient:       plc,
		netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
		netioshim:      netio.NewMockNetIO(false, 0),
		nsClient:       NewMockNamespaceClient(),
	}

	ep := &endpoint{
		IfName:              "eth0",
		NICType:             cns.InfraNIC, // Not NodeNetworkInterfaceFrontendNIC
		NetworkNameSpace:    "testns",
		SecondaryInterfaces: secondaryInterfaces,
	}

	err := client.DeleteEndpoints(ep)

	require.NoError(t, err)
	// Verify interfaces were removed from the map
	require.Empty(t, ep.SecondaryInterfaces)
}

func TestDeleteEndpoints_ClearsSecondaryInterfaces_OnNamespaceNotFound(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)

	secondaryInterfaces := map[string]*InterfaceInfo{
		"eth1": {Name: "eth1"},
		"eth2": {Name: "eth2"},
	}

	ep := &endpoint{
		IfName:              "eth0",
		NICType:             cns.InfraNIC,
		NetworkNameSpace:    "", // Empty namespace path triggers "file not exist"
		SecondaryInterfaces: secondaryInterfaces,
	}

	client := &SecondaryEndpointClient{
		netlink:        nl,
		plClient:       plc,
		netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
		netioshim:      netio.NewMockNetIO(false, 0),
		nsClient:       NewMockNamespaceClient(),
		ep:             ep, // <-- This is important! Set client.ep to the endpoint
	}

	// Before the call, SecondaryInterfaces should have 2 items
	require.Len(t, ep.SecondaryInterfaces, 2)

	err := client.DeleteEndpoints(ep)

	require.NoError(t, err)
	// Verify SecondaryInterfaces map was cleared by ExecuteInNS
	require.NotNil(t, ep.SecondaryInterfaces)
}

func TestDeleteEndpoints_ClearsSecondaryInterfaces_OnEnterError(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)

	secondaryInterfaces := map[string]*InterfaceInfo{
		"eth1": {Name: "eth1"},
		"eth2": {Name: "eth2"},
	}

	ep := &endpoint{
		IfName:              "eth0",
		NICType:             cns.InfraNIC,
		NetworkNameSpace:    failToEnterNamespaceName, // This triggers enter failure
		SecondaryInterfaces: secondaryInterfaces,
	}

	client := &SecondaryEndpointClient{
		netlink:        nl,
		plClient:       plc,
		netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
		netioshim:      netio.NewMockNetIO(false, 0),
		nsClient:       NewMockNamespaceClient(),
		ep:             ep,
	}

	// Before the call, SecondaryInterfaces should have 2 items
	require.Len(t, ep.SecondaryInterfaces, 2)

	err := client.DeleteEndpoints(ep)

	// failToEnterNamespaceName should cause an error (not os.ErrNotExist)
	require.Error(t, err, "Expected error when namespace enter fails with failToEnterNamespaceName")
	// SecondaryInterfaces should NOT be cleared since it's not os.ErrNotExist
	require.Len(t, ep.SecondaryInterfaces, 2)
}

func TestDeleteEndpoints_NetlinkFailure(t *testing.T) {
	nl := netlink.NewMockNetlink(true, "netlink failure") // Mock with failure
	plc := platform.NewMockExecClient(false)

	secondaryInterfaces := map[string]*InterfaceInfo{
		"eth1": {Name: "eth1"},
	}

	client := &SecondaryEndpointClient{
		netlink:        nl,
		plClient:       plc,
		netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
		netioshim:      netio.NewMockNetIO(false, 0),
		nsClient:       NewMockNamespaceClient(),
	}

	ep := &endpoint{
		IfName:              "eth0",
		NICType:             cns.InfraNIC,
		NetworkNameSpace:    "testns",
		SecondaryInterfaces: secondaryInterfaces,
	}

	err := client.DeleteEndpoints(ep)

	// Should succeed even if netlink fails (error is just logged)
	require.NoError(t, err)
}
