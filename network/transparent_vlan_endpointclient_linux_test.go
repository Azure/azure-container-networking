//go:build linux
// +build linux

package network

import (
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

var errNetnsMock = errors.New("mock netns error")

func newNetnsErrorMock(errStr string) error {
	return errors.Wrap(errNetnsMock, errStr)
}

type mockNetns struct {
	get         func() (fileDescriptor int, err error)
	getFromName func(name string) (fileDescriptor int, err error)
	set         func(fileDescriptor int) (err error)
	newNamed    func(name string) (fileDescriptor int, err error)
	deleteNamed func(name string) (err error)
}

func (netns *mockNetns) Get() (fileDescriptor int, err error) {
	return netns.get()
}

func (netns *mockNetns) GetFromName(name string) (fileDescriptor int, err error) {
	return netns.getFromName(name)
}

func (netns *mockNetns) Set(fileDescriptor int) (err error) {
	return netns.set(fileDescriptor)
}

func (netns *mockNetns) NewNamed(name string) (fileDescriptor int, err error) {
	return netns.newNamed(name)
}

func (netns *mockNetns) DeleteNamed(name string) (err error) {
	return netns.deleteNamed(name)
}

func (netns *mockNetns) IsNamespaceEqual(_, _ int) bool {
	return false
}

func (netns *mockNetns) NamespaceUniqueID(_ int) string {
	return "nsid"
}

func defaultGet() (int, error) {
	return 1, nil
}

func defaultGetFromName(name string) (int, error) {
	return 1, nil
}

func defaultSet(handle int) error {
	return nil
}

func defaultNewNamed(name string) (int, error) {
	return 1, nil
}

func defaultDeleteNamed(name string) error {
	return nil
}

func TestTransparentVlanAddEndpoints(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)

	tests := []struct {
		name       string
		client     *TransparentVlanEndpointClient
		epInfo     *EndpointInfo
		wantErr    bool
		wantErrMsg string
	}{
		// Populating VM with data and creating interfaces/links
		{
			name: "Add endpoints create vnet ns failure",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient: &mockNetns{
					get: defaultGet,
					getFromName: func(name string) (fileDescriptor int, err error) {
						return 0, newNetnsErrorMock("netns failure")
					},
					newNamed: func(name string) (fileDescriptor int, err error) {
						return 0, newNetnsErrorMock("netns failure")
					},
				},
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "failed to create vnet ns: netns failure: " + errNetnsMock.Error(),
		},
		{
			name: "Add endpoints with existing vnet ns",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient: &mockNetns{
					get:         defaultGet,
					getFromName: defaultGetFromName,
					newNamed:    defaultNewNamed,
					set:         defaultSet,
					deleteNamed: defaultDeleteNamed,
				},
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
			},
			epInfo:  &EndpointInfo{},
			wantErr: false,
		},
		{
			name: "Add endpoints netlink fail",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient: &mockNetns{
					get:         defaultGet,
					getFromName: defaultGetFromName,
					newNamed:    defaultNewNamed,
					set:         defaultSet,
					deleteNamed: defaultDeleteNamed,
				},
				netlink:        netlink.NewMockNetlink(true, "netlink fail"),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "failed to move vnetVethName into vnet ns, deleting: " + netlink.ErrorMockNetlink.Error() + " : netlink fail",
		},
		{
			name: "Add endpoints get interface fail for primary interface (eth0)",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient: &mockNetns{
					get: defaultGet,
					getFromName: func(name string) (fileDescriptor int, err error) {
						return 0, newNetnsErrorMock("netns failure")
					},
					newNamed:    defaultNewNamed,
					set:         defaultSet,
					deleteNamed: defaultDeleteNamed,
				},
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(true, 1),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "failed to get eth0 interface: " + netio.ErrMockNetIOFail.Error() + ":eth0",
		},
		{
			name: "Add endpoints get interface fail for getting container veth",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient: &mockNetns{
					get:         defaultGet,
					getFromName: defaultGetFromName,
					newNamed:    defaultNewNamed,
					set:         defaultSet,
					deleteNamed: defaultDeleteNamed,
				},
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(true, 1),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "container veth does not exist: " + netio.ErrMockNetIOFail.Error() + ":B1veth0",
		},
		{
			name: "Add endpoints NetNS Get fail",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient: &mockNetns{
					get: func() (fileDescriptor int, err error) {
						return 0, newNetnsErrorMock("netns failure")
					},
				},
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "failed to get vm ns handle: netns failure: " + errNetnsMock.Error(),
		},
		{
			name: "Add endpoints NetNS Set fail",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient: &mockNetns{
					get: defaultGet,
					getFromName: func(name string) (fileDescriptor int, err error) {
						return 0, newNetnsErrorMock("do not fail on this error")
					},
					newNamed: defaultNewNamed,
					set: func(fileDescriptor int) (err error) {
						return newNetnsErrorMock("netns failure")
					},
					deleteNamed: defaultDeleteNamed,
				},
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "failed to set current ns to vm: netns failure: " + errNetnsMock.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.PopulateVM(tt.epInfo)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrMsg, "Expected:%v actual:%v", tt.wantErrMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}

	tests = []struct {
		name       string
		client     *TransparentVlanEndpointClient
		epInfo     *EndpointInfo
		wantErr    bool
		wantErrMsg string
	}{
		// Populate the client with information from the vnet and set up vnet
		{
			name: "Add endpoints get vnet veth mac address",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo:  &EndpointInfo{},
			wantErr: false,
		},
		{
			name: "Add endpoints fail check vlan veth exists",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(true, 1),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "vlan veth doesn't exist: " + netio.ErrMockNetIOFail.Error() + ":eth0.1",
		},
		{
			name: "Add endpoints fail check vnet veth exists",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(true, 2),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "vnet veth doesn't exist: " + netio.ErrMockNetIOFail.Error() + ":A1veth0",
		},
		{
			name: "Add endpoints fail populate vnet disable rp filter",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(true),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "transparent vlan failed to disable rp filter in vnet: " + platform.ErrMockExec.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.PopulateVnet(tt.epInfo)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrMsg, "Expected:%v actual:%v", tt.wantErrMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestTransparentVlanDeleteEndpoints(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)
	IPAddresses := []net.IPNet{
		{
			IP:   net.ParseIP("192.168.0.4"),
			Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
		},
		{
			IP:   net.ParseIP("192.168.0.6"),
			Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
		},
	}

	tests := []struct {
		name       string
		client     *TransparentVlanEndpointClient
		ep         *endpoint
		wantErr    bool
		wantErrMsg string
		routesLeft func() (int, error)
	}{
		{
			name: "Delete endpoint delete vnet ns",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient: &mockNetns{
					deleteNamed: defaultDeleteNamed,
				},
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
			},
			ep: &endpoint{
				IPAddresses: IPAddresses,
			},
			routesLeft: func() (int, error) {
				return numDefaultRoutes, nil
			},
			wantErr: false,
		},
		{
			name: "Delete endpoint do not delete vnet ns it is still in use",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient: &mockNetns{
					deleteNamed: func(name string) (err error) {
						return newNetnsErrorMock("netns failure")
					},
				},
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
			},
			ep: &endpoint{
				IPAddresses: IPAddresses,
			},
			routesLeft: func() (int, error) {
				return numDefaultRoutes + 1, nil
			},
			wantErr: false,
		},
		//nolint gocritic
		/*		{
				name: "Delete endpoint fail to delete namespace",
				client: &TransparentVlanEndpointClient{
					primaryHostIfName: "eth0",
					vlanIfName:        "eth0.1",
					vnetVethName:      "A1veth0",
					containerVethName: "B1veth0",
					vnetNSName:        "az_ns_1",
					netnsClient: &mockNetns{
						deleteNamed: func(name string) (err error) {
							return newNetnsErrorMock("netns failure")
						},
					},
					netlink:        netlink.NewMockNetlink(false, ""),
					plClient:       platform.NewMockExecClient(false),
					netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
					netioshim:      netio.NewMockNetIO(false, 0),
				},
				ep: &endpoint{
					IPAddresses: IPAddresses,
				},
				routesLeft: func() (int, error) {
					return numDefaultRoutes, nil
				},
				wantErr:    true,
				wantErrMsg: "failed to delete namespace: netns failure: " + errNetnsMock.Error(),
			},*/
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.DeleteEndpointsImpl(tt.ep, tt.routesLeft)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrMsg, "Expected:%v actual:%v", tt.wantErrMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestTransparentVlanConfigureContainerInterfacesAndRoutes(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)

	vnetMac, _ := net.ParseMAC("ab:cd:ef:12:34:56")

	tests := []struct {
		name       string
		client     *TransparentVlanEndpointClient
		epInfo     *EndpointInfo
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "Configure interface and routes good path for container",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				vnetMac:           vnetMac,
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
			name: "Configure interface and routes multiple IPs",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				vnetMac:           vnetMac,
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
						IP:   net.ParseIP("192.168.0.6"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
					{
						IP:   net.ParseIP("192.168.0.8"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Configure interface and routes assign ip fail",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				vnetMac:           vnetMac,
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
			name: "Configure interface and routes container 2nd default route added fail",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				vnetMac:           vnetMac,
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(true, 3),
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
			wantErrMsg: "failed container ns add default routes: addRoutes failed: " + netio.ErrMockNetIOFail.Error() + ":B1veth0",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.ConfigureContainerInterfacesAndRoutesImpl(tt.epInfo)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrMsg, "Expected:%v actual:%v", tt.wantErrMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
	tests = []struct {
		name       string
		client     *TransparentVlanEndpointClient
		epInfo     *EndpointInfo
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "Configure interface and routes good path for vnet",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				vnetMac:           vnetMac,
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
			// fail route that tells which device container ip is on for vnet
			name: "Configure interface and routes fail final routes for vnet",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				vnetMac:           vnetMac,
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(true, 3),
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
			wantErrMsg: "failed adding routes to vnet specific to this container: addRoutes failed: " + netio.ErrMockNetIOFail.Error() + ":A1veth0",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.ConfigureVnetInterfacesAndRoutesImpl(tt.epInfo)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrMsg, "Expected:%v actual:%v", tt.wantErrMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
