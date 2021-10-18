//+build linux

package network

import (
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/stretchr/testify/require"
)

const (
	subnetv4Mask = 24
	subnetv6Mask = 64
)

func TestTransAddEndpoints(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)

	tests := []struct {
		name       string
		client     *TransparentEndpointClient
		epInfo     *EndpointInfo
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "Add endpoints",
			client: &TransparentEndpointClient{
				hostPrimaryIfName: "eth0",
				hostVethName:      "azvhost",
				containerVethName: "azvcontainer",
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo:  &EndpointInfo{},
			wantErr: false,
		},
		{
			name: "Add endpoints netlink fail",
			client: &TransparentEndpointClient{
				hostPrimaryIfName: "eth0",
				hostVethName:      "azvhost",
				containerVethName: "azvcontainer",
				netlink:           netlink.NewMockNetlink(true, "netlink fail"),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "TransparentEndpointClient Error : " + netlink.ErrorMockNetlink.Error() + " : netlink fail",
		},
		{
			name: "Add endpoints get interface fail for old veth",
			client: &TransparentEndpointClient{
				hostPrimaryIfName: "eth0",
				hostVethName:      "azvhost",
				containerVethName: "azvcontainer",
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(true, 1),
			},
			epInfo:  &EndpointInfo{},
			wantErr: false,
		},
		{
			name: "Add endpoints get interface fail for primary interface",
			client: &TransparentEndpointClient{
				hostPrimaryIfName: "eth0",
				hostVethName:      "azvhost",
				containerVethName: "azvcontainer",
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(true, 2),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "TransparentEndpointClient Error : " + netio.ErrMockNetIOFail.Error() + ":eth0",
		},
		{
			name: "Add endpoints get interface fail for host veth",
			client: &TransparentEndpointClient{
				hostPrimaryIfName: "eth0",
				hostVethName:      "azvhost",
				containerVethName: "azvcontainer",
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(true, 3),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "TransparentEndpointClient Error : " + netio.ErrMockNetIOFail.Error() + ":azvcontainer",
		},
		{
			name: "get interface fail for container veth",
			client: &TransparentEndpointClient{
				hostPrimaryIfName: "eth0",
				hostVethName:      "azvhost",
				containerVethName: "azvcontainer",
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(true, 4),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "TransparentEndpointClient Error : " + netio.ErrMockNetIOFail.Error() + ":azvhost",
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
			}
		})
	}
}

func TestTransAddEndpointsRules(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)

	tests := []struct {
		name       string
		client     *TransparentEndpointClient
		epInfo     *EndpointInfo
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "Add endpoint rules happy path",
			client: &TransparentEndpointClient{
				hostPrimaryIfName: "eth0",
				hostVethName:      "azvhost",
				containerVethName: "azvcontainer",
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo: &EndpointInfo{
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Add endpoint rules Dualstack happy path",
			client: &TransparentEndpointClient{
				hostPrimaryIfName: "eth0",
				hostVethName:      "azvhost",
				containerVethName: "azvcontainer",
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo: &EndpointInfo{
				IPV6Mode: IPV6Nat,
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4FullMask),
					},
					{
						IP:   net.ParseIP("fc00::4"),
						Mask: net.CIDRMask(subnetv6Mask, ipv6FullMask),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Add endpoint rules fail",
			client: &TransparentEndpointClient{
				hostPrimaryIfName: "eth0",
				hostVethName:      "azvhost",
				containerVethName: "azvcontainer",
				netlink:           netlink.NewMockNetlink(true, "addroute fail"),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo: &EndpointInfo{
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
				},
			},
			wantErr:    true,
			wantErrMsg: "addroute fail",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.AddEndpointRules(tt.epInfo)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrMsg, "Expected:%v actual:%v", tt.wantErrMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestTransDeleteEndpointsRules(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)

	tests := []struct {
		name   string
		client *TransparentEndpointClient
		ep     *endpoint
	}{
		{
			name: "Delete endpoint rules happy path",
			client: &TransparentEndpointClient{
				hostPrimaryIfName: "eth0",
				hostVethName:      "azvhost",
				containerVethName: "azvcontainer",
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			ep: &endpoint{
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
					{
						IP:   net.ParseIP("fc00::4"),
						Mask: net.CIDRMask(subnetv6Mask, ipv6FullMask),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.client.DeleteEndpointRules(tt.ep)
		})
	}
}

func TestTransConfigureContainerInterfacesAndRoutes(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)

	tests := []struct {
		name       string
		client     *TransparentEndpointClient
		epInfo     *EndpointInfo
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "Configure Interface and routes happy path",
			client: &TransparentEndpointClient{
				hostPrimaryIfName: "eth0",
				hostVethName:      "azvhost",
				containerVethName: "azvcontainer",
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo: &EndpointInfo{
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Configure Interface and routes dualstack happy path",
			client: &TransparentEndpointClient{
				hostPrimaryIfName: "eth0",
				hostVethName:      "azvhost",
				containerVethName: "azvcontainer",
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo: &EndpointInfo{
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
					{
						IP:   net.ParseIP("fc00::4"),
						Mask: net.CIDRMask(subnetv6Mask, ipv6FullMask),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Configure Interface and routes assign ip fail",
			client: &TransparentEndpointClient{
				hostPrimaryIfName: "eth0",
				hostVethName:      "azvhost",
				containerVethName: "azvcontainer",
				netlink:           netlink.NewMockNetlink(true, "netlink fail"),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo: &EndpointInfo{
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
				},
			},
			wantErr:    true,
			wantErrMsg: "netlink fail",
		},
		{
			name: "Configure Interface and routes add routes fail",
			client: &TransparentEndpointClient{
				hostPrimaryIfName: "eth0",
				hostVethName:      "azvhost",
				containerVethName: "azvcontainer",
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(true, 2),
			},
			epInfo: &EndpointInfo{
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
				},
			},
			wantErr:    true,
			wantErrMsg: netio.ErrMockNetIOFail.Error(),
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
