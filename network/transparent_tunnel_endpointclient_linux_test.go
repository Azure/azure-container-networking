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
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	vishnetlink "github.com/vishvananda/netlink"
)

// transparentTunnelMockIPTablesClient tracks all iptables calls for test verification.
type transparentTunnelMockIPTablesClient struct {
	insertCalls []iptablesCall
	appendCalls []iptablesCall
	deleteCalls []iptablesCall
	// deleteErr, when non-nil, is returned from every DeleteIptableRule call.
	deleteErr error
	// appendErr, when non-nil, is returned from every AppendIptableRule call.
	appendErr error
	// ruleExistsFn, when non-nil, decides whether a given rule exists. Defaults
	// to "exists" when nil so existing tests continue to invoke deletes.
	ruleExistsFn func(version, tableName, chainName, match, target string) bool
}

func (c *transparentTunnelMockIPTablesClient) InsertIptableRule(version, tableName, chainName, match, target string) error {
	c.insertCalls = append(c.insertCalls, iptablesCall{version, tableName, chainName, match, target})
	return nil
}

func (c *transparentTunnelMockIPTablesClient) AppendIptableRule(version, tableName, chainName, match, target string) error {
	c.appendCalls = append(c.appendCalls, iptablesCall{version, tableName, chainName, match, target})
	return c.appendErr
}

func (c *transparentTunnelMockIPTablesClient) DeleteIptableRule(version, tableName, chainName, match, target string) error {
	c.deleteCalls = append(c.deleteCalls, iptablesCall{version, tableName, chainName, match, target})
	return c.deleteErr
}

func (c *transparentTunnelMockIPTablesClient) RuleExists(version, tableName, chainName, match, target string) bool {
	if c.ruleExistsFn != nil {
		return c.ruleExistsFn(version, tableName, chainName, match, target)
	}
	return true
}

func (c *transparentTunnelMockIPTablesClient) CreateChain(_, _, _ string) error { return nil }
func (c *transparentTunnelMockIPTablesClient) RunCmd(_, _ string) error         { return nil }

// transparentTunnelMockExecClient tracks executed commands and returns canned responses.
type transparentTunnelMockExecClient struct {
	platform.ExecClient
	executedCmds []string
	// cmdResponses maps a substring to the response returned when a command contains it.
	cmdResponses map[string]string
	// cmdErr is returned from ExecuteCommand when non-nil (overrides cmdResponses).
	cmdErr error
}

func (c *transparentTunnelMockExecClient) ExecuteCommand(_ context.Context, cmd string, args ...string) (string, error) {
	full := cmd + " " + strings.Join(args, " ")
	c.executedCmds = append(c.executedCmds, full)
	if c.cmdErr != nil {
		return "", c.cmdErr
	}
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
	ruleDelErr        error // injected error for RuleDel
	routeDelErr       error // injected error for RouteDel
}

func (c *transparentTunnelMockNlClient) RuleAdd(rule *vishnetlink.Rule) error {
	c.ruleAddCalls = append(c.ruleAddCalls, rule)
	return c.ruleAddErr
}

func (c *transparentTunnelMockNlClient) RuleDel(rule *vishnetlink.Rule) error {
	c.ruleDelCalls = append(c.ruleDelCalls, rule)
	return c.ruleDelErr
}

func (c *transparentTunnelMockNlClient) RouteReplace(route *vishnetlink.Route) error {
	c.routeReplaceCalls = append(c.routeReplaceCalls, route)
	return nil
}

func (c *transparentTunnelMockNlClient) RouteDel(route *vishnetlink.Route) error {
	c.routeDelCalls = append(c.routeDelCalls, route)
	return c.routeDelErr
}

// ipsetCall records a single ipset operation made by the mock client.
type ipsetCall struct {
	op    string // "create" | "add" | "del" | "destroy"
	set   string
	arg   string // setType for create; entry for add/del; "" for destroy
}

// transparentTunnelMockIpsetClient records ipset operations for verification
// and lets tests inject per-op errors.
type transparentTunnelMockIpsetClient struct {
	calls      []ipsetCall
	createErr  error
	addErr     error
	delErr     error
	destroyErr error
}

func (c *transparentTunnelMockIpsetClient) Create(setName, setType string) error {
	c.calls = append(c.calls, ipsetCall{op: "create", set: setName, arg: setType})
	return c.createErr
}

