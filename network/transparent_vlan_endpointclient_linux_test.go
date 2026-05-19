//go:build linux
// +build linux

package network

import (
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	vishnetlink "github.com/vishvananda/netlink"
)

var (
	errNetnsMock         = errors.New("mock netns error")
	errMockNetIOFail     = errors.New("netio fail")
	errMockNetIONoIfFail = &net.OpError{Op: "route", Net: "ip+net", Source: nil, Addr: nil, Err: errors.New("no such network interface")}
)

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

// This mock netioshim provides more flexbility in when it errors compared to the one in the netio package
type mockNetIO struct {
	existingInterfaces map[string]bool
	err                error
}

func (ns *mockNetIO) GetNetworkInterfaceByName(name string) (*net.Interface, error) {
	if ns.existingInterfaces[name] {
		hwAddr, _ := net.ParseMAC("ab:cd:ef:12:34:56")
		return &net.Interface{
			//nolint:gomnd // Dummy MTU
			MTU:          1000,
			Name:         name,
			HardwareAddr: hwAddr,
			//nolint:gomnd // Dummy interface index
			Index: 2,
		}, nil
	}
	return nil, errors.Wrap(ns.err, name)
}

func (ns *mockNetIO) GetNetworkInterfaceAddrs(_ *net.Interface) ([]net.Addr, error) {
	return []net.Addr{}, nil
}

func (ns *mockNetIO) GetNetworkInterfaceByMac(mac net.HardwareAddr) (*net.Interface, error) {
	return &net.Interface{
		//nolint:gomnd // Dummy MTU
		MTU:          1000,
		Name:         "eth1",
		HardwareAddr: mac,
		//nolint:gomnd // Dummy interface index
		Index: 2,
	}, nil
}

