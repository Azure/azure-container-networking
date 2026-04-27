package network

import (
	"context"
	"net"
	"strings"
	"syscall"
	"testing"

	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	vishnetlink "github.com/vishvananda/netlink"
)

// transparentTunnelMockIPTablesClient tracks all iptables calls for test verification.
type transparentTunnelMockIPTablesClient struct {
	insertCalls []iptablesCall
	appendCalls []iptablesCall
	deleteCalls []iptablesCall
}

func (c *transparentTunnelMockIPTablesClient) InsertIptableRule(version, tableName, chainName, match, target string) error {
	c.insertCalls = append(c.insertCalls, iptablesCall{version, tableName, chainName, match, target})
	return nil
}

func (c *transparentTunnelMockIPTablesClient) AppendIptableRule(version, tableName, chainName, match, target string) error {
	c.appendCalls = append(c.appendCalls, iptablesCall{version, tableName, chainName, match, target})
	return nil
}

func (c *transparentTunnelMockIPTablesClient) DeleteIptableRule(version, tableName, chainName, match, target string) error {
	c.deleteCalls = append(c.deleteCalls, iptablesCall{version, tableName, chainName, match, target})
	return nil
}

func (c *transparentTunnelMockIPTablesClient) CreateChain(_, _, _ string) error { return nil }
func (c *transparentTunnelMockIPTablesClient) RunCmd(_, _ string) error         { return nil }

// transparentTunnelMockExecClient tracks executed commands and returns canned responses.
type transparentTunnelMockExecClient struct {
	platform.ExecClient
	executedCmds []string
	// cmdResponses maps a substring to the response returned when a command contains it.
	cmdResponses map[string]string
}

func (c *transparentTunnelMockExecClient) ExecuteCommand(_ context.Context, cmd string, args ...string) (string, error) {
	full := cmd + " " + strings.Join(args, " ")
	c.executedCmds = append(c.executedCmds, full)
	for substr, resp := range c.cmdResponses {
		if strings.Contains(full, substr) {
			return resp, nil
		}
	}
	return "", nil
}

// transparentTunnelMockNlClient tracks netlink rule/route calls for test verification.
type transparentTunnelMockNlClient struct {
	ruleAddCalls      []*vishnetlink.Rule
	ruleDelCalls      []*vishnetlink.Rule
	routeReplaceCalls []*vishnetlink.Route
	routeDelCalls     []*vishnetlink.Route
	ruleAddErr        error // injected error for RuleAdd
}

func (c *transparentTunnelMockNlClient) RuleAdd(rule *vishnetlink.Rule) error {
	c.ruleAddCalls = append(c.ruleAddCalls, rule)
	return c.ruleAddErr
}

func (c *transparentTunnelMockNlClient) RuleDel(rule *vishnetlink.Rule) error {
	c.ruleDelCalls = append(c.ruleDelCalls, rule)
	return nil
}

func (c *transparentTunnelMockNlClient) RouteReplace(route *vishnetlink.Route) error {
	c.routeReplaceCalls = append(c.routeReplaceCalls, route)
	return nil
}

func (c *transparentTunnelMockNlClient) RouteDel(route *vishnetlink.Route) error {
	c.routeDelCalls = append(c.routeDelCalls, route)
	return nil
}