func (c *transparentTunnelMockIpsetClient) Add(setName, entry string) error {
	c.calls = append(c.calls, ipsetCall{op: "add", set: setName, arg: entry})
	return c.addErr
}

func (c *transparentTunnelMockIpsetClient) Del(setName, entry string) error {
	c.calls = append(c.calls, ipsetCall{op: "del", set: setName, arg: entry})
	return c.delErr
}

func (c *transparentTunnelMockIpsetClient) Destroy(setName string) error {
	c.calls = append(c.calls, ipsetCall{op: "destroy", set: setName})
	return c.destroyErr
}

// countOps returns the number of calls matching op.
func (c *transparentTunnelMockIpsetClient) countOps(op string) int {
	n := 0
	for _, call := range c.calls {
		if call.op == op {
			n++
		}
	}
	return n
}

func TestTransparentTunnelAddEndpointRules(t *testing.T) {
	tests := []struct {
		name              string
		ipAddresses       []net.IPNet
		gateway           net.IP
		ruleAddErr        error // injected RuleAdd error (nil = success, EEXIST = tolerated)
		expectError       bool
		errorContains     string
		expectIpsetAdds   int  // number of ipset Add calls expected
		expectNotrackRule bool // NOTRACK rule expected in raw PREROUTING
	}{
		{
			name: "single ipv4 pod IP",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			gateway:           net.ParseIP("10.224.0.1"),
			expectIpsetAdds:   1,
			expectNotrackRule: true,
		},
		{
			name: "dual-stack pod skips ipv6 from ipset",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
				{IP: net.ParseIP("fd00::1"), Mask: net.CIDRMask(128, 128)},
			},
			gateway:           net.ParseIP("10.224.0.1"),
			expectIpsetAdds:   1, // only IPv4
			expectNotrackRule: true,
		},
		{
			name:              "no pod IPs still installs shared rules",
			ipAddresses:       nil,
			gateway:           net.ParseIP("10.224.0.1"),
			expectIpsetAdds:   0,
			expectNotrackRule: true,
		},
		{
			name: "rule already exists is tolerated",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			gateway:           net.ParseIP("10.224.0.1"),
			ruleAddErr:        syscall.EEXIST,
			expectIpsetAdds:   1,
			expectNotrackRule: true,
		},
		{
			name: "nil gateway returns error before creating any rules",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			gateway:       nil,
			expectError:   true,
			errorContains: "gateway is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iptMock := &transparentTunnelMockIPTablesClient{}
			nlMock := &transparentTunnelMockNlClient{ruleAddErr: tt.ruleAddErr}
			ipsetMock := &transparentTunnelMockIpsetClient{}

			client := &TransparentTunnelEndpointClient{
				TransparentEndpointClient: &TransparentEndpointClient{
					hostVethName:      "azv1234",
					hostPrimaryIfName: "eth0",
					netioshim:         netio.NewMockNetIO(false, 0),
				},
				iptablesClient: iptMock,
				nlPolicyRoute:  nlMock,
				ipsetClient:    ipsetMock,
				gateway:        tt.gateway,
			}

			epInfo := &EndpointInfo{IPAddresses: tt.ipAddresses}
			err := client.addTransparentTunnelRules(epInfo)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
				assert.Empty(t, iptMock.appendCalls, "no iptables rules should be created on error")
				assert.Empty(t, nlMock.ruleAddCalls, "no netlink calls should run on error")
				assert.Empty(t, ipsetMock.calls, "no ipset calls should run on error")
				return
			}

			require.NoError(t, err)

			// Always create the ipset exactly once.
			assert.Equal(t, 1, ipsetMock.countOps("create"), "ipset create should run exactly once")
			createCall := ipsetMock.calls[0]
			assert.Equal(t, "create", createCall.op)
			assert.Equal(t, transparentTunnelLocalPodsSet, createCall.set)
			assert.Equal(t, transparentTunnelLocalPodsSetType, createCall.arg)

			// Add one entry per IPv4 pod IP.
			assert.Equal(t, tt.expectIpsetAdds, ipsetMock.countOps("add"))
			for _, call := range ipsetMock.calls {
				if call.op != "add" {
					continue
				}
				assert.Equal(t, transparentTunnelLocalPodsSet, call.set)
				ip := net.ParseIP(call.arg)
				require.NotNil(t, ip, "ipset add entry must parse as IP: %s", call.arg)
				assert.NotNil(t, ip.To4(), "ipset entries must be IPv4: %s", call.arg)
			}

			// NOTRACK rule + MARK rule should both be appended.
			require.Len(t, iptMock.appendCalls, 2, "expected NOTRACK and MARK appends")

			// NOTRACK rule (raw table).
			notrackCall := iptMock.appendCalls[0]
			assert.Equal(t, iptables.V4, notrackCall.version)
			assert.Equal(t, iptables.Raw, notrackCall.tableName)
			assert.Equal(t, iptables.Prerouting, notrackCall.chainName)
			assert.Equal(t, iptables.Notrack, notrackCall.target)
			assert.Contains(t, notrackCall.match, "-i eth0")
			assert.Contains(t, notrackCall.match, "--match-set "+transparentTunnelLocalPodsSet+" src")
			assert.Contains(t, notrackCall.match, "--match-set "+transparentTunnelLocalPodsSet+" dst")

			// MARK rule (mangle table).
			markCall := iptMock.appendCalls[1]
			assert.Equal(t, iptables.V4, markCall.version)
			assert.Equal(t, iptables.Mangle, markCall.tableName)
			assert.Equal(t, iptables.Prerouting, markCall.chainName)
			assert.Contains(t, markCall.match, "-i azv1234")
			assert.Contains(t, markCall.target, "MARK --set-mark 3")

			// Verify netlink rule add.
			require.Len(t, nlMock.ruleAddCalls, 1)
			assert.Equal(t, transparentTunnelFwmark, int(nlMock.ruleAddCalls[0].Mark))
			assert.Equal(t, transparentTunnelRouteTable, nlMock.ruleAddCalls[0].Table)

			// Verify netlink route replace.
			require.Len(t, nlMock.routeReplaceCalls, 1)
			assert.Equal(t, transparentTunnelRouteTable, nlMock.routeReplaceCalls[0].Table)
			assert.True(t, tt.gateway.Equal(nlMock.routeReplaceCalls[0].Gw))
		})
	}
}

