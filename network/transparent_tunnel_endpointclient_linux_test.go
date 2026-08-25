package network

import (
	"net"
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

const testHostVethName = "azv1234"

// testHostLinkIndex is the index MockNetIO returns for the primary interface.
const testHostLinkIndex = 2

// testHostDefaultRoutes is the happy-path netlink reply for the host's IPv4
// default route on the primary interface: the single source resolveTunnelGateway
// reads. Tests that only care about other behaviour use it so gateway
// resolution succeeds and does not short-circuit the add path.
func testHostDefaultRoutes() []vishnetlink.Route {
	return []vishnetlink.Route{{LinkIndex: testHostLinkIndex, Gw: net.ParseIP("10.224.0.1")}}
}

// transparentTunnelMockIPTablesClient tracks all iptables calls for test verification.
type transparentTunnelMockIPTablesClient struct {
	insertCalls []iptablesCall
	appendCalls []iptablesCall
	deleteCalls []iptablesCall
	// deleteErr, when non-nil, is returned from every DeleteIptableRule call.
	// Tests drive the "rule already absent" vs "real failure surfaces" branches
	// by returning either an iptables missing-rule error or another error.
	deleteErr error
	// appendErr, when non-nil, is returned from every AppendIptableRule call.
	appendErr error
	// ruleExistsFn, when non-nil, decides whether a given rule exists. Defaults
	// to "absent" when nil so a fresh node installs its shared rules.
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
	return false
}

func (c *transparentTunnelMockIPTablesClient) CreateChain(_, _, _ string) error { return nil }
func (c *transparentTunnelMockIPTablesClient) RunCmd(_, _ string) error         { return nil }

// transparentTunnelMockNlClient tracks netlink rule/route calls for test verification.
type transparentTunnelMockNlClient struct {
	ruleAddCalls        []*vishnetlink.Rule
	ruleListCalls       int
	routeListCalls      int
	routeListFilter     *vishnetlink.Route
	routeListFilterMask uint64
	routeReplaceCalls   []*vishnetlink.Route
	ruleAddErr          error // injected error for RuleAdd
	ruleListErr         error // injected error for RuleList
	routeListErr        error // injected error for RouteListFiltered
	defaultRoutes       []vishnetlink.Route
	// existingRules is what RuleList returns. The add path skips RuleAdd
	// when a matching (Mark, Table) rule is already present here.
	existingRules []vishnetlink.Rule
}

func (c *transparentTunnelMockNlClient) RuleAdd(rule *vishnetlink.Rule) error {
	c.ruleAddCalls = append(c.ruleAddCalls, rule)
	return c.ruleAddErr
}

func (c *transparentTunnelMockNlClient) RuleList(_ int) ([]vishnetlink.Rule, error) {
	c.ruleListCalls++
	if c.ruleListErr != nil {
		return nil, c.ruleListErr
	}
	return c.existingRules, nil
}

func (c *transparentTunnelMockNlClient) RouteListFiltered(_ int, filter *vishnetlink.Route, filterMask uint64) ([]vishnetlink.Route, error) {
	c.routeListCalls++
	c.routeListFilter = filter
	c.routeListFilterMask = filterMask
	if c.routeListErr != nil {
		return nil, c.routeListErr
	}
	return c.defaultRoutes, nil
}

func (c *transparentTunnelMockNlClient) RouteReplace(route *vishnetlink.Route) error {
	c.routeReplaceCalls = append(c.routeReplaceCalls, route)
	return nil
}

// ipsetCall records a single ipset operation made by the mock client.
type ipsetCall struct {
	op  string // "create" | "add" | "del" | "destroy"
	set string
	arg string // setType for create; entry for add/del; "" for destroy
}

// transparentTunnelMockIpsetClient records ipset operations for verification
// and lets tests inject per-op errors.
type transparentTunnelMockIpsetClient struct {
	calls     []ipsetCall
	createErr error
	addErr    error
	delErr    error
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
	c.calls = append(c.calls, ipsetCall{op: ipsetOpDel, set: setName, arg: entry})
	return c.delErr
}

func (c *transparentTunnelMockIpsetClient) Destroy(setName string) error {
	c.calls = append(c.calls, ipsetCall{op: "destroy", set: setName})
	return nil
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
		ruleAddErr         error // injected RuleAdd error (nil = success, EEXIST = tolerated)
		existingRules      []vishnetlink.Rule
		ruleListErr        error
		routeListErr       error
		defaultRoutes      []vishnetlink.Route
		wantGateway        net.IP
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
			defaultRoutes:      testHostDefaultRoutes(),
			wantGateway:        net.ParseIP("10.224.0.1"),
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
			defaultRoutes:      testHostDefaultRoutes(),
			wantGateway:        net.ParseIP("10.224.0.1"),
			expectIpsetAdds:    1, // only IPv4
			expectNotrackRule:  true,
			expectRuleAddCalls: 1,
		},
		{
			name:               "no pod IPs still installs shared rules",
			ipAddresses:        nil,
			defaultRoutes:      testHostDefaultRoutes(),
			wantGateway:        net.ParseIP("10.224.0.1"),
			expectIpsetAdds:    0,
			expectNotrackRule:  true,
			expectRuleAddCalls: 1,
		},
		{
			name: "ip rule already in kernel — RuleAdd skipped (dedup)",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			// Kernel already has a matching (Mark, Table) rule (e.g., from
			// a previous pod on the same node) — RuleAdd must not run.
			existingRules: []vishnetlink.Rule{
				{Mark: uint32(transparentTunnelFwmark), Table: transparentTunnelRouteTable, Priority: 32765},
			},
			defaultRoutes:      testHostDefaultRoutes(),
			wantGateway:        net.ParseIP("10.224.0.1"),
			expectIpsetAdds:    1,
			expectNotrackRule:  true,
			expectRuleAddCalls: 0,
		},
		{
			name: "ip rule with matching mark but different table — RuleAdd still runs",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			existingRules: []vishnetlink.Rule{
				{Mark: uint32(transparentTunnelFwmark), Table: 254 /* main */, Priority: 32765},
			},
			defaultRoutes:      testHostDefaultRoutes(),
			wantGateway:        net.ParseIP("10.224.0.1"),
			expectIpsetAdds:    1,
			expectNotrackRule:  true,
			expectRuleAddCalls: 1,
		},
		{
			name: "concurrent add race — RuleAdd returns EEXIST is tolerated",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			ruleAddErr:         syscall.EEXIST,
			defaultRoutes:      testHostDefaultRoutes(),
			wantGateway:        net.ParseIP("10.224.0.1"),
			expectIpsetAdds:    1,
			expectNotrackRule:  true,
			expectRuleAddCalls: 1,
		},
		{
			name: "RuleList failure surfaces as ip rule error",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			defaultRoutes: testHostDefaultRoutes(),
			ruleListErr:   assert.AnError,
			expectError:   true,
			errorContains: "list ip rules",
		},
		{
			// The gateway comes only from the live host route, so an empty
			// route list must fail the ADD rather than install a gateway-less
			// route that would blackhole pod traffic.
			name: "no default route on the link returns error before creating any rules",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			defaultRoutes: nil,
			expectError:   true,
			errorContains: "no usable IPv4 gateway",
		},
		{
			// A default route whose gateway is 0.0.0.0 is the exact state seen
			// on a node after a CNI restart. It must be rejected, not used.
			name: "default route with unspecified gateway is rejected",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			defaultRoutes: []vishnetlink.Route{
				{LinkIndex: testHostLinkIndex, Gw: net.IPv4zero},
			},
			expectError:   true,
			errorContains: "no usable IPv4 gateway",
		},
		{
			// RouteListFiltered is filtered by output interface, not by
			// destination, so on-link and subnet routes come back too and must
			// be skipped in favour of the 0.0.0.0/0 entry.
			name: "non-default routes on the link are skipped",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			defaultRoutes: []vishnetlink.Route{
				{LinkIndex: testHostLinkIndex, Dst: &net.IPNet{IP: net.ParseIP("10.224.0.0"), Mask: net.CIDRMask(16, 32)}, Gw: net.ParseIP("10.224.0.99")},
				{LinkIndex: testHostLinkIndex, Dst: &net.IPNet{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)}, Gw: net.ParseIP("10.224.0.1")},
			},
			wantGateway:        net.ParseIP("10.224.0.1"),
			expectIpsetAdds:    1,
			expectNotrackRule:  true,
			expectRuleAddCalls: 1,
		},
		{
			name: "default route lookup failure surfaces",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			routeListErr:  assert.AnError,
			expectError:   true,
			errorContains: "host default routes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iptMock := &transparentTunnelMockIPTablesClient{}
			nlMock := &transparentTunnelMockNlClient{
				ruleAddErr:    tt.ruleAddErr,
				existingRules: tt.existingRules,
				ruleListErr:   tt.ruleListErr,
				routeListErr:  tt.routeListErr,
				defaultRoutes: tt.defaultRoutes,
			}
			ipsetMock := &transparentTunnelMockIpsetClient{}

			client := &TransparentTunnelEndpointClient{
				TransparentEndpointClient: &TransparentEndpointClient{
					hostVethName:      testHostVethName,
					hostPrimaryIfName: InfraInterfaceName,
					netioshim:         netio.NewMockNetIO(false, 0),
				},
				iptablesClient: iptMock,
				nlPolicyRoute:  nlMock,
				ipsetClient:    ipsetMock,
			}

			epInfo := &EndpointInfo{IPAddresses: tt.ipAddresses}
			err := client.addTransparentTunnelRules(epInfo)

			// The gateway is resolved from the live host route on every ADD, so
			// the lookup must happen exactly once whatever the outcome.
			assert.Equal(t, 1, nlMock.routeListCalls, "host default route must be read exactly once")
			require.NotNil(t, nlMock.routeListFilter)
			assert.Equal(t, testHostLinkIndex, nlMock.routeListFilter.LinkIndex)
			assert.Equal(t, vishnetlink.RT_FILTER_OIF, nlMock.routeListFilterMask)

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
			assert.Contains(t, notrackCall.match, "-i "+InfraInterfaceName)
			assert.Contains(t, notrackCall.match, "--match-set "+transparentTunnelLocalPodsSet+" src")
			assert.Contains(t, notrackCall.match, "--match-set "+transparentTunnelLocalPodsSet+" dst")

			// MARK rule (mangle table).
			markCall := iptMock.appendCalls[1]
			assert.Equal(t, iptables.V4, markCall.version)
			assert.Equal(t, iptables.Mangle, markCall.tableName)
			assert.Equal(t, iptables.Prerouting, markCall.chainName)
			assert.Contains(t, markCall.match, "-i "+testHostVethName)
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
			require.NotNil(t, tt.wantGateway, "test setup error: expected non-nil gateway in success case")
			assert.True(t, tt.wantGateway.Equal(nlMock.routeReplaceCalls[0].Gw),
				"route Gw mismatch: got %v, want %v", nlMock.routeReplaceCalls[0].Gw, tt.wantGateway)
		})
	}
}

