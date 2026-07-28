//go:build linux
// +build linux

package network

import (
	"net"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	vishnetlink "github.com/vishvananda/netlink"
)

// test interface name constants shared across master-resolution tests.
const (
	testMasterIface = "eth1"       // upper/master interface (netvsc) name
	testVFIface     = "enP12217s2" // SR-IOV VF bonded to testMasterIface (shares its MAC)
)

var errStubLinkNotFound = errors.New("stub: link not found by index")

// permissiveNetlinkClient resolves any interface to itself (MasterIndex == 0),
// except testVFIface which is treated as a VF enslaved to testMasterIface. It lets
// tests exercise AddEndpoints without touching the real netlink socket while still
// preserving each interface name (so mocks keyed on names like "badeth" still work).
type permissiveNetlinkClient struct{}

func (permissiveNetlinkClient) LinkByName(name string) (vishnetlink.Link, error) {
	if name == testVFIface {
		return &vishnetlink.Dummy{LinkAttrs: vishnetlink.LinkAttrs{Name: testVFIface, Index: 5, MasterIndex: 2}}, nil
	}
	return &vishnetlink.Dummy{LinkAttrs: vishnetlink.LinkAttrs{Name: name, Index: 2, MasterIndex: 0}}, nil
}

func (permissiveNetlinkClient) LinkByIndex(index int) (vishnetlink.Link, error) {
	if index == 2 {
		return &vishnetlink.Dummy{LinkAttrs: vishnetlink.LinkAttrs{Name: testMasterIface, Index: 2, MasterIndex: 0}}, nil
	}
	return nil, errStubLinkNotFound
}

// setStubMasterResolution installs the permissive netlink client into the package
// resolver and returns a function that restores the previous client. Usable from both
// standard tests and Ginkgo BeforeEach/AfterEach blocks.
func setStubMasterResolution() (restore func()) {
	original := masterNl
	masterNl = permissiveNetlinkClient{}
	return func() { masterNl = original }
}

// stubMasterResolution installs the permissive netlink client and restores the
// original on test cleanup.
func stubMasterResolution(t *testing.T) {
	t.Helper()
	t.Cleanup(setStubMasterResolution())
}

// vfReturningNetIO is a netio.NetIOInterface whose MAC lookup returns a bare VF
// name, emulating an accelerated-networking node where net.Interfaces() enumerated
// the VF before its netvsc master.
type vfReturningNetIO struct{ vfName string }

func (v *vfReturningNetIO) GetNetworkInterfaceByName(name string) (*net.Interface, error) {
	return &net.Interface{Name: name, Index: 5}, nil
}

func (v *vfReturningNetIO) GetNetworkInterfaceAddrs(*net.Interface) ([]net.Addr, error) {
	return []net.Addr{}, nil
}

func (v *vfReturningNetIO) GetNetworkInterfaceByMac(mac net.HardwareAddr) (*net.Interface, error) {
	return &net.Interface{Name: v.vfName, HardwareAddr: mac, Index: 5}, nil
}

// TestSecondaryAddEndpointsResolvesMaster proves that when the MAC lookup returns
// the bare SR-IOV VF, AddEndpoints resolves it to the netvsc master before the
// interface name is recorded/moved into the pod netns.
func TestSecondaryAddEndpointsResolvesMaster(t *testing.T) {
	stubMasterResolution(t) // VF (testVFIface) is enslaved to master (testMasterIface)

	mac, _ := net.ParseMAC("ab:cd:ef:12:34:56")
	client := &SecondaryEndpointClient{
		netioshim: &vfReturningNetIO{vfName: testVFIface},
		ep:        &endpoint{SecondaryInterfaces: make(map[string]*InterfaceInfo)},
	}
	epInfo := &EndpointInfo{MacAddress: mac}

	require.NoError(t, client.AddEndpoints(epInfo))
	require.Equal(t, testMasterIface, epInfo.IfName, "bare VF must be resolved to its netvsc master before use")
	_, ok := client.ep.SecondaryInterfaces[testMasterIface]
	require.True(t, ok, "master interface should be recorded, not the bare VF")
	_, vfRecorded := client.ep.SecondaryInterfaces[testVFIface]
	require.False(t, vfRecorded, "bare VF must not be recorded as the secondary interface")
}
