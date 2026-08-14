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
	c.calls = append(c.calls, ipsetCall{op: "del", set: setName, arg: entry})
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
		gateway            net.IP
		epGateways         []net.IP // per-pod IPAM gateways (preferred over extIf gateway)
		ruleAddErr         error    // injected RuleAdd error (nil = success, EEXIST = tolerated)
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
		expectRouteLists   int
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
			gateway:          nil,
			expectError:      true,
			errorContains:    "no usable IPv4 gateway",
			expectRouteLists: 1,
		},
		{
			name: "zero-IP extIf gateway with no pod gateway returns error",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			gateway:          net.IPv4zero,
			expectError:      true,
			errorContains:    "no usable IPv4 gateway",
			expectRouteLists: 1,
		},
		{
			name: "zero-IP gateways use live host default route",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			gateway: net.IPv4zero,
			defaultRoutes: []vishnetlink.Route{
				{LinkIndex: 2, Gw: net.ParseIP("10.3.1.1")},
			},
			wantGateway:        net.ParseIP("10.3.1.1"),
			expectIpsetAdds:    1,
			expectNotrackRule:  true,
			expectRuleAddCalls: 1,
			expectRouteLists:   1,
		},
		{
			name: "default route lookup failure surfaces",
			ipAddresses: []net.IPNet{
				{IP: net.ParseIP("10.224.0.46"), Mask: net.CIDRMask(32, 32)},
			},
			gateway:          net.IPv4zero,
			routeListErr:     assert.AnError,
			expectError:      true,
			errorContains:    "host default routes",
			expectRouteLists: 1,
		},
		{
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
				gateway:        tt.gateway,
			}

			epInfo := &EndpointInfo{IPAddresses: tt.ipAddresses, Gateways: tt.epGateways}
			err := client.addTransparentTunnelRules(epInfo)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
				assert.Equal(t, tt.expectRouteLists, nlMock.routeListCalls)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectRouteLists, nlMock.routeListCalls)
			if tt.expectRouteLists > 0 {
				require.NotNil(t, nlMock.routeListFilter)
				assert.Equal(t, 2, nlMock.routeListFilter.LinkIndex)
				assert.Equal(t, vishnetlink.RT_FILTER_OIF, nlMock.routeListFilterMask)
			}

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
			wantGw := tt.wantGateway
			if wantGw == nil {
				wantGw = getTunnelGateway(&EndpointInfo{Gateways: tt.epGateways}, tt.gateway)
			}
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
			hostVethName:      testHostVethName,
			hostPrimaryIfName: InfraInterfaceName,
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
			hostVethName:      testHostVethName,
			hostPrimaryIfName: InfraInterfaceName,
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
			nlPolicyRoute:  &transparentTunnelMockNlClient{},
			ipsetClient:    &transparentTunnelMockIpsetClient{},
			gateway:        net.ParseIP("10.224.0.1"),
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
			nlPolicyRoute:  &transparentTunnelMockNlClient{},
			ipsetClient:    &transparentTunnelMockIpsetClient{},
			gateway:        net.ParseIP("10.224.0.1"),
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
			gateway:        net.ParseIP("10.224.0.1"),
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

		assert.Equal(t, 1, ipsetMock.countOps("del"))
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

		assert.Equal(t, 1, ipsetMock.countOps("del"), "tunnel ipset cleanup must run via the interface")
		assert.Len(t, iptMock.deleteCalls, 1, "tunnel MARK rule cleanup must run via the interface")
		assert.Equal(t, 0, ipsetMock.countOps("destroy"), "should not destroy shared ipset")
	})
}

func TestGetTunnelGateway(t *testing.T) {
	v4 := func(s string) net.IP { return net.ParseIP(s).To4() }
	v6 := net.ParseIP

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
			got := getTunnelGateway(&EndpointInfo{Gateways: tt.epGateways}, tt.extIfGw)
			if tt.want == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.True(t, got.Equal(tt.want), "got %v, want %v", got, tt.want)
		})
	}

	t.Run("nil epInfo falls back to extIf without panicking", func(t *testing.T) {
		got := getTunnelGateway(nil, v4("10.224.0.1"))
		require.NotNil(t, got)
		assert.True(t, got.Equal(v4("10.224.0.1")))
	})
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