func TestTransparentTunnelAddEndpointRules_IpsetCreateFails(t *testing.T) {
	iptMock := &transparentTunnelMockIPTablesClient{}
	nlMock := &transparentTunnelMockNlClient{defaultRoutes: testHostDefaultRoutes()}
	ipsetMock := &transparentTunnelMockIpsetClient{createErr: assert.AnError}

	client := &TransparentTunnelEndpointClient{
		TransparentEndpointClient: &TransparentEndpointClient{
			hostVethName:      testHostVethName,
			hostPrimaryIfName: InfraInterfaceName,
			netioshim:         netio.NewMockNetIO(false, 0),
		},
		iptablesClient: iptMock,
		nlPolicyRoute:  nlMock,
		ipsetClient:    ipsetMock,
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
	nlMock := &transparentTunnelMockNlClient{defaultRoutes: testHostDefaultRoutes()}
	ipsetMock := &transparentTunnelMockIpsetClient{addErr: assert.AnError}

	client := &TransparentTunnelEndpointClient{
		TransparentEndpointClient: &TransparentEndpointClient{
			hostVethName:      testHostVethName,
			hostPrimaryIfName: InfraInterfaceName,
			netioshim:         netio.NewMockNetIO(false, 0),
		},
		iptablesClient: iptMock,
		nlPolicyRoute:  nlMock,
		ipsetClient:    ipsetMock,
	}

	epInfo := &EndpointInfo{IPAddresses: []net.IPNet{
		{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
	}}
	err := client.addTransparentTunnelRules(epInfo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "10.224.0.46")
	require.Len(t, iptMock.appendCalls, 1)
	assert.Equal(t, iptables.Raw, iptMock.appendCalls[0].tableName)
	assert.Len(t, nlMock.ruleAddCalls, 1)
	assert.Len(t, nlMock.routeReplaceCalls, 1)
}

func TestTransparentTunnelAddEndpointRules_NotrackRuleIsIdempotent(t *testing.T) {
	// Model a node's iptables: RuleExists reports whatever was already appended,
	// so repeated ADDs on the same node must not stack duplicate NOTRACK rules.
	iptMock := &transparentTunnelMockIPTablesClient{}
	iptMock.ruleExistsFn = func(version, tableName, chainName, match, target string) bool {
		for _, call := range iptMock.appendCalls {
			if call.version == version && call.tableName == tableName &&
				call.chainName == chainName && call.match == match && call.target == target {
				return true
			}
		}
		return false
	}

	newClient := func(hostVeth string) *TransparentTunnelEndpointClient {
		return &TransparentTunnelEndpointClient{
			TransparentEndpointClient: &TransparentEndpointClient{
				hostVethName:      hostVeth,
				hostPrimaryIfName: InfraInterfaceName,
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			iptablesClient: iptMock,
			nlPolicyRoute:  &transparentTunnelMockNlClient{defaultRoutes: testHostDefaultRoutes()},
			ipsetClient:    &transparentTunnelMockIpsetClient{},
		}
	}

	// Three pods land on the same node, each with its own host veth.
	pods := []struct {
		hostVeth string
		podIP    string
	}{
		{testHostVethName, "10.224.0.46"},
		{"azveth2", "10.224.0.47"},
		{"azveth3", "10.224.0.48"},
	}

	for _, pod := range pods {
		epInfo := &EndpointInfo{IPAddresses: []net.IPNet{
			{IP: net.ParseIP(pod.podIP), Mask: net.CIDRMask(32, 32)},
		}}
		require.NoError(t, newClient(pod.hostVeth).addTransparentTunnelRules(epInfo))
	}

	var notrackAppends, markAppends []iptablesCall
	for _, call := range iptMock.appendCalls {
		switch call.tableName {
		case iptables.Raw:
			notrackAppends = append(notrackAppends, call)
		case iptables.Mangle:
			markAppends = append(markAppends, call)
		}
	}

	// The NOTRACK rule is node-scoped, so it must be installed exactly once no
	// matter how many pods are added.
	assert.Len(t, notrackAppends, 1, "NOTRACK rule must be appended exactly once per node")
	assert.Equal(t, iptables.Notrack, notrackAppends[0].target)

	// The MARK rule is per-pod, so every pod still gets its own.
	require.Len(t, markAppends, len(pods), "each pod needs its own MARK rule")
	for i, pod := range pods {
		assert.Contains(t, markAppends[i].match, "-i "+pod.hostVeth)
	}
}

func TestTransparentTunnelAddEndpointRules_MarkRuleIsIdempotent(t *testing.T) {
	// Host veth names are derived from the endpoint ID, so a retried ADD for the
	// same pod reuses the same veth name. Deleting the veth link does not remove
	// iptables rules referencing it, so a retry must not stack a duplicate MARK
	// rule in mangle PREROUTING.
	iptMock := &transparentTunnelMockIPTablesClient{}
	iptMock.ruleExistsFn = func(version, tableName, chainName, match, target string) bool {
		for _, call := range iptMock.appendCalls {
			if call.version == version && call.tableName == tableName &&
				call.chainName == chainName && call.match == match && call.target == target {
				return true
			}
		}
		return false
	}

	newClient := func() *TransparentTunnelEndpointClient {
		return &TransparentTunnelEndpointClient{
			TransparentEndpointClient: &TransparentEndpointClient{
				hostVethName:      testHostVethName,
				hostPrimaryIfName: InfraInterfaceName,
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			iptablesClient: iptMock,
			nlPolicyRoute:  &transparentTunnelMockNlClient{defaultRoutes: testHostDefaultRoutes()},
			ipsetClient:    &transparentTunnelMockIpsetClient{},
		}
	}

	epInfo := &EndpointInfo{IPAddresses: []net.IPNet{
		{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
	}}

	// The same pod is added twice, as happens when a CNI ADD is retried.
	require.NoError(t, newClient().addTransparentTunnelRules(epInfo))
	require.NoError(t, newClient().addTransparentTunnelRules(epInfo))

	var markAppends []iptablesCall
	for _, call := range iptMock.appendCalls {
		if call.tableName == iptables.Mangle {
			markAppends = append(markAppends, call)
		}
	}

	require.Len(t, markAppends, 1, "MARK rule must be appended exactly once per pod")
	assert.Equal(t, "-i "+testHostVethName, markAppends[0].match)
}

func TestTransparentTunnelDeleteEndpointRules(t *testing.T) {
	makeClient := func(nlMock *transparentTunnelMockNlClient,
		iptMock *transparentTunnelMockIPTablesClient,
		ipsetMock *transparentTunnelMockIpsetClient,
	) *TransparentTunnelEndpointClient {
		return &TransparentTunnelEndpointClient{
			TransparentEndpointClient: &TransparentEndpointClient{
				hostVethName:      testHostVethName,
				hostPrimaryIfName: InfraInterfaceName,
				netlink:           netlink.NewMockNetlink(false, ""),
				netioshim:         netio.NewMockNetIO(false, 0),
			},
			iptablesClient: iptMock,
			nlPolicyRoute:  nlMock,
			ipsetClient:    ipsetMock,
		}
	}

	makeEndpoint := func() *endpoint {
		return &endpoint{
			HostIfName: testHostVethName,
			IPAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
		}
	}

	t.Run("removes per-pod state and leaves shared setup", func(t *testing.T) {
		nlMock := &transparentTunnelMockNlClient{}
		iptMock := &transparentTunnelMockIPTablesClient{}
		ipsetMock := &transparentTunnelMockIpsetClient{}
		client := makeClient(nlMock, iptMock, ipsetMock)

		require.NoError(t, client.DeleteEndpointRules(makeEndpoint()))

		assert.Equal(t, 1, ipsetMock.countOps(ipsetOpDel))
		assert.Equal(t, "10.224.0.46", ipsetMock.calls[0].arg)

		require.Len(t, iptMock.deleteCalls, 1)
		markCall := iptMock.deleteCalls[0]
		assert.Equal(t, iptables.Mangle, markCall.tableName)
		assert.Equal(t, iptables.Prerouting, markCall.chainName)
		assert.Contains(t, markCall.target, "MARK --set-mark 3")
		assert.Empty(t, nlMock.ruleAddCalls)
		assert.Empty(t, nlMock.routeReplaceCalls)
		assert.Equal(t, 0, ipsetMock.countOps("destroy"), "should not destroy shared ipset")
	})

	t.Run("iptables already-absent rule is treated as success at the call site", func(t *testing.T) {
		nlMock := &transparentTunnelMockNlClient{}
		// The exact stderr iptables emits when -D is given a rule that is not present.
		iptMock := &transparentTunnelMockIPTablesClient{
			deleteErr: errors.New("iptables: Bad rule (does a matching rule exist in that chain?)"),
		}
		ipsetMock := &transparentTunnelMockIpsetClient{}
		client := makeClient(nlMock, iptMock, ipsetMock)

		require.NoError(t, client.DeleteEndpointRules(makeEndpoint()))
		assert.Len(t, iptMock.deleteCalls, 1)
	})

	t.Run("iptables real failure during cleanup is surfaced (xtables lock contention)", func(t *testing.T) {
		nlMock := &transparentTunnelMockNlClient{}
		iptMock := &transparentTunnelMockIPTablesClient{
			deleteErr: errors.New("exit status 4: another app is currently holding the xtables lock; waiting for it to exit"),
		}
		ipsetMock := &transparentTunnelMockIpsetClient{}
		client := makeClient(nlMock, iptMock, ipsetMock)

		err := client.DeleteEndpointRules(makeEndpoint())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MARK")
		assert.Len(t, iptMock.deleteCalls, 1)
	})

	t.Run("ipset del failure is surfaced", func(t *testing.T) {
		nlMock := &transparentTunnelMockNlClient{}
		iptMock := &transparentTunnelMockIPTablesClient{}
		ipsetMock := &transparentTunnelMockIpsetClient{delErr: assert.AnError}
		client := makeClient(nlMock, iptMock, ipsetMock)

		err := client.DeleteEndpointRules(makeEndpoint())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "10.224.0.46")
		assert.Len(t, iptMock.deleteCalls, 1)
	})

	t.Run("iptables delete failure is surfaced", func(t *testing.T) {
		nlMock := &transparentTunnelMockNlClient{}
		iptMock := &transparentTunnelMockIPTablesClient{
			deleteErr: assert.AnError,
		}
		ipsetMock := &transparentTunnelMockIpsetClient{}
		client := makeClient(nlMock, iptMock, ipsetMock)

		err := client.DeleteEndpointRules(makeEndpoint())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MARK")
		assert.Len(t, iptMock.deleteCalls, 1)
	})

	t.Run("tunnel cleanup runs when invoked through the EndpointClient interface", func(t *testing.T) {
		nlMock := &transparentTunnelMockNlClient{}
		iptMock := &transparentTunnelMockIPTablesClient{}
		ipsetMock := &transparentTunnelMockIpsetClient{}
		client := makeClient(nlMock, iptMock, ipsetMock)

		// Assign to the interface so this fails if tunnel cleanup ever again needs a
		// concrete-type assertion at the call site to run.
		var epClient EndpointClient = client
		require.NoError(t, epClient.DeleteEndpointRules(makeEndpoint()))

		assert.Equal(t, 1, ipsetMock.countOps(ipsetOpDel), "tunnel ipset cleanup must run via the interface")
		assert.Len(t, iptMock.deleteCalls, 1, "tunnel MARK rule cleanup must run via the interface")
		assert.Equal(t, 0, ipsetMock.countOps("destroy"), "should not destroy shared ipset")
	})

	// The host veth name is unavailable when the endpoint is rebuilt from a netns
	// whose peer index could not be resolved. Deleting on "-i " would be rejected by
	// iptables, so the ipset entry is still removed and the MARK rule is left alone.
	t.Run("skips the MARK rule when the host veth name is unknown", func(t *testing.T) {
		nlMock := &transparentTunnelMockNlClient{}
		iptMock := &transparentTunnelMockIPTablesClient{}
		ipsetMock := &transparentTunnelMockIpsetClient{}
		client := makeClient(nlMock, iptMock, ipsetMock)

		ep := makeEndpoint()
		ep.HostIfName = ""

		require.NoError(t, client.DeleteEndpointRules(ep))

		assert.Equal(t, 1, ipsetMock.countOps(ipsetOpDel), "pod ip must still be removed from the ipset")
		assert.Empty(t, iptMock.deleteCalls, "must not issue a delete with an empty interface match")
	})
}

// TestResolveTunnelGateway covers the single source of truth for the table-101
// next-hop: the gateway of the host's IPv4 default route on the primary
// interface. There is deliberately no fallback to extIf.IPv4Gateway or
// EndpointInfo.Gateways, since both are cached copies of this same route and can
// be stale or 0.0.0.0.
func TestResolveTunnelGateway(t *testing.T) {
	v4 := func(s string) net.IP { return net.ParseIP(s).To4() }

	defaultRoute := func(gw net.IP) vishnetlink.Route {
		return vishnetlink.Route{
			LinkIndex: testHostLinkIndex,
			Dst:       &net.IPNet{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)},
			Gw:        gw,
		}
	}

	tests := []struct {
		name         string
		routes       []vishnetlink.Route
		routeListErr error
		want         net.IP
		wantErr      error
		errContains  string
	}{
		{
			name:   "default route with a usable gateway is used",
			routes: []vishnetlink.Route{defaultRoute(v4("10.224.0.1"))},
			want:   v4("10.224.0.1"),
		},
		{
			name:   "nil Dst counts as the default route",
			routes: []vishnetlink.Route{{LinkIndex: testHostLinkIndex, Gw: v4("10.3.1.1")}},
			want:   v4("10.3.1.1"),
		},
		{
			name:    "no routes at all returns errNoTunnelGateway",
			routes:  nil,
			wantErr: errNoTunnelGateway,
		},
		{
			// This is the post-CNI-restart state that made the old cached
			// gateway unusable. A zero next-hop must never reach RouteReplace.
			name:    "default route with 0.0.0.0 gateway is rejected",
			routes:  []vishnetlink.Route{defaultRoute(net.IPv4zero)},
			wantErr: errNoTunnelGateway,
		},
		{
			name:    "default route with no gateway at all is rejected",
			routes:  []vishnetlink.Route{defaultRoute(nil)},
			wantErr: errNoTunnelGateway,
		},
		{
			name: "only non-default routes returns errNoTunnelGateway",
			routes: []vishnetlink.Route{
				{
					LinkIndex: testHostLinkIndex,
					Dst:       &net.IPNet{IP: v4("10.224.0.0"), Mask: net.CIDRMask(16, 32)},
					Gw:        v4("10.224.0.99"),
				},
			},
			wantErr: errNoTunnelGateway,
		},
		{
			name: "default route is picked out of a mixed route list",
			routes: []vishnetlink.Route{
				{
					LinkIndex: testHostLinkIndex,
					Dst:       &net.IPNet{IP: v4("10.224.0.0"), Mask: net.CIDRMask(16, 32)},
					Gw:        v4("10.224.0.99"),
				},
				defaultRoute(v4("10.224.0.1")),
			},
			want: v4("10.224.0.1"),
		},
		{
			// A zero-gateway default route must not mask a later usable one.
			name: "first default route unusable falls through to the next",
			routes: []vishnetlink.Route{
				defaultRoute(net.IPv4zero),
				defaultRoute(v4("10.224.0.1")),
			},
			want: v4("10.224.0.1"),
		},
		{
			name:         "netlink failure is surfaced, not swallowed",
			routeListErr: assert.AnError,
			errContains:  "host default routes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nlMock := &transparentTunnelMockNlClient{
				defaultRoutes: tt.routes,
				routeListErr:  tt.routeListErr,
			}
			client := &TransparentTunnelEndpointClient{nlPolicyRoute: nlMock}

			got, err := client.resolveTunnelGateway(testHostLinkIndex)

			// The lookup must be filtered by output interface so routes on other
			// links can never supply the tunnel gateway.
			require.NotNil(t, nlMock.routeListFilter)
			assert.Equal(t, testHostLinkIndex, nlMock.routeListFilter.LinkIndex)
			assert.Equal(t, vishnetlink.RT_FILTER_OIF, nlMock.routeListFilterMask)

			switch {
			case tt.wantErr != nil:
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
			case tt.errContains != "":
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, got)
			default:
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.True(t, tt.want.Equal(got), "got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransparentTunnelDeleteEndpointImplCleansUpOnTunnelFailure(t *testing.T) {
	// A tunnel cleanup failure must not abort the rest of the DEL path, or the
	// veth and base endpoint state leak on every retry.
	iptMock := &transparentTunnelMockIPTablesClient{
		deleteErr: errors.New("xtables lock held"),
	}

	routesDeleted := 0
	nl := netlink.NewMockNetlink(false, "")
	nl.SetDeleteRouteValidationFn(func(_ *netlink.Route) error {
		routesDeleted++
		return nil
	})

	nw := &network{
		Endpoints: map[string]*endpoint{},
		extIf: &externalInterface{
			Name:        InfraInterfaceName,
			IPv4Gateway: net.ParseIP("10.224.0.1"),
		},
	}

	ep := &endpoint{
		Id:         "test-ep",
		HostIfName: testHostVethName,
		IPAddresses: []net.IPNet{
			{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
		},
	}

	// epClient is nil so deleteEndpointImpl builds a real tunnel client around the mocks.
	err := nw.deleteEndpointImpl(nl, platform.NewMockExecClient(false), nil,
		netio.NewMockNetIO(false, 0), NewMockNamespaceClient(), iptMock, &mockDHCP{},
		ep, opModeTransparentTunnel)

	// The failure still surfaces so the runtime retries the DEL.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete transparent tunnel rules")

	// ...but the base cleanup ran anyway rather than being skipped.
	assert.Positive(t, routesDeleted, "base endpoint routes must be deleted despite tunnel failure")
	require.Len(t, iptMock.deleteCalls, 1, "tunnel MARK rule delete must be attempted")
}