func TestTransparentTunnelAddEndpointRules(t *testing.T) {
	tests := []struct {
		name            string
		serviceCIDRs    string
		gateway         net.IP
		expectedInserts int // Number of iptables INSERT calls (service CIDR RETURN rules)
		expectedAppends int // Number of iptables APPEND calls (MARK rule)
		expectRuleAdd   bool
		expectRouteAdd  bool
		ruleAddErr      error // Injected RuleAdd error (nil = success, EEXIST = tolerated)
		expectError     bool
		errorContains   string
	}{
		{
			name:            "single service CIDR",
			serviceCIDRs:    "10.0.0.0/16",
			gateway:         net.ParseIP("10.224.0.1"),
			expectedInserts: 1,
			expectedAppends: 1,
			expectRuleAdd:   true,
			expectRouteAdd:  true,
		},
		{
			name:            "multiple service CIDRs",
			serviceCIDRs:    "10.0.0.0/16,fd00::/108",
			gateway:         net.ParseIP("10.224.0.1"),
			expectedInserts: 2,
			expectedAppends: 1,
			expectRuleAdd:   true,
			expectRouteAdd:  true,
		},
		{
			name:            "no service CIDRs",
			serviceCIDRs:    "",
			gateway:         net.ParseIP("10.224.0.1"),
			expectedInserts: 0,
			expectedAppends: 1,
			expectRuleAdd:   true,
			expectRouteAdd:  true,
		},
		{
			name:            "rule already exists is tolerated",
			serviceCIDRs:    "10.0.0.0/16",
			gateway:         net.ParseIP("10.224.0.1"),
			expectedInserts: 1,
			expectedAppends: 1,
			expectRuleAdd:   true,
			expectRouteAdd:  true,
			ruleAddErr:      syscall.EEXIST,
		},
		{
			name:          "nil gateway returns error before creating any rules",
			serviceCIDRs:  "10.0.0.0/16",
			gateway:       nil,
			expectError:   true,
			errorContains: "gateway is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iptMock := &transparentTunnelMockIPTablesClient{}
			nlMock := &transparentTunnelMockNlClient{ruleAddErr: tt.ruleAddErr}

			var serviceCIDRs []string
			if tt.serviceCIDRs != "" {
				serviceCIDRs = strings.Split(tt.serviceCIDRs, ",")
			}

			client := &TransparentTunnelEndpointClient{
				TransparentEndpointClient: &TransparentEndpointClient{
					hostVethName:      "azv1234",
					hostPrimaryIfName: "eth0",
					netioshim:         netio.NewMockNetIO(false, 0),
				},
				iptablesClient: iptMock,
				nlPolicyRoute:  nlMock,
				serviceCIDRs:   serviceCIDRs,
				gateway:        tt.gateway,
			}

			err := client.addTransparentTunnelRules()

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
				assert.Empty(t, iptMock.insertCalls, "no iptables rules should be created on error")
				assert.Empty(t, iptMock.appendCalls, "no iptables rules should be created on error")
				assert.Empty(t, nlMock.ruleAddCalls, "no netlink calls should run on error")
				return
			}

			require.NoError(t, err)

			// Verify service CIDR RETURN rules were inserted.
			assert.Len(t, iptMock.insertCalls, tt.expectedInserts)
			for _, call := range iptMock.insertCalls {
				assert.Equal(t, iptables.V4, call.version)
				assert.Equal(t, iptables.Mangle, call.tableName)
				assert.Equal(t, iptables.Prerouting, call.chainName)
				assert.Equal(t, "RETURN", call.target)
				assert.Contains(t, call.match, "-i azv1234")
			}

			// Verify MARK rule was appended.
			assert.Len(t, iptMock.appendCalls, tt.expectedAppends)
			if tt.expectedAppends > 0 {
				markCall := iptMock.appendCalls[0]
				assert.Equal(t, iptables.V4, markCall.version)
				assert.Equal(t, iptables.Mangle, markCall.tableName)
				assert.Equal(t, iptables.Prerouting, markCall.chainName)
				assert.Contains(t, markCall.match, "-i azv1234")
				assert.Contains(t, markCall.target, "MARK --set-mark 3")
			}

			// Verify netlink rule add.
			if tt.expectRuleAdd {
				require.Len(t, nlMock.ruleAddCalls, 1)
				assert.Equal(t, transparentTunnelFwmark, int(nlMock.ruleAddCalls[0].Mark))
				assert.Equal(t, transparentTunnelRouteTable, nlMock.ruleAddCalls[0].Table)
			}

			// Verify netlink route replace.
			if tt.expectRouteAdd {
				require.Len(t, nlMock.routeReplaceCalls, 1)
				assert.Equal(t, transparentTunnelRouteTable, nlMock.routeReplaceCalls[0].Table)
				assert.True(t, tt.gateway.Equal(nlMock.routeReplaceCalls[0].Gw))
			}
		})
	}
}

