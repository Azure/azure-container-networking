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
	insertCalls               []iptablesCall
	appendCalls               []iptablesCall
	deleteCalls               []iptablesCall
	deleteIfExistsCalls       []iptablesCall
	// deleteErr, when non-nil, is returned from every DeleteIptableRule call.
	deleteErr error
	// deleteIfExistsErr, when non-nil, is returned from every
	// DeleteIptableRuleIfExists call. Distinct from deleteErr so tests can
	// drive the "real failure surfaces" vs "rule already absent" branches
	// independently of the legacy DeleteIptableRule mock.
	deleteIfExistsErr error
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

func (c *transparentTunnelMockIPTablesClient) DeleteIptableRuleIfExists(version, tableName, chainName, match, target string) error {
	c.deleteIfExistsCalls = append(c.deleteIfExistsCalls, iptablesCall{version, tableName, chainName, match, target})
	return c.deleteIfExistsErr
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
	ruleListCalls     int
	routeReplaceCalls []*vishnetlink.Route
	routeDelCalls     []*vishnetlink.Route
	ruleAddErr        error // injected error for RuleAdd
	ruleDelErr        error // injected error for RuleDel
	routeDelErr       error // injected error for RouteDel
	ruleListErr       error // injected error for RuleList
	// existingRules is what RuleList returns. The add path skips RuleAdd
	// when a matching (Mark, Table) rule is already present here.
	existingRules []vishnetlink.Rule
	// ruleDelSequence (optional) lets a test simulate "delete drains
	// stacked duplicates": each successive RuleDel pops the head error,
	// or returns nil if empty. When set, ruleDelErr is ignored.
	ruleDelSequence []error
}

func (c *transparentTunnelMockNlClient) RuleAdd(rule *vishnetlink.Rule) error {
	c.ruleAddCalls = append(c.ruleAddCalls, rule)
	return c.ruleAddErr
}

func (c *transparentTunnelMockNlClient) RuleDel(rule *vishnetlink.Rule) error {
	c.ruleDelCalls = append(c.ruleDelCalls, rule)
	if len(c.ruleDelSequence) > 0 {
		err := c.ruleDelSequence[0]
		c.ruleDelSequence = c.ruleDelSequence[1:]
		return err
	}
	return c.ruleDelErr
}

