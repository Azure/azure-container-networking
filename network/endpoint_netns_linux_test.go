package network

import (
	"net"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	vishnetlink "github.com/vishvananda/netlink"
)

var errTestLinkNotFound = errors.New("link not found")

const (
	testNetnsPath  = "/var/run/netns/pod"
	testHostIfName = "azv768e8de"
	testPodIP      = "10.0.0.5"
)

// fakeNetnsLinkClient serves links by name for the in-namespace lookup and by
// index for the host-side peer lookup.
type fakeNetnsLinkClient struct {
	linksByName  map[string]vishnetlink.Link
	linksByIndex map[int]vishnetlink.Link
	addrs        []vishnetlink.Addr
	addrErr      error
}

func (f *fakeNetnsLinkClient) LinkByName(name string) (vishnetlink.Link, error) {
	link, ok := f.linksByName[name]
	if !ok {
		return nil, errTestLinkNotFound
	}
	return link, nil
}

func (f *fakeNetnsLinkClient) LinkByIndex(index int) (vishnetlink.Link, error) {
	link, ok := f.linksByIndex[index]
	if !ok {
		return nil, errTestLinkNotFound
	}
	return link, nil
}

func (f *fakeNetnsLinkClient) AddrList(_ vishnetlink.Link, _ int) ([]vishnetlink.Addr, error) {
	return f.addrs, f.addrErr
}

func testAddr(t *testing.T, cidr string) vishnetlink.Addr {
	t.Helper()
	ip, ipNet, err := net.ParseCIDR(cidr)
	require.NoError(t, err)
	ipNet.IP = ip
	return vishnetlink.Addr{IPNet: ipNet}
}

func TestGetEndpointInfoFromNetns(t *testing.T) {
	containerLink := &vishnetlink.Veth{
		LinkAttrs: vishnetlink.LinkAttrs{Name: InfraInterfaceName, Index: 12, ParentIndex: 45},
	}
	hostLink := &vishnetlink.Veth{
		LinkAttrs: vishnetlink.LinkAttrs{Name: testHostIfName, Index: 45},
	}

	tests := []struct {
		name           string
		netnsPath      string
		ifName         string
		client         *fakeNetnsLinkClient
		wantErr        bool
		wantHostIfName string
		wantIPs        []string
	}{
		{
			name:      "resolves pod ip and host veth from peer index",
			netnsPath: testNetnsPath,
			ifName:    InfraInterfaceName,
			client: &fakeNetnsLinkClient{
				linksByName:  map[string]vishnetlink.Link{InfraInterfaceName: containerLink},
				linksByIndex: map[int]vishnetlink.Link{45: hostLink},
				addrs:        []vishnetlink.Addr{testAddr(t, testPodIP+"/24")},
			},
			wantHostIfName: testHostIfName,
			wantIPs:        []string{testPodIP},
		},
		{
			name:      "returns ip even when host veth cannot be resolved",
			netnsPath: testNetnsPath,
			ifName:    InfraInterfaceName,
			client: &fakeNetnsLinkClient{
				linksByName:  map[string]vishnetlink.Link{InfraInterfaceName: containerLink},
				linksByIndex: map[int]vishnetlink.Link{},
				addrs:        []vishnetlink.Addr{testAddr(t, testPodIP+"/24")},
			},
			wantHostIfName: "",
			wantIPs:        []string{testPodIP},
		},
		{
			name:      "skips link local addresses",
			netnsPath: testNetnsPath,
			ifName:    InfraInterfaceName,
			client: &fakeNetnsLinkClient{
				linksByName:  map[string]vishnetlink.Link{InfraInterfaceName: containerLink},
				linksByIndex: map[int]vishnetlink.Link{45: hostLink},
				addrs: []vishnetlink.Addr{
					testAddr(t, "fe80::1/64"),
					testAddr(t, testPodIP+"/24"),
				},
			},
			wantHostIfName: testHostIfName,
			wantIPs:        []string{testPodIP},
		},
		{
			name:      "errors when the interface has no usable address",
			netnsPath: testNetnsPath,
			ifName:    InfraInterfaceName,
			client: &fakeNetnsLinkClient{
				linksByName:  map[string]vishnetlink.Link{InfraInterfaceName: containerLink},
				linksByIndex: map[int]vishnetlink.Link{45: hostLink},
				addrs:        []vishnetlink.Addr{},
			},
			wantErr: true,
		},
		{
			name:      "errors when the interface is gone",
			netnsPath: testNetnsPath,
			ifName:    InfraInterfaceName,
			client: &fakeNetnsLinkClient{
				linksByName:  map[string]vishnetlink.Link{},
				linksByIndex: map[int]vishnetlink.Link{},
			},
			wantErr: true,
		},
		{
			name:      "errors when no netns path is provided",
			netnsPath: "",
			ifName:    InfraInterfaceName,
			client:    &fakeNetnsLinkClient{},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			epInfo, err := getEndpointInfoFromNetns(NewMockNamespaceClient(), tt.client,
				"test-endpoint", tt.netnsPath, tt.ifName)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, "test-endpoint", epInfo.EndpointID)
			require.Equal(t, tt.wantHostIfName, epInfo.HostIfName)
			require.Equal(t, tt.netnsPath, epInfo.NetNsPath)

			gotIPs := make([]string, 0, len(epInfo.IPAddresses))
			for i := range epInfo.IPAddresses {
				gotIPs = append(gotIPs, epInfo.IPAddresses[i].IP.String())
			}
			require.Equal(t, tt.wantIPs, gotIPs)
		})
	}
}