func TestTransparentTunnelDeleteEndpointRules(t *testing.T) {
	makeClient := func(plMock *transparentTunnelMockExecClient, nlMock *transparentTunnelMockNlClient) (*TransparentTunnelEndpointClient, *transparentTunnelMockIPTablesClient) {
		iptMock := &transparentTunnelMockIPTablesClient{}
		client := &TransparentTunnelEndpointClient{
			TransparentEndpointClient: &TransparentEndpointClient{
				hostVethName:      "azv1234",
				hostPrimaryIfName: "eth0",
				plClient:          plMock,
				netlink:           netlink.NewMockNetlink(false, ""),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			iptablesClient: iptMock,
			nlPolicyRoute:  nlMock,
			serviceCIDRs:   []string{"10.0.0.0/16"},
			gateway:        net.ParseIP("10.224.0.1"),
		}
		return client, iptMock
	}

	ep := &endpoint{
		HostIfName: "azv1234",
		IPAddresses: []net.IPNet{
			{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
		},
	}

	t.Run("last pod cleans up shared ip rule and route", func(t *testing.T) {
		// Mock returns empty iptables output — no remaining MARK rules.
		plMock := &transparentTunnelMockExecClient{
			cmdResponses: map[string]string{
				"iptables": "",
			},
		}
		nlMock := &transparentTunnelMockNlClient{}
		client, iptMock := makeClient(plMock, nlMock)

		client.deleteTransparentTunnelRules(ep)

		// Should delete service CIDR RETURN rule + MARK rule.
		assert.Len(t, iptMock.deleteCalls, 2)

		// Verify RETURN rule deletion.
		returnCall := iptMock.deleteCalls[0]
		assert.Equal(t, iptables.Mangle, returnCall.tableName)
		assert.Contains(t, returnCall.match, "-d 10.0.0.0/16")
		assert.Equal(t, "RETURN", returnCall.target)

		// Verify MARK rule deletion.
		markCall := iptMock.deleteCalls[1]
		assert.Equal(t, iptables.Mangle, markCall.tableName)
		assert.Contains(t, markCall.target, "MARK --set-mark 3")

		// Verify netlink cleanup: rule del + route del.
		assert.Len(t, nlMock.ruleDelCalls, 1)
		assert.Equal(t, transparentTunnelFwmark, int(nlMock.ruleDelCalls[0].Mark))
		assert.Len(t, nlMock.routeDelCalls, 1)
		assert.Equal(t, transparentTunnelRouteTable, nlMock.routeDelCalls[0].Table)
	})

	t.Run("other pods remain skips shared rule cleanup", func(t *testing.T) {
		// Mock returns iptables output with matching MARK rules still present.
		plMock := &transparentTunnelMockExecClient{
			cmdResponses: map[string]string{
				"iptables": "-A PREROUTING -i azv5678 -j MARK --set-xmark 0x3/0xffffffff\n-A PREROUTING -i azv9999 -j MARK --set-xmark 0x3/0xffffffff\n",
			},
		}
		nlMock := &transparentTunnelMockNlClient{}
		client, iptMock := makeClient(plMock, nlMock)

		client.deleteTransparentTunnelRules(ep)

		// Per-pod iptables rules still deleted.
		assert.Len(t, iptMock.deleteCalls, 2)

		// No netlink cleanup — other pods still active.
		assert.Empty(t, nlMock.ruleDelCalls, "should not delete shared rule")
		assert.Empty(t, nlMock.routeDelCalls, "should not delete shared route")
	})
}
