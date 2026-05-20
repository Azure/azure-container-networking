package network

import (
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
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

func TestHandleCommonOptions_SkipsRoutesForDelegatedNIC(t *testing.T) {
	tests := []struct {
		name            string
		nicType         cns.NICType
		expectRouteCall bool
	}{
		{
			name:            "InfraNIC applies routes",
			nicType:         cns.InfraNIC,
			expectRouteCall: true,
		},
		{
			name:            "empty NICType applies routes (legacy)",
			nicType:         "",
			expectRouteCall: true,
		},
		{
			name:            "DelegatedVMNIC skips routes",
			nicType:         cns.DelegatedVMNIC,
			expectRouteCall: false,
		},
		{
			name:            "NodeNetworkInterfaceFrontendNIC skips routes",
			nicType:         cns.NodeNetworkInterfaceFrontendNIC,
			expectRouteCall: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
				NICType: tt.nicType,
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
			assert.Equal(t, tt.expectRouteCall, routeCalled,
				"route add called=%v, want=%v for NICType=%q", routeCalled, tt.expectRouteCall, tt.nicType)
		})
	}
}

func TestHandleCommonOptions_SkipsIPTablesForDelegatedNIC(t *testing.T) {
	tests := []struct {
		name              string
		nicType           cns.NICType
		expectIPTableCall bool
	}{
		{
			name:              "InfraNIC applies iptables",
			nicType:           cns.InfraNIC,
			expectIPTableCall: true,
		},
		{
			name:              "empty NICType applies iptables (legacy)",
			nicType:           "",
			expectIPTableCall: true,
		},
		{
			name:              "DelegatedVMNIC skips iptables",
			nicType:           cns.DelegatedVMNIC,
			expectIPTableCall: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iptc := &mockIPTablesClientWithRunCmd{}

			nm := &networkManager{
				netlink:        netlink.NewMockNetlink(false, ""),
				netio:          netio.NewMockNetIO(false, 0),
				iptablesClient: iptc,
			}

			nwInfo := &EndpointInfo{
				NICType: tt.nicType,
				Options: map[string]interface{}{
					IPTablesKey: []iptables.IPTableEntry{
						{Version: iptables.V4, Params: "-A FORWARD -j ACCEPT"},
					},
				},
			}

			err := nm.handleCommonOptions("eth0", nwInfo)
			require.NoError(t, err)

			if tt.expectIPTableCall {
				assert.NotEmpty(t, iptc.runCmdCalls, "expected iptables RunCmd to be called for NICType=%q", tt.nicType)
			} else {
				assert.Empty(t, iptc.runCmdCalls, "expected no iptables RunCmd call for NICType=%q", tt.nicType)
			}
		})
	}
}

func TestEndpointCreate_DoesNotSort(t *testing.T) {
	// Verify EndpointCreate processes epInfos in the order given (sorting is now
	// the caller's responsibility in cni/network).
	epInfos := []*EndpointInfo{
		{
			NICType:    cns.DelegatedVMNIC,
			EndpointID: "delegated-ep",
			NetworkID:  DefaultNetworkID,
		},
		{
			NICType:    cns.InfraNIC,
			EndpointID: "infra-ep",
			NetworkID:  DefaultNetworkID,
		},
	}

	nm := &networkManager{
		ExternalInterfaces: map[string]*externalInterface{},
	}

	_ = nm.EndpointCreate(nil, epInfos)

	// Order should be unchanged — EndpointCreate no longer sorts
	assert.Equal(t, cns.DelegatedVMNIC, epInfos[0].NICType)
	assert.Equal(t, cns.InfraNIC, epInfos[1].NICType)
}