func TestTransparentTunnelAddEndpointRules_IpsetCreateFails(t *testing.T) {
	iptMock := &transparentTunnelMockIPTablesClient{}
	nlMock := &transparentTunnelMockNlClient{}
	ipsetMock := &transparentTunnelMockIpsetClient{createErr: assert.AnError}

	client := &TransparentTunnelEndpointClient{
		TransparentEndpointClient: &TransparentEndpointClient{
			hostVethName:      "azv1234",
			hostPrimaryIfName: "eth0",
			netioshim:         netio.NewMockNetIO(false, 0),
		},
		iptablesClient: iptMock,
		nlPolicyRoute:  nlMock,
		ipsetClient:    ipsetMock,
		gateway:        net.ParseIP("10.224.0.1"),
	}

	epInfo := &EndpointInfo{IPAddresses: []net.IPNet{
		{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
	}}
	err := client.addTransparentTunnelRules(epInfo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "local-pods ipset")
	// No subsequent operations should have run after the ipset create failure.
	assert.Empty(t, iptMock.appendCalls)
	assert.Empty(t, nlMock.ruleAddCalls)
}

func TestTransparentTunnelAddEndpointRules_IpsetAddFails(t *testing.T) {
	iptMock := &transparentTunnelMockIPTablesClient{}
	nlMock := &transparentTunnelMockNlClient{}
	ipsetMock := &transparentTunnelMockIpsetClient{addErr: assert.AnError}

	client := &TransparentTunnelEndpointClient{
		TransparentEndpointClient: &TransparentEndpointClient{
			hostVethName:      "azv1234",
			hostPrimaryIfName: "eth0",
			netioshim:         netio.NewMockNetIO(false, 0),
		},
		iptablesClient: iptMock,
		nlPolicyRoute:  nlMock,
		ipsetClient:    ipsetMock,
		gateway:        net.ParseIP("10.224.0.1"),
	}

	epInfo := &EndpointInfo{IPAddresses: []net.IPNet{
		{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
	}}
	err := client.addTransparentTunnelRules(epInfo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "10.224.0.46")
	// Create ran, Add ran (and failed); nothing else should have.
	assert.Empty(t, iptMock.appendCalls)
	assert.Empty(t, nlMock.ruleAddCalls)
}

func TestTransparentTunnelDeleteEndpointRules(t *testing.T) {
	makeClient := func(plMock *transparentTunnelMockExecClient,
		nlMock *transparentTunnelMockNlClient,
		iptMock *transparentTunnelMockIPTablesClient,
		ipsetMock *transparentTunnelMockIpsetClient,
	) *TransparentTunnelEndpointClient {
		return &TransparentTunnelEndpointClient{
			TransparentEndpointClient: &TransparentEndpointClient{
				hostVethName:      "azv1234",
				hostPrimaryIfName: "eth0",
				plClient:          plMock,
				netlink:           netlink.NewMockNetlink(false, ""),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			iptablesClient: iptMock,
			nlPolicyRoute:  nlMock,
			ipsetClient:    ipsetMock,
			gateway:        net.ParseIP("10.224.0.1"),
		}
	}

	makeEndpoint := func() *endpoint {
		return &endpoint{
			HostIfName: "azv1234",
			IPAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
		}
	}

	t.Run("last pod cleans up shared NOTRACK, ip rule, route, and ipset", func(t *testing.T) {
		// Mock returns empty iptables output — no remaining MARK rules.
		plMock := &transparentTunnelMockExecClient{
			cmdResponses: map[string]string{"iptables": ""},
		}
		nlMock := &transparentTunnelMockNlClient{}
		iptMock := &transparentTunnelMockIPTablesClient{}
		ipsetMock := &transparentTunnelMockIpsetClient{}
		client := makeClient(plMock, nlMock, iptMock, ipsetMock)

		require.NoError(t, client.DeleteTransparentTunnelRules(makeEndpoint()))

		// Per-pod ipset del should have been invoked once with the pod IP.
		assert.Equal(t, 1, ipsetMock.countOps("del"))
		assert.Equal(t, "10.224.0.46", ipsetMock.calls[0].arg)

		// MARK rule + NOTRACK rule deletes.
		require.Len(t, iptMock.deleteCalls, 2)
		markCall := iptMock.deleteCalls[0]
		assert.Equal(t, iptables.Mangle, markCall.tableName)
		assert.Contains(t, markCall.target, "MARK --set-mark 3")

		notrackCall := iptMock.deleteCalls[1]
		assert.Equal(t, iptables.Raw, notrackCall.tableName)
		assert.Equal(t, iptables.Notrack, notrackCall.target)
		assert.Contains(t, notrackCall.match, "--match-set "+transparentTunnelLocalPodsSet+" src")
		assert.Contains(t, notrackCall.match, "--match-set "+transparentTunnelLocalPodsSet+" dst")

		// Netlink cleanup: rule del + route del.
		assert.Len(t, nlMock.ruleDelCalls, 1)
		assert.Equal(t, transparentTunnelFwmark, int(nlMock.ruleDelCalls[0].Mark))
		assert.Len(t, nlMock.routeDelCalls, 1)
		assert.Equal(t, transparentTunnelRouteTable, nlMock.routeDelCalls[0].Table)

		// ipset Destroy should have been called.
		assert.Equal(t, 1, ipsetMock.countOps("destroy"))
	})

	t.Run("other pods remain skips shared cleanup", func(t *testing.T) {
		// Mock returns iptables output with matching MARK rules still present.
		plMock := &transparentTunnelMockExecClient{
			cmdResponses: map[string]string{
				"iptables": "-A PREROUTING -i azv5678 -j MARK --set-xmark 0x3/0xffffffff\n-A PREROUTING -i azv9999 -j MARK --set-xmark 0x3/0xffffffff\n",
			},
		}
		nlMock := &transparentTunnelMockNlClient{}
		iptMock := &transparentTunnelMockIPTablesClient{}
		ipsetMock := &transparentTunnelMockIpsetClient{}
		client := makeClient(plMock, nlMock, iptMock, ipsetMock)

		require.NoError(t, client.DeleteTransparentTunnelRules(makeEndpoint()))

		// Per-pod cleanup still ran: ipset Del + MARK rule delete.
		assert.Equal(t, 1, ipsetMock.countOps("del"))
		assert.Len(t, iptMock.deleteCalls, 1, "only the MARK rule should be deleted")
		assert.Contains(t, iptMock.deleteCalls[0].target, "MARK --set-mark 3")

		// Shared state must be left alone.
		assert.Empty(t, nlMock.ruleDelCalls, "should not delete shared rule")
		assert.Empty(t, nlMock.routeDelCalls, "should not delete shared route")
		assert.Equal(t, 0, ipsetMock.countOps("destroy"), "should not destroy shared ipset")
	})

	t.Run("rules already absent skipped via RuleExists pre-check", func(t *testing.T) {
		plMock := &transparentTunnelMockExecClient{
			cmdResponses: map[string]string{"iptables": ""},
		}
		nlMock := &transparentTunnelMockNlClient{
			ruleDelErr:  syscall.ENOENT,
			routeDelErr: syscall.ESRCH,
		}
		iptMock := &transparentTunnelMockIPTablesClient{
			// Pretend all iptables rules are already gone.
			ruleExistsFn: func(_, _, _, _, _ string) bool { return false },
		}
		ipsetMock := &transparentTunnelMockIpsetClient{}
		client := makeClient(plMock, nlMock, iptMock, ipsetMock)

		// Idempotent: absent rules surface no error.
		require.NoError(t, client.DeleteTransparentTunnelRules(makeEndpoint()))

		// No DeleteIptableRule calls because RuleExists returned false.
		assert.Empty(t, iptMock.deleteCalls, "should not invoke delete for absent rules")
		// Netlink calls were still attempted; ENOENT/ESRCH must not surface.
		assert.Len(t, nlMock.ruleDelCalls, 1)
		assert.Len(t, nlMock.routeDelCalls, 1)
		// ipset Destroy still attempted.
		assert.Equal(t, 1, ipsetMock.countOps("destroy"))
	})

	t.Run("ipset destroy already-gone errors are benign", func(t *testing.T) {
		plMock := &transparentTunnelMockExecClient{
			cmdResponses: map[string]string{"iptables": ""},
		}
		nlMock := &transparentTunnelMockNlClient{}
		iptMock := &transparentTunnelMockIPTablesClient{}
		ipsetMock := &transparentTunnelMockIpsetClient{
			destroyErr: errors.New("ipset destroy azure-tt-local-pods: The set with the given name does not exist"),
		}
		client := makeClient(plMock, nlMock, iptMock, ipsetMock)

		require.NoError(t, client.DeleteTransparentTunnelRules(makeEndpoint()))
		assert.Equal(t, 1, ipsetMock.countOps("destroy"))
	})

	t.Run("ipset destroy real failure is surfaced", func(t *testing.T) {
		plMock := &transparentTunnelMockExecClient{
			cmdResponses: map[string]string{"iptables": ""},
		}
		nlMock := &transparentTunnelMockNlClient{}
		iptMock := &transparentTunnelMockIPTablesClient{}
		ipsetMock := &transparentTunnelMockIpsetClient{
			destroyErr: errors.New("ipset destroy: set is in use by a kernel component"),
		}
		client := makeClient(plMock, nlMock, iptMock, ipsetMock)

		err := client.DeleteTransparentTunnelRules(makeEndpoint())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ipset")
	})

	t.Run("ipset del failure is surfaced", func(t *testing.T) {
		plMock := &transparentTunnelMockExecClient{
			cmdResponses: map[string]string{"iptables": ""},
		}
		nlMock := &transparentTunnelMockNlClient{}
		iptMock := &transparentTunnelMockIPTablesClient{}
		ipsetMock := &transparentTunnelMockIpsetClient{delErr: assert.AnError}
		client := makeClient(plMock, nlMock, iptMock, ipsetMock)

		err := client.DeleteTransparentTunnelRules(makeEndpoint())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "10.224.0.46")
		// Cleanup still proceeded — MARK + NOTRACK deletes ran.
		assert.Len(t, iptMock.deleteCalls, 2)
	})

	t.Run("iptables delete failure is surfaced", func(t *testing.T) {
		plMock := &transparentTunnelMockExecClient{
			cmdResponses: map[string]string{"iptables": ""},
		}
		nlMock := &transparentTunnelMockNlClient{}
		iptMock := &transparentTunnelMockIPTablesClient{
			deleteErr: assert.AnError,
		}
		ipsetMock := &transparentTunnelMockIpsetClient{}
		client := makeClient(plMock, nlMock, iptMock, ipsetMock)

		err := client.DeleteTransparentTunnelRules(makeEndpoint())
		require.Error(t, err)
		// Both the MARK and NOTRACK delete failures should surface.
		assert.Contains(t, err.Error(), "MARK")
		assert.Contains(t, err.Error(), "NOTRACK")

		// Cleanup proceeded for every step regardless of intermediate errors.
		assert.Len(t, iptMock.deleteCalls, 2)
		assert.Len(t, nlMock.ruleDelCalls, 1)
		assert.Len(t, nlMock.routeDelCalls, 1)
	})

	t.Run("netlink RuleDel failure other than ENOENT is surfaced", func(t *testing.T) {
		plMock := &transparentTunnelMockExecClient{
			cmdResponses: map[string]string{"iptables": ""},
		}
		nlMock := &transparentTunnelMockNlClient{
			ruleDelErr: syscall.EPERM, // permission denied — not "already gone"
		}
		iptMock := &transparentTunnelMockIPTablesClient{}
		ipsetMock := &transparentTunnelMockIpsetClient{}
		client := makeClient(plMock, nlMock, iptMock, ipsetMock)

		err := client.DeleteTransparentTunnelRules(makeEndpoint())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ip rule")
	})

	t.Run("netlink RouteDel failure other than ENOENT is surfaced", func(t *testing.T) {
		plMock := &transparentTunnelMockExecClient{
			cmdResponses: map[string]string{"iptables": ""},
		}
		nlMock := &transparentTunnelMockNlClient{
			routeDelErr: syscall.EPERM,
		}
		iptMock := &transparentTunnelMockIPTablesClient{}
		ipsetMock := &transparentTunnelMockIpsetClient{}
		client := makeClient(plMock, nlMock, iptMock, ipsetMock)

		err := client.DeleteTransparentTunnelRules(makeEndpoint())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "route")
	})

	t.Run("refcount list failure skips shared teardown and surfaces error", func(t *testing.T) {
		plMock := &transparentTunnelMockExecClient{
			cmdErr: assert.AnError,
		}
		nlMock := &transparentTunnelMockNlClient{}
		iptMock := &transparentTunnelMockIPTablesClient{}
		ipsetMock := &transparentTunnelMockIpsetClient{}
		client := makeClient(plMock, nlMock, iptMock, ipsetMock)

		err := client.DeleteTransparentTunnelRules(makeEndpoint())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "refcount")
		// Per-pod cleanup still attempted (ipset Del + MARK delete).
		assert.Equal(t, 1, ipsetMock.countOps("del"))
		assert.Len(t, iptMock.deleteCalls, 1)
		// Shared teardown skipped because we can't safely refcount.
		assert.Empty(t, nlMock.ruleDelCalls, "must not tear down shared state when refcount unknown")
		assert.Empty(t, nlMock.routeDelCalls, "must not tear down shared state when refcount unknown")
		assert.Equal(t, 0, ipsetMock.countOps("destroy"), "must not destroy shared ipset when refcount unknown")
	})

	t.Run("DeleteEndpointRules (interface, void) does NOT touch tunnel state", func(t *testing.T) {
		plMock := &transparentTunnelMockExecClient{}
		nlMock := &transparentTunnelMockNlClient{}
		iptMock := &transparentTunnelMockIPTablesClient{}
		ipsetMock := &transparentTunnelMockIpsetClient{}
		client := makeClient(plMock, nlMock, iptMock, ipsetMock)

		// The void interface satisfier must only delegate to the base; all
		// transparent-tunnel cleanup must go through DeleteTransparentTunnelRules
		// so failures are returned to the caller.
		client.DeleteEndpointRules(makeEndpoint())

		assert.Empty(t, iptMock.deleteCalls, "void DeleteEndpointRules must not touch iptables")
		assert.Empty(t, nlMock.ruleDelCalls, "void DeleteEndpointRules must not touch netlink rules")
		assert.Empty(t, nlMock.routeDelCalls, "void DeleteEndpointRules must not touch netlink routes")
		assert.Empty(t, ipsetMock.calls, "void DeleteEndpointRules must not touch ipset")
		assert.Empty(t, plMock.executedCmds, "void DeleteEndpointRules must not exec iptables -S for refcount")
	})
}
