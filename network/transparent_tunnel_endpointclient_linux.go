package network

import (
	stderrors "errors"
	"fmt"
	"net"
	"strconv"
	"syscall"

	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/pkg/errors"
	vishnetlink "github.com/vishvananda/netlink"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

// NOTE: transparent-tunnel mode is IPv4-only. All tunnel state (ipset, NOTRACK,
// fwmark ip rule, and the table-101 route) is provisioned for IPv4 exclusively,
// and IPv6 pod addresses are intentionally skipped. IPv6 support is out of scope
// for this mode and would require a separate v6 dataplane.
const (
	transparentTunnelFwmark = 3

	transparentTunnelRouteTable = 101

	transparentTunnelLocalPodsSet = "azure-tt-local-pods"

	transparentTunnelLocalPodsSetType = "hash:ip"
)

var errNoTunnelGateway = errors.New("cannot add tunnel rules: no usable IPv4 gateway on the host default route")

// tunnelPolicyRouteClient abstracts vishvananda/netlink operations for policy
// routing so unit tests avoid touching real netlink sockets.
type tunnelPolicyRouteClient interface {
	RuleAdd(rule *vishnetlink.Rule) error
	RuleList(family int) ([]vishnetlink.Rule, error)
	RouteListFiltered(family int, filter *vishnetlink.Route, filterMask uint64) ([]vishnetlink.Route, error)
	RouteReplace(route *vishnetlink.Route) error
}

// defaultTunnelPolicyRouteClient delegates to the real vishvananda/netlink package.
type defaultTunnelPolicyRouteClient struct{}

func (defaultTunnelPolicyRouteClient) RuleAdd(rule *vishnetlink.Rule) error {
	if err := vishnetlink.RuleAdd(rule); err != nil {
		return fmt.Errorf("netlink rule add: %w", err)
	}
	return nil
}

func (defaultTunnelPolicyRouteClient) RuleList(family int) ([]vishnetlink.Rule, error) {
	rules, err := vishnetlink.RuleList(family)
	if err != nil {
		return nil, fmt.Errorf("netlink rule list: %w", err)
	}
	return rules, nil
}

func (defaultTunnelPolicyRouteClient) RouteListFiltered(family int, filter *vishnetlink.Route, filterMask uint64) ([]vishnetlink.Route, error) {
	routes, err := vishnetlink.RouteListFiltered(family, filter, filterMask)
	if err != nil {
		return nil, fmt.Errorf("netlink route list filtered: %w", err)
	}
	return routes, nil
}

func (defaultTunnelPolicyRouteClient) RouteReplace(route *vishnetlink.Route) error {
	if err := vishnetlink.RouteReplace(route); err != nil {
		return fmt.Errorf("netlink route replace: %w", err)
	}
	return nil
}

// TransparentTunnelEndpointClient extends TransparentEndpointClient with
// tunnel rules that send same-node pod traffic through VFP.
type TransparentTunnelEndpointClient struct {
	*TransparentEndpointClient
	iptablesClient ipTablesClient
	nlPolicyRoute  tunnelPolicyRouteClient
	ipsetClient    transparentTunnelIpsetClient
}

func NewTransparentTunnelEndpointClient(
	nw *network,
	epInfo *EndpointInfo,
	hostVethName string,
	containerVethName string,
	nl netlink.NetlinkInterface,
	nioc netio.NetIOInterface,
	plc platform.ExecClient,
	iptc ipTablesClient,
) *TransparentTunnelEndpointClient {
	base := NewTransparentEndpointClient(nw.extIf, hostVethName, containerVethName, epInfo.Mode, nl, nioc, plc)

	return &TransparentTunnelEndpointClient{
		TransparentEndpointClient: base,
		iptablesClient:            iptc,
		nlPolicyRoute:             defaultTunnelPolicyRouteClient{},
		ipsetClient:               newDefaultTransparentTunnelIpsetClient(plc),
	}
}

// AddEndpointRules adds base transparent endpoint rules and transparent-tunnel rules.
func (client *TransparentTunnelEndpointClient) AddEndpointRules(epInfo *EndpointInfo) error {
	if err := client.TransparentEndpointClient.AddEndpointRules(epInfo); err != nil {
		return err
	}

	if err := client.addTransparentTunnelRules(epInfo); err != nil {
		return errors.Wrap(err, "failed to add tunnel rules")
	}

	return nil
}

// DeleteEndpointRules removes per-pod tunnel state, then the base transparent rules.
// Shared node-scoped tunnel setup is left installed and is inert without per-pod state.
// Tunnel errors are returned after the base cleanup runs so a tunnel failure does not
// leak the base endpoint rules.
func (client *TransparentTunnelEndpointClient) DeleteEndpointRules(ep *endpoint) error {
	tunnelErr := client.deleteTransparentTunnelRules(ep)
	if tunnelErr != nil {
		logger.Error("Failed to delete transparent tunnel rules, continuing cleanup", zap.Error(tunnelErr))
	}

	if err := client.TransparentEndpointClient.DeleteEndpointRules(ep); err != nil {
		return err
	}

	if tunnelErr != nil {
		return fmt.Errorf("failed to delete transparent tunnel rules: %w", tunnelErr)
	}
	return nil
}

func (client *TransparentTunnelEndpointClient) addTransparentTunnelRules(epInfo *EndpointInfo) error {
	hostVeth := client.hostVethName
	markStr := strconv.Itoa(transparentTunnelFwmark)
	iface, err := client.netioshim.GetNetworkInterfaceByName(client.hostPrimaryIfName)
	if err != nil {
		return errors.Wrapf(err, "failed to look up interface %s for tunnel route", client.hostPrimaryIfName)
	}
	gw, err := client.resolveTunnelGateway(iface.Index)
	if err != nil {
		return err
	}

	if err := client.ipsetClient.Create(transparentTunnelLocalPodsSet, transparentTunnelLocalPodsSetType); err != nil {
		return errors.Wrap(err, "failed to create local-pods ipset")
	}

	notrackMatch := buildTransparentTunnelNotrackMatch(client.hostPrimaryIfName)
	// iptables -t raw -A PREROUTING -i <hostPrimaryIf> \
	//     -m set --match-set azure-tt-local-pods src \
	//     -m set --match-set azure-tt-local-pods dst -j NOTRACK
	//
	// Node-scoped and identical for every pod, so append it only when absent.
	// Without this check every CNI ADD would stack another duplicate in raw
	// PREROUTING that nothing ever removes.
	if client.iptablesClient.RuleExists(iptables.V4, iptables.Raw, iptables.Prerouting, notrackMatch, iptables.Notrack) {
		logger.Info("transparent-tunnel: NOTRACK rule already present, skipping append",
			zap.String("dev", client.hostPrimaryIfName))
	} else if err := client.iptablesClient.AppendIptableRule(iptables.V4, iptables.Raw, iptables.Prerouting, notrackMatch, iptables.Notrack); err != nil {
		return errors.Wrap(err, "failed to append NOTRACK rule")
	}

	if err := client.ensureFwmarkRule(); err != nil {
		return err
	}

	_, defaultDst, _ := net.ParseCIDR("0.0.0.0/0")
	route := &vishnetlink.Route{
		LinkIndex: iface.Index,
		Dst:       defaultDst,
		Gw:        gw,
		Table:     transparentTunnelRouteTable,
	}
	if err := client.nlPolicyRoute.RouteReplace(route); err != nil {
		return errors.Wrapf(err, "failed to add default route in table %d", transparentTunnelRouteTable)
	}
	logger.Info("transparent-tunnel: ensured shared routing state",
		zap.String("gw", gw.String()),
		zap.String("dev", client.hostPrimaryIfName),
		zap.Int("table", transparentTunnelRouteTable))

	for _, ipAddr := range epInfo.IPAddresses {
		if ipAddr.IP.To4() == nil {
			continue
		}
		entry := ipAddr.IP.String()
		if err := client.ipsetClient.Add(transparentTunnelLocalPodsSet, entry); err != nil {
			return errors.Wrapf(err, "failed to add %s to local-pods ipset", entry)
		}
		logger.Info("transparent-tunnel: added pod IP to local-pods ipset",
			zap.String("set", transparentTunnelLocalPodsSet), zap.String("ip", entry))
	}

	markMatch := "-i " + hostVeth
	markTarget := "MARK --set-mark " + markStr
	// iptables -t mangle -A PREROUTING -i <hostVeth> -j MARK --set-mark 3
	//
	// Matched on the ingress veth, not on an ipset src match: the latter would also
	// match the tunnelled packet coming back in on the physical NIC (both ends are
	// local pods) and loop it back out.
	//
	// Host veth names are derived from the endpoint ID, so a retried ADD for the
	// same pod reuses the same name and match string. Deleting the veth link does
	// not remove iptables rules referencing it, so append only when absent to
	// avoid stacking duplicates in mangle PREROUTING.
	if client.iptablesClient.RuleExists(iptables.V4, iptables.Mangle, iptables.Prerouting, markMatch, markTarget) {
		logger.Info("transparent-tunnel: fwmark MARK rule already present, skipping append",
			zap.String("veth", hostVeth), zap.String("mark", markStr))
		return nil
	}
	if err := client.iptablesClient.AppendIptableRule(iptables.V4, iptables.Mangle, iptables.Prerouting, markMatch, markTarget); err != nil {
		return errors.Wrap(err, "failed to append fwmark MARK rule")
	}
	logger.Info("transparent-tunnel: added fwmark MARK rule",
		zap.String("veth", hostVeth), zap.String("mark", markStr))

	return nil
}

// resolveTunnelGateway returns the next-hop for the table-101 default route: the
// gateway of the host's IPv4 default route on linkIndex, read live from netlink.
//
// This is the single source of truth on purpose. The other candidates that were
// once consulted here are not independent values, they are staler copies of this
// same route: extIf.IPv4Gateway is cached from it by saveIPConfig at network
// create time, and EndpointInfo.Gateways is in turn populated from extIf. A cached
// copy can be 0.0.0.0 (for example after a CNI restart), which would install a
// gateway-less route and blackhole pod traffic, so the live route is read instead.
// Returns errNoTunnelGateway when the host has no usable default gateway.
func (client *TransparentTunnelEndpointClient) resolveTunnelGateway(linkIndex int) (net.IP, error) {
	gw, err := client.hostDefaultGateway(linkIndex)
	if err != nil {
		return nil, err
	}
	if gw == nil {
		return nil, errNoTunnelGateway
	}
	return gw, nil
}

// hostDefaultGateway returns the gateway of the IPv4 default route on linkIndex,
// or nil when the link has no default route with a usable (non-zero) gateway.
func (client *TransparentTunnelEndpointClient) hostDefaultGateway(linkIndex int) (net.IP, error) {
	routes, err := client.nlPolicyRoute.RouteListFiltered(unix.AF_INET, &vishnetlink.Route{LinkIndex: linkIndex}, vishnetlink.RT_FILTER_OIF)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list host default routes")
	}
	for i := range routes {
		if !isDefaultIPv4Route(routes[i]) {
			continue
		}
		if gw := routes[i].Gw.To4(); gw != nil && !gw.IsUnspecified() {
			return gw, nil
		}
	}
	return nil, nil
}