func TestTransparentVlanAddEndpoints(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)

	setLinkNetNSTests := []struct {
		name          string
		client        *TransparentVlanEndpointClient
		epInfo        *EndpointInfo
		moveInterface string
		moveNS        string
		wantErr       bool
		wantErrMsg    string
	}{
		// Set the link network namespace and confirm that it was moved inside
		{
			name: "Set link netns good path",
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
				nsClient:          NewMockNamespaceClient(),
			},
			moveInterface: "eth0.1",
			moveNS:        "az_ns_1",
			epInfo:        &EndpointInfo{},
			wantErr:       false,
		},
		{
			name: "Set link netns fail to set",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netlink:           netlink.NewMockNetlink(true, "netlink fail"),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim:         netio.NewMockNetIO(false, 0),
				nsClient:          NewMockNamespaceClient(),
			},
			moveInterface: "A1veth0",
			moveNS:        "az_ns_2",
			epInfo:        &EndpointInfo{},
			wantErr:       true,
			wantErrMsg:    "failed to set A1veth0 inside namespace 1 (az_ns_2)",
		},
		{
			name: "Set link netns fail to detect",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netlink:           netlink.NewMockNetlink(false, ""),
				plClient:          platform.NewMockExecClient(false),
				netUtilsClient:    networkutils.NewNetworkUtils(nl, plc),
				netioshim: &mockNetIO{
					existingInterfaces: map[string]bool{},
					err:                errMockNetIOFail,
				},
				nsClient: NewMockNamespaceClient(),
			},
			moveInterface: "eth0.1",
			moveNS:        "az_ns_1",
			epInfo:        &EndpointInfo{},
			wantErr:       true,
			wantErrMsg:    "failed to detect eth0.1 inside namespace 1 (az_ns_1)",
		},
	}
	for _, tt := range setLinkNetNSTests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.setLinkNetNSAndConfirm(tt.moveInterface, 1, tt.moveNS)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrMsg, "Expected:%v actual:%v", tt.wantErrMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}

	tests := []struct {
		name       string
		client     *TransparentVlanEndpointClient
		epInfo     *EndpointInfo
		wantErr    bool
		wantErrMsg string
	}{
		// Ensuring vnet namespace and vlan both exist or are both absent before populating the vm
		{
			name: "Ensure clean populate VM neither vnet ns nor vlan if exists",
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
				},
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
				nsClient:       NewMockNamespaceClient(),
			},
			epInfo:  &EndpointInfo{},
			wantErr: false,
		},
		{
			name: "Ensure clean populate VM vnet ns exists vlan does not exist",
			client: &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient: &mockNetns{
					get:         defaultGet,
					getFromName: defaultGetFromName,
					deleteNamed: func(name string) (err error) {
						return newNetnsErrorMock("netns failure")
					},
				},
				netlink:        netlink.NewMockNetlink(false, ""),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim: &mockNetIO{
					existingInterfaces: map[string]bool{},
					err:                errMockNetIONoIfFail,
				},
				nsClient: NewMockNamespaceClient(),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "failed to cleanup/delete ns after noticing vlan interface does not exist: netns failure: " + errNetnsMock.Error(),
		},
		{
			name: "Ensure clean populate VM cleanup straggling vlan if in vm ns",
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
				},
				netlink:        netlink.NewMockNetlink(true, "netlink fail"),
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
				nsClient:       NewMockNamespaceClient(),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "failed to clean up vlan interface",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.ensureCleanPopulateVM()
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
				netlink: &netlink.MockNetlink{
					DeleteLinkFn: func(_ string) error {
						// should still succeed
						return netlink.ErrorMockNetlink
					},
				},
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
				nsClient:       NewMockNamespaceClient(),
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
			wantErrMsg: "failed to move or detect vnetVethName in vnet ns, deleting: failed to set A1veth0 inside namespace 1 (az_ns_1): " + netlink.ErrorMockNetlink.Error() + " : netlink fail",
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
				netioshim: &mockNetIO{
					existingInterfaces: map[string]bool{
						"A1veth0": true,
					},
					err: errMockNetIOFail,
				},
				nsClient: NewMockNamespaceClient(),
			},
			epInfo:     &EndpointInfo{},
			wantErr:    true,
			wantErrMsg: "container veth does not exist: failed to get container veth: B1veth0: " + errMockNetIOFail.Error() + "",
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
			name: "Add endpoints no vnet ns NetNS Set fail",
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
			name: "Add endpoints fail check vlan interface exists",
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
			wantErrMsg: "vlan interface doesn't exist: " + netio.ErrMockNetIOFail.Error() + ":eth0.1",
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
	dualStackIPAddresses := []net.IPNet{
		{
			IP:   net.ParseIP("192.168.0.4"),
			Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
		},
		{
			IP:   net.ParseIP("fd11::1"),
			Mask: net.CIDRMask(subnetv6Mask, ipv6Bits),
		},
	}

	tests := []struct {
		name                 string
		ipAddresses          []net.IPNet
		wantDeleteRouteCount int
		wantDeleteLinkCalled bool
	}{
		{
			name:                 "Delete endpoint with IPv4 addresses",
			ipAddresses:          IPAddresses,
			wantDeleteRouteCount: 2,
			wantDeleteLinkCalled: true,
		},
		{
			name:                 "Delete endpoint dual-stack (IPv4 + IPv6)",
			ipAddresses:          dualStackIPAddresses,
			wantDeleteRouteCount: 2,
			wantDeleteLinkCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockNl := netlink.NewMockNetlink(false, "")
			deleteRouteCount := 0
			mockNl.SetDeleteRouteValidationFn(func(_ *netlink.Route) error {
				deleteRouteCount++
				return nil
			})
			deleteLinkCalled := false
			mockNl.DeleteLinkFn = func(_ string) error {
				deleteLinkCalled = true
				return nil
			}

			client := &TransparentVlanEndpointClient{
				primaryHostIfName: "eth0",
				vlanIfName:        "eth0.1",
				vnetVethName:      "A1veth0",
				containerVethName: "B1veth0",
				vnetNSName:        "az_ns_1",
				netnsClient: &mockNetns{
					deleteNamed: defaultDeleteNamed,
				},
				netlink:        mockNl,
				plClient:       platform.NewMockExecClient(false),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
				netioshim:      netio.NewMockNetIO(false, 0),
			}
			ep := &endpoint{
				IPAddresses: tt.ipAddresses,
			}
			client.DeleteEndpointsImpl(ep)

			require.Equal(t, tt.wantDeleteRouteCount, deleteRouteCount, "unexpected number of route deletions")
			require.Equal(t, tt.wantDeleteLinkCalled, deleteLinkCalled, "delete link call mismatch")
		})
	}

	t.Run("Delete endpoint runs even if delete routes fails", func(t *testing.T) {
		nl := netlink.NewMockNetlink(true, "netlink failure")
		// count number of times delete and link and set route are called
		// even if deleting the routes fail, we should still delete the veth pair in the vnet ns
		deleteLinkFlag := 0
		nl.DeleteLinkFn = func(_ string) error {
			deleteLinkFlag++
			return errors.New("err mock")
		}
		errOnDeleteRouteFlag := 0
		nl.SetDeleteRouteValidationFn(func(_ *netlink.Route) error {
			errOnDeleteRouteFlag++
			return errors.New("err mock")
		})

		client := TransparentVlanEndpointClient{
			primaryHostIfName: "eth0",
			vlanIfName:        "eth0.1",
			vnetVethName:      "A1veth0",
			containerVethName: "B1veth0",
			vnetNSName:        "az_ns_1",
			netnsClient: &mockNetns{
				deleteNamed: defaultDeleteNamed,
			},
			netlink:        nl,
			plClient:       platform.NewMockExecClient(false),
			netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
			netioshim:      netio.NewMockNetIO(false, 0),
		}
		ep := &endpoint{
			IPAddresses: IPAddresses,
		}
		client.DeleteEndpointsImpl(ep)

		require.Equal(t, 1, errOnDeleteRouteFlag, "error must occur during delete route path")
		require.Equal(t, 1, deleteLinkFlag, "delete link must still be called")
	})

	t.Run("Delete dual-stack endpoint runs even if delete routes fails", func(t *testing.T) {
		nl := netlink.NewMockNetlink(true, "netlink failure")
		deleteLinkFlag := 0
		nl.DeleteLinkFn = func(_ string) error {
			deleteLinkFlag++
			return errors.New("err mock")
		}
		errOnDeleteRouteFlag := 0
		nl.SetDeleteRouteValidationFn(func(_ *netlink.Route) error {
			errOnDeleteRouteFlag++
			return errors.New("err mock")
		})

		client := TransparentVlanEndpointClient{
			primaryHostIfName: "eth0",
			vlanIfName:        "eth0.1",
			vnetVethName:      "A1veth0",
			containerVethName: "B1veth0",
			vnetNSName:        "az_ns_1",
			netnsClient: &mockNetns{
				deleteNamed: defaultDeleteNamed,
			},
			netlink:        nl,
			plClient:       platform.NewMockExecClient(false),
			netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
			netioshim:      netio.NewMockNetIO(false, 0),
		}
		ep := &endpoint{
			IPAddresses: dualStackIPAddresses,
		}
		client.DeleteEndpointsImpl(ep)

		require.Equal(t, 1, errOnDeleteRouteFlag, "error must occur during delete route path for first address")
		require.Equal(t, 1, deleteLinkFlag, "delete link must still be called even with dual-stack failure")
	})

	t.Run("Delete dual-stack endpoint verifies routes for both families", func(t *testing.T) {
		nl := netlink.NewMockNetlink(false, "")
		deletedRoutes := make([]netlink.Route, 0)
		nl.SetDeleteRouteValidationFn(func(r *netlink.Route) error {
			deletedRoutes = append(deletedRoutes, *r)
			return nil
		})
		deleteLinkCalled := false
		nl.DeleteLinkFn = func(_ string) error {
			deleteLinkCalled = true
			return nil
		}

		client := TransparentVlanEndpointClient{
			primaryHostIfName: "eth0",
			vlanIfName:        "eth0.1",
			vnetVethName:      "A1veth0",
			containerVethName: "B1veth0",
			vnetNSName:        "az_ns_1",
			netnsClient: &mockNetns{
				deleteNamed: defaultDeleteNamed,
			},
			netlink:        nl,
			plClient:       platform.NewMockExecClient(false),
			netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
			netioshim:      netio.NewMockNetIO(false, 0),
		}
		ep := &endpoint{
			IPAddresses: dualStackIPAddresses,
		}
		client.DeleteEndpointsImpl(ep)

		require.Len(t, deletedRoutes, 2, "both IPv4 and IPv6 routes should be deleted")
		// Verify the routes correspond to the correct IPs
		hasV4 := false
		hasV6 := false
		for _, r := range deletedRoutes {
			if r.Dst != nil && r.Dst.IP.To4() != nil {
				hasV4 = true
			}
			if r.Dst != nil && r.Dst.IP.To4() == nil {
				hasV6 = true
			}
		}
		require.True(t, hasV4, "should have deleted an IPv4 route")
		require.True(t, hasV6, "should have deleted an IPv6 route")
		require.True(t, deleteLinkCalled, "delete link must be called")
	})
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
		{
			name: "Configure interface and routes good path with SkipDefaultRoutes set to true for container",
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
				SkipDefaultRoutes: true,
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
				},
				Subnets: []SubnetInfo{
					{
						Gateway: net.ParseIP("192.168.0.1"),
						Prefix: net.IPNet{
							IP:   net.ParseIP("192.168.0.0"),
							Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Configure interface and routes good path with SkipDefaultRoutes set to false for container",
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
				SkipDefaultRoutes: true,
				IPAddresses: []net.IPNet{
					{
						IP:   net.ParseIP("192.168.0.4"),
						Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
					},
				},
				Subnets: []SubnetInfo{
					{
						Gateway: net.ParseIP("192.168.0.1"),
						Prefix: net.IPNet{
							IP:   net.ParseIP("192.168.0.0"),
							Mask: net.CIDRMask(subnetv4Mask, ipv4Bits),
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
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

func createFunctionWithFailurePattern(errorPattern []error) func() error {
	s := 0
	return func() error {
		if s >= len(errorPattern) {
			return nil
		}
		result := errorPattern[s]
		s++
		return result
	}
}

func TestAddDefaultRoutes(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	plc := platform.NewMockExecClient(false)

	tests := []struct {
		name          string
		client        *TransparentVlanEndpointClient
		linkName      string
		table         int
		virtualGwCIDR string
		defaultPrefix string
		wantErr       bool
	}{
		{
			name: "IPv4 good path",
			client: &TransparentVlanEndpointClient{
				netlink:        netlink.NewMockNetlink(false, ""),
				netioshim:      netio.NewMockNetIO(false, 0),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
			},
			linkName:      "eth0",
			table:         0,
			virtualGwCIDR: virtualGwIPVlanString,
			defaultPrefix: defaultGwCidr,
			wantErr:       false,
		},
		{
			name: "IPv4 with tunneling table - good path",
			client: &TransparentVlanEndpointClient{
				netlink:        netlink.NewMockNetlink(false, ""),
				netioshim:      netio.NewMockNetIO(false, 0),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
			},
			linkName:      "eth0.1",
			table:         tunnelingTable,
			virtualGwCIDR: virtualGwIPVlanString,
			defaultPrefix: defaultGwCidr,
			wantErr:       false,
		},
		{
			name: "IPv4 route fails",
			client: &TransparentVlanEndpointClient{
				netlink:        netlink.NewMockNetlink(false, ""),
				netioshim:      netio.NewMockNetIO(true, 2),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
			},
			linkName:      "eth0",
			table:         0,
			virtualGwCIDR: virtualGwIPVlanString,
			defaultPrefix: defaultGwCidr,
			wantErr:       true,
		},
		{
			name: "IPv6 good path",
			client: &TransparentVlanEndpointClient{
				netlink:        netlink.NewMockNetlink(false, ""),
				netioshim:      netio.NewMockNetIO(false, 0),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
			},
			linkName:      "eth0",
			table:         0,
			virtualGwCIDR: virtualv6GwString,
			defaultPrefix: defaultIPv6Prefix,
			wantErr:       false,
		},
		{
			name: "IPv6 with tunneling table - good path",
			client: &TransparentVlanEndpointClient{
				netlink:        netlink.NewMockNetlink(false, ""),
				netioshim:      netio.NewMockNetIO(false, 0),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
			},
			linkName:      "eth0.1",
			table:         tunnelingTable,
			virtualGwCIDR: virtualv6GwString,
			defaultPrefix: defaultIPv6Prefix,
			wantErr:       false,
		},
		{
			name: "IPv6 route fails",
			client: &TransparentVlanEndpointClient{
				netlink:        netlink.NewMockNetlink(false, ""),
				netioshim:      netio.NewMockNetIO(true, 2),
				netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
			},
			linkName:      "eth0",
			table:         0,
			virtualGwCIDR: virtualv6GwString,
			defaultPrefix: defaultIPv6Prefix,
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.addDefaultRoutesHelper(tt.linkName, tt.table, tt.virtualGwCIDR, tt.defaultPrefix)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAddDefaultArp(t *testing.T) {
	tests := []struct {
		name        string
		client      *TransparentVlanEndpointClient
		ifName      string
		destMac     string
		gatewayCIDR string
		wantErr     bool
	}{
		{
			name: "IPv4 - good path",
			client: &TransparentVlanEndpointClient{
				netlink: netlink.NewMockNetlink(false, ""),
			},
			ifName:      "eth0",
			destMac:     azureMac,
			gatewayCIDR: virtualGwIPVlanString,
			wantErr:     false,
		},
		{
			name: "IPv6 - good path",
			client: &TransparentVlanEndpointClient{
				netlink: netlink.NewMockNetlink(false, ""),
			},
			ifName:      "eth0",
			destMac:     azureMac,
			gatewayCIDR: virtualv6GwString,
			wantErr:     false,
		},
		{
			name: "ARP fails with invalid MAC on first (IPv4) neighbor",
			client: &TransparentVlanEndpointClient{
				netlink: netlink.NewMockNetlink(false, ""),
			},
			ifName:      "eth0",
			destMac:     "invalid-mac",
			gatewayCIDR: virtualGwIPVlanString,
			wantErr:     true,
		},
		{
			name: "IPv6 neighbor insertion fails",
			client: func() *TransparentVlanEndpointClient {
				nl := netlink.NewMockNetlink(false, "")
				nl.SetOrRemoveLinkAddressFn = func(linkInfo netlink.LinkInfo, _, _ int) error {
					if linkInfo.IPAddr.To4() == nil {
						return errors.New("mock IPv6 neighbor failure")
					}
					return nil
				}
				return &TransparentVlanEndpointClient{netlink: nl}
			}(),
			ifName:      "eth0",
			destMac:     azureMac,
			gatewayCIDR: virtualv6GwString,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.addDefaultNeighbors(tt.ifName, tt.destMac, tt.gatewayCIDR)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// mockNetlinkRuleClient stubs vishvananda/netlink RuleList/RuleAdd so tests
// never touch real netlink sockets or require CAP_NET_ADMIN.
type mockNetlinkRuleClient struct {
	rules []vishnetlink.Rule
	added []*vishnetlink.Rule
}

func (m *mockNetlinkRuleClient) RuleList(_ int) ([]vishnetlink.Rule, error) {
	return m.rules, nil
}

func (m *mockNetlinkRuleClient) RuleAdd(rule *vishnetlink.Rule) error {
	m.added = append(m.added, rule)
	return nil
}

func TestAddVnetRulesIPTables(t *testing.T) {
	t.Run("IPv6 rules", func(t *testing.T) {
		mockIPT := &mockIPTablesClient{}
		mockNLRule := &mockNetlinkRuleClient{}
		client := &TransparentVlanEndpointClient{
			vlanIfName:     "eth0.1",
			iptablesClient: mockIPT,
			nlRuleClient:   mockNLRule,
		}

		err := client.addVnetMangleAndTunnelingRules(iptables.V6, vishnetlink.FAMILY_V6)
		require.NoError(t, err)

		var v6Calls int
		for _, call := range mockIPT.insertCalls {
			if call.version == iptables.V6 {
				v6Calls++
			}
		}
		require.Equal(t, 2, v6Calls, "expected 2 IPv6 ip6tables calls (mark + accept)")
		require.Len(t, mockNLRule.added, 1, "expected one rule added via RuleAdd")
		require.Equal(t, tunnelingMark, int(mockNLRule.added[0].Mark))
		require.Equal(t, tunnelingTable, mockNLRule.added[0].Table)
	})

	t.Run("IPv4 rules", func(t *testing.T) {
		mockIPT := &mockIPTablesClient{}
		mockNLRule := &mockNetlinkRuleClient{}
		client := &TransparentVlanEndpointClient{
			vlanIfName:     "eth0.1",
			iptablesClient: mockIPT,
			nlRuleClient:   mockNLRule,
		}

		err := client.addVnetMangleAndTunnelingRules(iptables.V4, vishnetlink.FAMILY_V4)
		require.NoError(t, err)

		var v4Calls int
		for _, call := range mockIPT.insertCalls {
			if call.version == iptables.V4 {
				v4Calls++
			}
		}
		require.Equal(t, 2, v4Calls, "expected 2 IPv4 iptables calls (mark + accept)")
		require.Len(t, mockNLRule.added, 1, "expected one rule added via RuleAdd")
		require.Equal(t, tunnelingMark, int(mockNLRule.added[0].Mark))
		require.Equal(t, tunnelingTable, mockNLRule.added[0].Table)
	})

	t.Run("skips RuleAdd when rule already exists", func(t *testing.T) {
		mockIPT := &mockIPTablesClient{}
		mockNLRule := &mockNetlinkRuleClient{
			rules: []vishnetlink.Rule{{Mark: tunnelingMark}},
		}
		client := &TransparentVlanEndpointClient{
			vlanIfName:     "eth0.1",
			iptablesClient: mockIPT,
			nlRuleClient:   mockNLRule,
		}

		err := client.addVnetMangleAndTunnelingRules(iptables.V4, vishnetlink.FAMILY_V4)
		require.NoError(t, err)
		require.Empty(t, mockNLRule.added, "RuleAdd should not be called when rule already exists")
	})
}

func TestRunWithRetries(t *testing.T) {
	errMock := errors.New("mock error")
	runs := 4

	tests := []struct {
		name    string
		wantErr bool
		f       func() error
	}{
		{
			name:    "Succeed on first try",
			f:       createFunctionWithFailurePattern([]error{}),
			wantErr: false,
		},
		{
			name:    "Succeed on first try do not check again",
			f:       createFunctionWithFailurePattern([]error{nil, errMock, errMock, errMock}),
			wantErr: false,
		},
		{
			name:    "Succeed on last try",
			f:       createFunctionWithFailurePattern([]error{errMock, errMock, errMock, nil, errMock}),
			wantErr: false,
		},
		{
			name:    "Fail after too many attempts",
			f:       createFunctionWithFailurePattern([]error{errMock, errMock, errMock, errMock, nil, nil}),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RunWithRetries(tt.f, runs, 100)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
