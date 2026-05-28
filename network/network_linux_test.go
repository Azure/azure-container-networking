package network

import (
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockIPTablesClientWithRunCmd extends mock to track RunCmd calls.
type mockIPTablesClientWithRunCmd struct {
	mockIPTablesClient
	runCmdCalls []string
}

func (c *mockIPTablesClientWithRunCmd) RunCmd(version, params string) error {
	c.runCmdCalls = append(c.runCmdCalls, version+" "+params)
	return nil
}

func TestHandleCommonOptions_AppliesRoutes(t *testing.T) {
	routeCalled := false
	nl := netlink.NewMockNetlink(false, "")
	nl.SetAddRouteValidationFn(func(_ *netlink.Route) error {
		routeCalled = true
		return nil
	})

	nm := &networkManager{
		netlink:        nl,
		netio:          netio.NewMockNetIO(false, 0),
		iptablesClient: &mockIPTablesClient{},
	}

	nwInfo := &EndpointInfo{
		Options: map[string]interface{}{
			RoutesKey: []RouteInfo{
				{
					Dst: net.IPNet{IP: net.ParseIP("10.1.0.0"), Mask: net.CIDRMask(16, 32)},
					Gw:  net.ParseIP("10.0.0.1"),
				},
			},
		},
	}

	err := nm.handleCommonOptions("eth0", nwInfo)
	require.NoError(t, err)
	assert.True(t, routeCalled, "expected route to be added when RoutesKey is present in options")
}

func TestHandleCommonOptions_AppliesIPTables(t *testing.T) {
	iptc := &mockIPTablesClientWithRunCmd{}

	nm := &networkManager{
		netlink:        netlink.NewMockNetlink(false, ""),
		netio:          netio.NewMockNetIO(false, 0),
		iptablesClient: iptc,
	}

	nwInfo := &EndpointInfo{
		Options: map[string]interface{}{
			IPTablesKey: []iptables.IPTableEntry{
				{Version: iptables.V4, Params: "-A FORWARD -j ACCEPT"},
			},
		},
	}

	err := nm.handleCommonOptions("eth0", nwInfo)
	require.NoError(t, err)
	assert.NotEmpty(t, iptc.runCmdCalls, "expected iptables RunCmd to be called when IPTablesKey is present")
}

func TestHandleCommonOptions_NoOptionsNoError(t *testing.T) {
	nm := &networkManager{
		netlink:        netlink.NewMockNetlink(false, ""),
		netio:          netio.NewMockNetIO(false, 0),
		iptablesClient: &mockIPTablesClient{},
	}

	nwInfo := &EndpointInfo{
		Options: map[string]interface{}{},
	}

	err := nm.handleCommonOptions("eth0", nwInfo)
	require.NoError(t, err)
}

func TestHandleCommonOptions_NilOptionsNoError(t *testing.T) {
	nm := &networkManager{
		netlink:        netlink.NewMockNetlink(false, ""),
		netio:          netio.NewMockNetIO(false, 0),
		iptablesClient: &mockIPTablesClient{},
	}

	nwInfo := &EndpointInfo{
		Options: nil,
	}

	err := nm.handleCommonOptions("eth0", nwInfo)
	require.NoError(t, err)
}
