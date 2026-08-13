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

var errNoTunnelGateway = errors.New("cannot add tunnel rules: no usable IPv4 gateway from epInfo, extIf, or host default route")

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
	gateway        net.IP // Host's IPv4 gateway (for custom route table)
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

	var gw net.IP
	if nw.extIf != nil {
		gw = nw.extIf.IPv4Gateway
	}

	return &TransparentTunnelEndpointClient{
		TransparentEndpointClient: base,
		iptablesClient:            iptc,
		nlPolicyRoute:             defaultTunnelPolicyRouteClient{},
		ipsetClient:               newDefaultTransparentTunnelIpsetClient(plc),
		gateway:                   gw,
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

// DeleteEndpointRules only delegates to the base transparent client. Tunnel
// cleanup is separate because it must return errors to the CNI runtime.
func (client *TransparentTunnelEndpointClient) DeleteEndpointRules(ep *endpoint) {
	client.TransparentEndpointClient.DeleteEndpointRules(ep)
}

// DeleteTransparentTunnelRules removes only per-pod tunnel state. Shared
// node-scoped tunnel setup is left installed and is inert without per-pod state.
func (client *TransparentTunnelEndpointClient) DeleteTransparentTunnelRules(ep *endpoint) error {
	return client.deleteTransparentTunnelRules(ep)
}

// getTunnelGateway returns a non-zero IPv4 gateway for table 101. Prefer the
// per-pod IPAM gateway because persisted extIf gateway can be 0.0.0.0.
func getTunnelGateway(epInfo *EndpointInfo, extIfGateway net.IP) net.IP {
	if epInfo != nil {
		for _, g := range epInfo.Gateways {
			if g4 := g.To4(); g4 != nil && !g4.IsUnspecified() {
				return g4
			}
		}
	}
	if g4 := extIfGateway.To4(); g4 != nil && !g4.IsUnspecified() {
		return g4
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
	gw, err := client.resolveTunnelGateway(epInfo, iface.Index)
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
	// Node-scoped and identical for every pod: it matches on set membership, not
	// on any per-pod interface, so it is appended once and then left alone. Both
	// src and dst must be local pods, which is why per-pod ADD only has to insert
	// the pod IP into the ipset for this rule to start covering that pod.
	// Without the RuleExists check every CNI ADD would stack another duplicate in
	// raw PREROUTING that nothing ever removes.
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
	// Per-pod, and matched on the host veth rather than on the ipset: the mark is
	// what sends the packet to table 101 and out the physical NIC. Matching on
	// "src is a local pod" instead would also match the tunnelled packet when it
	// comes back in on the physical NIC (both ends are local pods), re-marking it
	// and looping it back out. Ingress interface is what distinguishes the two.
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

// resolveTunnelGateway computes the gateway used for the table-101 default
// route, in strict preference order:
//
//  1. epInfo.Gateways    — the per-pod gateway handed down by IPAM/CNS. Preferred
//     because it is always populated on ADD and is correct for the pod's subnet.
//  2. client.gateway     — the external interface's persisted gateway. Used only
//     as a fallback: it can be 0.0.0.0 after a CNI restart, which is why the
//     unspecified check in getTunnelGateway exists.
//  3. host default route — read live from netlink for linkIndex (the physical
//     NIC), as a last resort when neither of the above yields a usable address.
//
// Steps 1 and 2 are the pure-selection half and live in getTunnelGateway so they
// stay unit-testable without netlink. Only step 3 touches the host. Returns
// errNoTunnelGateway if all three come up empty, since a tunnel route without a
// gateway would silently blackhole pod traffic.
func (client *TransparentTunnelEndpointClient) resolveTunnelGateway(epInfo *EndpointInfo, linkIndex int) (net.IP, error) {
	if gw := getTunnelGateway(epInfo, client.gateway); gw != nil {
		return gw, nil
	}

	gw, err := client.hostDefaultGateway(linkIndex)
	if err != nil {
		return nil, err
	}
	if gw == nil {
		return nil, errNoTunnelGateway
	}
	return gw, nil
}

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

func buildTransparentTunnelNotrackMatch(hostPrimaryIf string) string {
	return "-i " + hostPrimaryIf +
		" -m set --match-set " + transparentTunnelLocalPodsSet + " src" +
		" -m set --match-set " + transparentTunnelLocalPodsSet + " dst"
}

func findFwmarkRule(rules []vishnetlink.Rule, fwmark uint32, table int) *vishnetlink.Rule {
	for i := range rules {
		if rules[i].Mark == fwmark && rules[i].Table == table {
			return &rules[i]
		}
	}
	return nil
}

// deleteTransparentTunnelRules undoes, on pod DEL, exactly what addTransparentTunnelRules
// created per pod:
//
//   - removes each of the pod's IPv4 addresses from the azure-tt-local-pods ipset
//   - removes that pod's "-i <hostVeth> -j MARK --set-mark 3" rule from mangle PREROUTING
//
// It deliberately leaves all node-scoped state in place (the ipset itself, the raw
// PREROUTING NOTRACK rule, the fwmark ip rule and the table-101 route). That state is
// shared by every pod on the node, so tearing it down here would break pods that are
// still running, and racing ADDs would lose shared setup. It is also inert once the set
// is empty: NOTRACK only matches when src and dst are both in the set.
//
// Errors are collected rather than returned early so that a failure removing one pod IP
// still lets the MARK rule be cleaned up; the joined error is returned to the caller.
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