func isDefaultIPv4Route(route vishnetlink.Route) bool {
	if route.Dst == nil {
		return true
	}
	ones, bits := route.Dst.Mask.Size()
	return ones == 0 && bits == ipv4Bits && route.Dst.IP.To4() != nil && route.Dst.IP.IsUnspecified()
}

// ensureFwmarkRule installs the policy-routing rule that diverts marked packets
// into the tunnel routing table, equivalent to:
//
//	ip rule add fwmark 3 lookup 101
//
// This is the second half of the fwmark mechanism: the mangle PREROUTING MARK
// rule stamps mark 3 on packets arriving from a local pod's veth, and this rule
// makes the kernel consult table 101 for them, whose default route sends them
// out of the physical NIC to the gateway so VFP sees the traffic.
//
// The rule is node-scoped and identical for every pod, so it is added only when
// an equivalent one is not already present. Without the dedup, each CNI ADD
// would stack another copy that nothing removes. EEXIST is tolerated for the
// race where a concurrent ADD installs the rule between the list and the add.
func (client *TransparentTunnelEndpointClient) ensureFwmarkRule() error {
	rule := vishnetlink.NewRule()
	rule.Mark = transparentTunnelFwmark
	rule.Table = transparentTunnelRouteTable
	rule.Family = unix.AF_INET

	existingRules, err := client.nlPolicyRoute.RuleList(unix.AF_INET)
	if err != nil {
		return errors.Wrap(err, "failed to list ip rules for fwmark dedup")
	}
	if existing := findFwmarkRule(existingRules, transparentTunnelFwmark, transparentTunnelRouteTable); existing != nil {
		logger.Info("transparent-tunnel: ip rule already present, skipping add",
			zap.Int("fwmark", transparentTunnelFwmark),
			zap.Int("table", transparentTunnelRouteTable),
			zap.Int("priority", existing.Priority))
		return nil
	}
	if err := client.nlPolicyRoute.RuleAdd(rule); err != nil && !errors.Is(err, syscall.EEXIST) {
		return errors.Wrap(err, "failed to add ip rule for fwmark")
	}
	logger.Info("transparent-tunnel: ensured ip rule",
		zap.Int("fwmark", transparentTunnelFwmark), zap.Int("table", transparentTunnelRouteTable))
	return nil
}