func (c *transparentTunnelMockNlClient) RuleList(_ int) ([]vishnetlink.Rule, error) {
	c.ruleListCalls++
	if c.ruleListErr != nil {
		return nil, c.ruleListErr
	}
	return c.existingRules, nil
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
		name               string
		ipAddresses        []net.IPNet
		gateway            net.IP
		epGateways         []net.IP // per-pod IPAM gateways (preferred over extIf gateway)
		ruleAddErr         error    // injected RuleAdd error (nil = success, EEXIST = tolerated)
		existingRules      []vishnetlink.Rule
		ruleListErr        error
		expectError        bool
		errorContains      string
		expectIpsetAdds    int  // number of ipset Add calls expected
		expectNotrackRule  bool // NOTRACK rule expected in raw PREROUTING
		expectRuleAddCalls int  // RuleAdd call count expected (0 if dedup skip)
	}{
		{
			name: "single ipv4 pod IP",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			gateway:            net.ParseIP("10.224.0.1"),
			expectIpsetAdds:    1,
			expectNotrackRule:  true,
			expectRuleAddCalls: 1,
		},
		{
			name: "dual-stack pod skips ipv6 from ipset",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
				{IP: net.ParseIP("fd00::1"), Mask: net.CIDRMask(128, 128)},
			},
			gateway:            net.ParseIP("10.224.0.1"),
			expectIpsetAdds:    1, // only IPv4
			expectNotrackRule:  true,
			expectRuleAddCalls: 1,
		},
		{
			name:               "no pod IPs still installs shared rules",
			ipAddresses:        nil,
			gateway:            net.ParseIP("10.224.0.1"),
			expectIpsetAdds:    0,
			expectNotrackRule:  true,
			expectRuleAddCalls: 1,
		},
		{
			name: "ip rule already in kernel — RuleAdd skipped (dedup)",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			gateway: net.ParseIP("10.224.0.1"),
			// Kernel already has a matching (Mark, Table) rule (e.g., from
			// a previous pod on the same node) — RuleAdd must not run.
			existingRules: []vishnetlink.Rule{
				{Mark: uint32(transparentTunnelFwmark), Table: transparentTunnelRouteTable, Priority: 32765},
			},
			expectIpsetAdds:    1,
			expectNotrackRule:  true,
			expectRuleAddCalls: 0,
		},
		{
			name: "ip rule with matching mark but different table — RuleAdd still runs",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			gateway: net.ParseIP("10.224.0.1"),
			existingRules: []vishnetlink.Rule{
				{Mark: uint32(transparentTunnelFwmark), Table: 254 /* main */, Priority: 32765},
			},
			expectIpsetAdds:    1,
			expectNotrackRule:  true,
			expectRuleAddCalls: 1,
		},
		{
			name: "concurrent add race — RuleAdd returns EEXIST is tolerated",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			gateway:            net.ParseIP("10.224.0.1"),
			ruleAddErr:         syscall.EEXIST,
			expectIpsetAdds:    1,
			expectNotrackRule:  true,
			expectRuleAddCalls: 1,
		},
		{
			name: "RuleList failure surfaces as ip rule error",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			gateway:       net.ParseIP("10.224.0.1"),
			ruleListErr:   assert.AnError,
			expectError:   true,
			errorContains: "list ip rules",
		},
		{
			name: "nil gateway returns error before creating any rules",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			gateway:       nil,
			expectError:   true,
			errorContains: "no usable IPv4 gateway",
		},
		{
			// Regression for §7.5: when extIf.IPv4Gateway was captured before
			// the host's default route was visible it persists as net.IPv4zero
			// ("0.0.0.0"). The previous `== nil` guard let it through and the
			// kernel installed a link-scoped default in table 101, black-holing
			// all non-link-local pod egress (Azure DNS, IMDS, ARM, kube-svc).
			name: "zero-IP extIf gateway with no pod gateway returns error",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			gateway:       net.IPv4zero,
			expectError:   true,
			errorContains: "no usable IPv4 gateway",
		},
		{
			// §7.5 happy path: extIf gateway is the zero IP, but IPAM supplied
			// a real per-pod gateway. pickTunnelGateway prefers it and the
			// route gets installed correctly with a real next-hop.
			name: "zero-IP extIf gateway with valid pod gateway succeeds via IPAM",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			gateway:            net.IPv4zero,
			epGateways:         []net.IP{net.ParseIP("10.224.0.1")},
			expectIpsetAdds:    1,
			expectNotrackRule:  true,
			expectRuleAddCalls: 1,
		},
		{
			// Confirms IPAM gateway is preferred even when extIf has a
			// distinct (and valid) value. Eliminates ambiguity in the
			// happy path where both sources are populated.
			name: "pod gateway is preferred over extIf gateway",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			gateway:            net.ParseIP("10.224.0.250"),
			epGateways:         []net.IP{net.ParseIP("10.224.0.1")},
			expectIpsetAdds:    1,
			expectNotrackRule:  true,
			expectRuleAddCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iptMock := &transparentTunnelMockIPTablesClient{}
			nlMock := &transparentTunnelMockNlClient{
				ruleAddErr:    tt.ruleAddErr,
				existingRules: tt.existingRules,
				ruleListErr:   tt.ruleListErr,
			}
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

			epInfo := &EndpointInfo{IPAddresses: tt.ipAddresses, Gateways: tt.epGateways}
			err := client.addTransparentTunnelRules(epInfo)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
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

			// RuleList must always run as the dedup pre-check.
			assert.Equal(t, 1, nlMock.ruleListCalls, "RuleList should run exactly once as dedup pre-check")

			// RuleAdd count depends on whether dedup skipped it.
			assert.Len(t, nlMock.ruleAddCalls, tt.expectRuleAddCalls)
			if tt.expectRuleAddCalls > 0 {
				assert.Equal(t, transparentTunnelFwmark, int(nlMock.ruleAddCalls[0].Mark))
				assert.Equal(t, transparentTunnelRouteTable, nlMock.ruleAddCalls[0].Table)
			}

			// Verify netlink route replace.
			require.Len(t, nlMock.routeReplaceCalls, 1)
			assert.Equal(t, transparentTunnelRouteTable, nlMock.routeReplaceCalls[0].Table)
			// Expected Gw mirrors pickTunnelGateway preference: pod IPAM gateway
			// wins over extIf gateway. Compute the same way the production code does.
			wantGw := pickTunnelGateway(&EndpointInfo{Gateways: tt.epGateways}, tt.gateway)
			require.NotNil(t, wantGw, "test setup error: expected non-nil gateway in success case")
			assert.True(t, wantGw.Equal(nlMock.routeReplaceCalls[0].Gw),
				"route Gw mismatch: got %v, want %v", nlMock.routeReplaceCalls[0].Gw, wantGw)
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
		// First RuleDel succeeds (drains the one shared rule); second
		// returns ENOENT so the drain loop in deleteTransparentTunnelRules
		// terminates.
		nlMock := &transparentTunnelMockNlClient{
			ruleDelSequence: []error{nil, syscall.ENOENT},
		}
		iptMock := &transparentTunnelMockIPTablesClient{}
		ipsetMock := &transparentTunnelMockIpsetClient{}
		client := makeClient(plMock, nlMock, iptMock, ipsetMock)

		require.NoError(t, client.DeleteTransparentTunnelRules(makeEndpoint()))

		// Per-pod ipset del should have been invoked once with the pod IP.
		assert.Equal(t, 1, ipsetMock.countOps("del"))
		assert.Equal(t, "10.224.0.46", ipsetMock.calls[0].arg)

		// MARK rule + NOTRACK rule deletes (both via the safe helper).
		require.Len(t, iptMock.deleteIfExistsCalls, 2)
		markCall := iptMock.deleteIfExistsCalls[0]
		assert.Equal(t, iptables.Mangle, markCall.tableName)
		assert.Contains(t, markCall.target, "MARK --set-mark 3")

		notrackCall := iptMock.deleteIfExistsCalls[1]
		assert.Equal(t, iptables.Raw, notrackCall.tableName)
		assert.Equal(t, iptables.Notrack, notrackCall.target)
		assert.Contains(t, notrackCall.match, "--match-set "+transparentTunnelLocalPodsSet+" src")
		assert.Contains(t, notrackCall.match, "--match-set "+transparentTunnelLocalPodsSet+" dst")
		// Legacy DeleteIptableRule must NOT be called — TT cleanup must use
		// the safe error-propagating helper.
		assert.Empty(t, iptMock.deleteCalls, "TT cleanup must use DeleteIptableRuleIfExists, not DeleteIptableRule")

		// Netlink cleanup: rule del loop drains exactly 2 attempts
		// (one success + one ENOENT terminator), then route del runs once.
		assert.Len(t, nlMock.ruleDelCalls, 2)
		assert.Equal(t, transparentTunnelFwmark, int(nlMock.ruleDelCalls[0].Mark))
		assert.Len(t, nlMock.routeDelCalls, 1)
		assert.Equal(t, transparentTunnelRouteTable, nlMock.routeDelCalls[0].Table)

		// ipset Destroy should have been called.
		assert.Equal(t, 1, ipsetMock.countOps("destroy"))
	})

	t.Run("delete loop drains stacked duplicate ip rules", func(t *testing.T) {
		// Simulate a node where the buggy old binary leaked 3 stacked
		// fwmark ip rules. The drain loop should call RuleDel 3 times
		// successfully and then once more to hit the ENOENT terminator,
		// leaving exactly zero rules behind.
		plMock := &transparentTunnelMockExecClient{
			cmdResponses: map[string]string{"iptables": ""},
		}
		nlMock := &transparentTunnelMockNlClient{
			ruleDelSequence: []error{nil, nil, nil, syscall.ENOENT},
		}
		iptMock := &transparentTunnelMockIPTablesClient{}
		ipsetMock := &transparentTunnelMockIpsetClient{}
		client := makeClient(plMock, nlMock, iptMock, ipsetMock)

		require.NoError(t, client.DeleteTransparentTunnelRules(makeEndpoint()))

		// 3 successful deletes + 1 ENOENT terminator == 4 calls total.
		assert.Len(t, nlMock.ruleDelCalls, 4, "drain loop should empty stacked duplicates and terminate on ENOENT")
	})

	t.Run("delete loop bounded by drain cap on non-terminating mock", func(t *testing.T) {
		// Defensive: if the netlink RuleDel never signals "not found",
		// the loop must still terminate via deleteIPRuleDrainCap so the
		// CNI DEL does not spin forever. We don't inject a sequence so
		// every RuleDel returns nil (the default), exercising the cap.
		plMock := &transparentTunnelMockExecClient{
			cmdResponses: map[string]string{"iptables": ""},
		}
		nlMock := &transparentTunnelMockNlClient{}
		iptMock := &transparentTunnelMockIPTablesClient{}
		ipsetMock := &transparentTunnelMockIpsetClient{}
		client := makeClient(plMock, nlMock, iptMock, ipsetMock)

		require.NoError(t, client.DeleteTransparentTunnelRules(makeEndpoint()))
		assert.Equal(t, deleteIPRuleDrainCap, len(nlMock.ruleDelCalls),
			"drain loop must cap at deleteIPRuleDrainCap even when RuleDel never returns ENOENT")
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
		assert.Len(t, iptMock.deleteIfExistsCalls, 1, "only the MARK rule should be deleted")
		assert.Contains(t, iptMock.deleteIfExistsCalls[0].target, "MARK --set-mark 3")

		// Shared state must be left alone.
		assert.Empty(t, nlMock.ruleDelCalls, "should not delete shared rule")
		assert.Empty(t, nlMock.routeDelCalls, "should not delete shared route")
		assert.Equal(t, 0, ipsetMock.countOps("destroy"), "should not destroy shared ipset")
	})

	t.Run("iptables already-absent rules are silently tolerated by helper", func(t *testing.T) {
		// The helper itself returns nil when the kernel says "rule does
		// not exist", so the mock receives the calls but the cleanup
		// surfaces no error.
		plMock := &transparentTunnelMockExecClient{
			cmdResponses: map[string]string{"iptables": ""},
		}
		nlMock := &transparentTunnelMockNlClient{
			ruleDelErr:  syscall.ENOENT,
			routeDelErr: syscall.ESRCH,
		}
		iptMock := &transparentTunnelMockIPTablesClient{
			// nil deleteIfExistsErr == helper returned no-error (rule absent
			// was caught and swallowed inside the helper itself).
		}
		ipsetMock := &transparentTunnelMockIpsetClient{}
		client := makeClient(plMock, nlMock, iptMock, ipsetMock)

		require.NoError(t, client.DeleteTransparentTunnelRules(makeEndpoint()))

		// MARK + NOTRACK delete attempts both ran through the safe helper.
		assert.Len(t, iptMock.deleteIfExistsCalls, 2,
			"both iptables deletes go through the helper, regardless of whether the rule was actually there")
		// Legacy DeleteIptableRule must not be invoked from TT cleanup.
		assert.Empty(t, iptMock.deleteCalls)
		// Netlink calls were still attempted; ENOENT/ESRCH must not surface.
		assert.Len(t, nlMock.ruleDelCalls, 1)
		assert.Len(t, nlMock.routeDelCalls, 1)
		// ipset Destroy still attempted.
		assert.Equal(t, 1, ipsetMock.countOps("destroy"))
	})

	t.Run("iptables real failure during cleanup is surfaced (xtables lock contention)", func(t *testing.T) {
		// §7.2 regression guard. The previous RuleExists()+Delete pattern
		// silently skipped the delete when RuleExists returned false on
		// xtables lock errors, leaving stale MARK/NOTRACK rules on the
		// host. With the new DeleteIptableRuleIfExists helper, any error
		// other than the kernel's explicit "rule does not exist" message
		// must propagate up so the runtime retries the CNI DEL.
		plMock := &transparentTunnelMockExecClient{
			cmdResponses: map[string]string{"iptables": ""},
		}
		nlMock := &transparentTunnelMockNlClient{ruleDelErr: syscall.ENOENT}
		iptMock := &transparentTunnelMockIPTablesClient{
			// Helper returns a real error — e.g., the wrapped exec error
			// from "Another app is currently holding the xtables lock".
			deleteIfExistsErr: errors.New("exit status 4:Another app is currently holding the xtables lock; waiting for it to exit"),
		}
		ipsetMock := &transparentTunnelMockIpsetClient{}
		client := makeClient(plMock, nlMock, iptMock, ipsetMock)

		err := client.DeleteTransparentTunnelRules(makeEndpoint())
		require.Error(t, err)
		// Both MARK and NOTRACK delete attempts failed and were surfaced.
		assert.Contains(t, err.Error(), "MARK")
		assert.Contains(t, err.Error(), "NOTRACK")
		assert.Len(t, iptMock.deleteIfExistsCalls, 2,
			"cleanup must attempt both deletes even after the first one fails")
	})

	t.Run("ipset destroy already-gone errors are benign", func(t *testing.T) {
		plMock := &transparentTunnelMockExecClient{
			cmdResponses: map[string]string{"iptables": ""},
		}
		nlMock := &transparentTunnelMockNlClient{ruleDelErr: syscall.ENOENT}
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
		nlMock := &transparentTunnelMockNlClient{ruleDelErr: syscall.ENOENT}
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
		nlMock := &transparentTunnelMockNlClient{ruleDelErr: syscall.ENOENT}
		iptMock := &transparentTunnelMockIPTablesClient{}
		ipsetMock := &transparentTunnelMockIpsetClient{delErr: assert.AnError}
		client := makeClient(plMock, nlMock, iptMock, ipsetMock)

		err := client.DeleteTransparentTunnelRules(makeEndpoint())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "10.224.0.46")
		// Cleanup still proceeded — MARK + NOTRACK deletes ran via the helper.
		assert.Len(t, iptMock.deleteIfExistsCalls, 2)
	})

	t.Run("iptables delete failure is surfaced", func(t *testing.T) {
		plMock := &transparentTunnelMockExecClient{
			cmdResponses: map[string]string{"iptables": ""},
		}
		nlMock := &transparentTunnelMockNlClient{ruleDelErr: syscall.ENOENT}
		iptMock := &transparentTunnelMockIPTablesClient{
			deleteIfExistsErr: assert.AnError,
		}
		ipsetMock := &transparentTunnelMockIpsetClient{}
		client := makeClient(plMock, nlMock, iptMock, ipsetMock)

		err := client.DeleteTransparentTunnelRules(makeEndpoint())
		require.Error(t, err)
		// Both the MARK and NOTRACK delete failures should surface.
		assert.Contains(t, err.Error(), "MARK")
		assert.Contains(t, err.Error(), "NOTRACK")

		// Cleanup proceeded for every step regardless of intermediate errors.
		assert.Len(t, iptMock.deleteIfExistsCalls, 2)
		// Drain loop terminated on first ENOENT (configured via ruleDelErr).
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
		// Per-pod cleanup still attempted (ipset Del + MARK delete via helper).
		assert.Equal(t, 1, ipsetMock.countOps("del"))
		assert.Len(t, iptMock.deleteIfExistsCalls, 1)
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
		assert.Empty(t, iptMock.deleteIfExistsCalls, "void DeleteEndpointRules must not touch iptables")
		assert.Empty(t, nlMock.ruleDelCalls, "void DeleteEndpointRules must not touch netlink rules")
		assert.Empty(t, nlMock.routeDelCalls, "void DeleteEndpointRules must not touch netlink routes")
		assert.Empty(t, ipsetMock.calls, "void DeleteEndpointRules must not touch ipset")
		assert.Empty(t, plMock.executedCmds, "void DeleteEndpointRules must not exec iptables -S for refcount")
	})
}

