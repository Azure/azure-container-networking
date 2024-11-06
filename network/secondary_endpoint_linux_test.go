//go:build linux
// +build linux

package network

import (
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/stretchr/testify/require"
)

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
		client  *SecondaryEndpointClient
		ep      *endpoint
		wantErr bool
	}{
		{
			name: "Delete endpoint happy path",
			client: &SecondaryEndpointClient{
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
				nsClient:       NewMockNamespaceClient(),
			},
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
			client: &SecondaryEndpointClient{
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
				nsClient:       NewMockNamespaceClient(),
			},
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
			client: &SecondaryEndpointClient{
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
				nsClient:       NewMockNamespaceClient(),
			},
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
			client: &SecondaryEndpointClient{
				netlink:        netlink.NewMockNetlink(true, "netlink failure"),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
				nsClient:       NewMockNamespaceClient(),
			},
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
			client: &SecondaryEndpointClient{
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
				nsClient:       NewMockNamespaceClient(),
			},
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
			require.Len(t, tt.ep.SecondaryInterfaces, 1)
			if tt.wantErr {
				require.Error(t, tt.client.DeleteEndpoints(tt.ep))
				require.Len(t, tt.ep.SecondaryInterfaces, 1)
			} else {
				require.Nil(t, tt.client.DeleteEndpoints(tt.ep))
				require.Len(t, tt.ep.SecondaryInterfaces, 0)
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