// buildTransparentTunnelNotrackMatch builds the match half of the raw PREROUTING
// NOTRACK rule: ingress on the host primary interface, with both source and
// destination in the local-pods ipset. Shared by rule add, exists-check and delete
// so all three agree on the exact match string.
func buildTransparentTunnelNotrackMatch(hostPrimaryIf string) string {
	return "-i " + hostPrimaryIf +
		" -m set --match-set " + transparentTunnelLocalPodsSet + " src" +
		" -m set --match-set " + transparentTunnelLocalPodsSet + " dst"
}

// findFwmarkRule returns the existing ip rule matching both fwmark and table, or nil
// if none is installed. Used to make the fwmark rule setup idempotent across ADDs.
func findFwmarkRule(rules []vishnetlink.Rule, fwmark uint32, table int) *vishnetlink.Rule {
	for i := range rules {
		if rules[i].Mark == fwmark && rules[i].Table == table {
			return &rules[i]
		}
	}
	return nil
}

// deleteTransparentTunnelRules removes this pod's ipset entries and its MARK rule.
// Shared node-scoped state (the ipset, NOTRACK rule, fwmark ip rule and table-101
// route) is deliberately kept so concurrent ADDs do not lose shared setup; it is
// inert once the set is empty. Errors are collected, not returned early, so one
// failure does not skip the remaining cleanup.
func (client *TransparentTunnelEndpointClient) deleteTransparentTunnelRules(ep *endpoint) error {
	hostVeth := ep.HostIfName
	markStr := strconv.Itoa(transparentTunnelFwmark)

	var errs []error

	for _, ipAddr := range ep.IPAddresses {
		if ipAddr.IP.To4() == nil {
			continue
		}
		entry := ipAddr.IP.String()
		if err := client.ipsetClient.Del(transparentTunnelLocalPodsSet, entry); err != nil {
			logger.Error("transparent-tunnel: failed to remove pod IP from local-pods ipset",
				zap.String("ip", entry), zap.Error(err))
			errs = append(errs, errors.Wrapf(err, "remove %s from local-pods ipset", entry))
		} else {
			logger.Info("transparent-tunnel: removed pod IP from local-pods ipset",
				zap.String("set", transparentTunnelLocalPodsSet), zap.String("ip", entry))
		}
	}

	markMatch := "-i " + hostVeth
	markTarget := "MARK --set-mark " + markStr
	// Without a veth name the match would be "-i " and iptables would reject it, so
	// the rule is left in place rather than issuing a malformed delete.
	if hostVeth == "" {
		logger.Warn("transparent-tunnel: no host veth name, skipping fwmark MARK rule delete",
			zap.String("endpointID", ep.Id))
		return stderrors.Join(errs...)
	}
	// A missing rule is success here: DEL can run after a partial ADD, or be
	// retried after a DEL that already removed it. Every other iptables failure
	// (permissions, xtables lock contention) is collected and returned.
	if err := client.iptablesClient.DeleteIptableRule(iptables.V4, iptables.Mangle, iptables.Prerouting, markMatch, markTarget); err != nil {
		if iptables.IsRuleNotFoundErr(err) {
			logger.Info("transparent-tunnel: fwmark MARK rule already absent, treating as success",
				zap.String("veth", hostVeth))
		} else {
			logger.Error("transparent-tunnel: failed to delete fwmark MARK rule", zap.Error(err))
			errs = append(errs, errors.Wrap(err, "delete fwmark MARK rule"))
		}
	}

	return stderrors.Join(errs...)
}