// TestPickTunnelGateway covers all permutations of the per-pod IPAM gateway
// vs the host extIf gateway. This is the §7.5 regression test surface — a
// "0.0.0.0" extIf gateway (persisted from a host whose default route was
// not visible at network-creation time) MUST NOT be returned, and the
// IPAM gateway MUST be preferred when both are populated.
func TestPickTunnelGateway(t *testing.T) {
	v4 := func(s string) net.IP { return net.ParseIP(s).To4() }
	v6 := func(s string) net.IP { return net.ParseIP(s) }

	tests := []struct {
		name       string
		epGateways []net.IP
		extIfGw    net.IP
		want       net.IP
	}{
		{
			name:       "both nil returns nil",
			epGateways: nil,
			extIfGw:    nil,
			want:       nil,
		},
		{
			name:       "zero extIf and no pod returns nil",
			epGateways: nil,
			extIfGw:    net.IPv4zero,
			want:       nil,
		},
		{
			name:       "valid extIf only is used",
			epGateways: nil,
			extIfGw:    v4("10.224.0.1"),
			want:       v4("10.224.0.1"),
		},
		{
			name:       "valid pod gateway is preferred over valid extIf",
			epGateways: []net.IP{v4("10.224.0.1")},
			extIfGw:    v4("10.224.0.250"),
			want:       v4("10.224.0.1"),
		},
		{
			name:       "pod zero-IP falls back to extIf",
			epGateways: []net.IP{net.IPv4zero},
			extIfGw:    v4("10.224.0.1"),
			want:       v4("10.224.0.1"),
		},
		{
			name:       "pod ipv6 only falls back to extIf",
			epGateways: []net.IP{v6("fe80::1")},
			extIfGw:    v4("10.224.0.1"),
			want:       v4("10.224.0.1"),
		},
		{
			name:       "pod ipv6 then ipv4 picks the ipv4",
			epGateways: []net.IP{v6("fe80::1"), v4("10.224.0.1")},
			extIfGw:    nil,
			want:       v4("10.224.0.1"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickTunnelGateway(&EndpointInfo{Gateways: tt.epGateways}, tt.extIfGw)
			if tt.want == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.True(t, got.Equal(tt.want), "got %v, want %v", got, tt.want)
		})
	}

	t.Run("nil epInfo falls back to extIf without panicking", func(t *testing.T) {
		got := pickTunnelGateway(nil, v4("10.224.0.1"))
		require.NotNil(t, got)
		assert.True(t, got.Equal(v4("10.224.0.1")))
	})
}
