package network

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
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

const (
	// transparentTunnelFwmark is the fwmark value used to re-route pod traffic through VFP.
	// Packets marked with this value are looked up in transparentTunnelRouteTable instead of
	// the main routing table, forcing them out via the host's physical interface where
	// VFP can enforce NSG rules on same-node pod-to-pod traffic.
	transparentTunnelFwmark = 3

	// transparentTunnelRouteTable is the custom routing table used for fwmark-marked packets.
	transparentTunnelRouteTable = 101
)

// tunnelPolicyRouteClient abstracts vishvananda/netlink operations for policy
// routing (ip rule) and route table management so that unit tests avoid
// touching real netlink sockets.
type tunnelPolicyRouteClient interface {
	RuleAdd(rule *vishnetlink.Rule) error
	RuleDel(rule *vishnetlink.Rule) error
	RouteReplace(route *vishnetlink.Route) error
	RouteDel(route *vishnetlink.Route) error
}

// defaultTunnelPolicyRouteClient delegates to the real vishvananda/netlink package.
type defaultTunnelPolicyRouteClient struct{}

func (defaultTunnelPolicyRouteClient) RuleAdd(rule *vishnetlink.Rule) error {
	if err := vishnetlink.RuleAdd(rule); err != nil {
		return fmt.Errorf("netlink rule add: %w", err)
	}
	return nil
}

func (defaultTunnelPolicyRouteClient) RuleDel(rule *vishnetlink.Rule) error {
	if err := vishnetlink.RuleDel(rule); err != nil {
		return fmt.Errorf("netlink rule del: %w", err)
	}
	return nil
}

func (defaultTunnelPolicyRouteClient) RouteReplace(route *vishnetlink.Route) error {
	if err := vishnetlink.RouteReplace(route); err != nil {
		return fmt.Errorf("netlink route replace: %w", err)
	}
	return nil
}

func (defaultTunnelPolicyRouteClient) RouteDel(route *vishnetlink.Route) error {
	if err := vishnetlink.RouteDel(route); err != nil {
		return fmt.Errorf("netlink route del: %w", err)
	}
	return nil
}

// TransparentTunnelEndpointClient extends TransparentEndpointClient with
// iptables and ip-rule based tunneling that forces same-node pod-to-pod
// traffic through the host's physical interface (and therefore through VFP)
// so that Azure NSG rules are enforced even for intra-node communication.
type TransparentTunnelEndpointClient struct {
	*TransparentEndpointClient
	iptablesClient ipTablesClient
	nlPolicyRoute  tunnelPolicyRouteClient
	serviceCIDRs   []string // Cluster service CIDRs to exempt from fwmark
	gateway        net.IP   // Host's IPv4 gateway (for custom route table)
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

	var serviceCIDRs []string
	if epInfo.ServiceCidrs != "" {
		serviceCIDRs = strings.Split(epInfo.ServiceCidrs, ",")
	}

	var gw net.IP
	if nw.extIf != nil {
		gw = nw.extIf.IPv4Gateway
	}

	return &TransparentTunnelEndpointClient{
		TransparentEndpointClient: base,
		iptablesClient:            iptc,
		nlPolicyRoute:             defaultTunnelPolicyRouteClient{},
		serviceCIDRs:              serviceCIDRs,
		gateway:                   gw,
	}
}

// AddEndpointRules sets up the base transparent rules (host route + ARP proxy)
// and then adds transparent-tunnel-specific iptables and ip-rule entries that tunnel pod traffic
// through VFP.
func (client *TransparentTunnelEndpointClient) AddEndpointRules(epInfo *EndpointInfo) error {
	if err := client.TransparentEndpointClient.AddEndpointRules(epInfo); err != nil {
		return err
	}

	if err := client.addTransparentTunnelRules(); err != nil {
		return errors.Wrap(err, "failed to add tunnel rules")
	}

	return nil
}

// DeleteEndpointRules removes transparent-tunnel-specific rules, then delegates to the base
// transparent client for standard cleanup.
func (client *TransparentTunnelEndpointClient) DeleteEndpointRules(ep *endpoint) {
	client.deleteTransparentTunnelRules(ep)
	client.TransparentEndpointClient.DeleteEndpointRules(ep)
}

// addTransparentTunnelRules installs per-endpoint iptables and routing-policy rules:
//
//  1. Service CIDR RETURN — exempts ClusterIP-destined traffic from fwmark to
//     prevent conntrack tuple collisions. Without this, same-node DNAT'd UDP
//     traffic (e.g. DNS to CoreDNS) suffers ~50% packet loss because the fwmark
//     causes a re-entry through the pod's veth that creates a second conntrack
//     entry whose reply tuple collides with the original DNAT entry.
//
//  2. MARK rule — sets fwmark on all remaining ingress traffic from the pod's
//     host-side veth, causing it to be looked up in the custom routing table.
//
//  3. IP rule + route — marked packets are routed via the host's physical
//     interface, which forces them through VFP for NSG enforcement.
//
// In NodeSubnet mode, pods and the node share the same VNet subnet (e.g.
// 10.224.0.0/16) — there is no distinct pod CIDR. An ip-rule "from" match
// would also capture node-originated traffic (kubelet, API-server health
// probes), which must NOT be re-routed through VFP. The fwmark approach
// uses interface-based matching (-i <vethName>) in iptables to identify
// only pod-originated traffic, then stamps it with a mark that the ip rule
// selects on. This is the only reliable way to distinguish pod vs node
// traffic when they share the same subnet.
//
// The MARK target is a packet-alteration operation and is only valid in the
// mangle (and raw) tables — filter can only ACCEPT/DROP/REJECT, and nat is
// for address translation. The mangle PREROUTING chain runs before the
// kernel routing decision (chain order: raw → mangle → nat → filter), so
// the fwmark is set before the kernel consults the routing table — exactly
// what we need for policy routing via table 101.
// Ref: iptables(8) man page, Netfilter Packet Traversal documentation.
func (client *TransparentTunnelEndpointClient) addTransparentTunnelRules() error {
	// Gateway is required — without it the custom routing table would have no default
	// route, and all fwmarked packets would be black-holed. Fail early before creating
	// any iptables rules to avoid leaving the node in a partially-configured state.
	if client.gateway == nil {
		return errors.New("cannot add tunnel rules: host gateway is nil")
	}

	hostVeth := client.hostVethName
	markStr := strconv.Itoa(transparentTunnelFwmark)

	// 1. Service CIDR RETURN rules (must be inserted BEFORE the MARK rule).
	for _, cidr := range client.serviceCIDRs {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		match := "-i " + hostVeth + " -d " + cidr
		if err := client.iptablesClient.InsertIptableRule(
			iptables.V4, iptables.Mangle, iptables.Prerouting, match, "RETURN",
		); err != nil {
			return errors.Wrapf(err, "failed to insert service CIDR RETURN rule for %s", cidr)
		}
		logger.Info("transparent-tunnel: added service CIDR RETURN rule",
			zap.String("veth", hostVeth), zap.String("cidr", cidr))
	}

	// 2. Fwmark MARK rule — append so it comes after RETURN rules.
	markMatch := "-i " + hostVeth
	markTarget := "MARK --set-mark " + markStr
	if err := client.iptablesClient.AppendIptableRule(
		iptables.V4, iptables.Mangle, iptables.Prerouting, markMatch, markTarget,
	); err != nil {
		return errors.Wrap(err, "failed to append fwmark MARK rule")
	}
	logger.Info("transparent-tunnel: added fwmark MARK rule",
		zap.String("veth", hostVeth), zap.String("mark", markStr))

	// 3. IP rule: fwmark → custom routing table (via netlink).
	// The ip rule is shared across all transparent-tunnel endpoints on this node —
	// every pod uses the same fwmark (3) and lookup table (101). We always attempt
	// the add and tolerate EEXIST. This avoids a TOCTOU race where two concurrent
	// pod creates both see the rule missing and both try to add it.
	rule := vishnetlink.NewRule()
	rule.Mark = transparentTunnelFwmark
	rule.Table = transparentTunnelRouteTable
	rule.Family = unix.AF_INET
	if err := client.nlPolicyRoute.RuleAdd(rule); err != nil {
		if !errors.Is(err, syscall.EEXIST) {
			return errors.Wrap(err, "failed to add ip rule for fwmark")
		}
		logger.Info("transparent-tunnel: ip rule already exists, skipping",
			zap.Int("fwmark", transparentTunnelFwmark), zap.Int("table", transparentTunnelRouteTable))
	} else {
		logger.Info("transparent-tunnel: added ip rule",
			zap.Int("fwmark", transparentTunnelFwmark), zap.Int("table", transparentTunnelRouteTable))
	}

	// 4. Default route in custom table via physical interface → VFP (via netlink).
	// RouteReplace is idempotent, so safe to call from every endpoint.
	iface, err := client.netioshim.GetNetworkInterfaceByName(client.hostPrimaryIfName)
	if err != nil {
		return errors.Wrapf(err, "failed to look up interface %s for tunnel route", client.hostPrimaryIfName)
	}
	_, defaultDst, _ := net.ParseCIDR("0.0.0.0/0")
	route := &vishnetlink.Route{
		LinkIndex: iface.Index,
		Dst:       defaultDst,
		Gw:        client.gateway,
		Table:     transparentTunnelRouteTable,
	}
	if err := client.nlPolicyRoute.RouteReplace(route); err != nil {
		return errors.Wrapf(err, "failed to add default route in table %d", transparentTunnelRouteTable)
	}
	logger.Info("transparent-tunnel: added default route in custom table",
		zap.String("gw", client.gateway.String()),
		zap.String("dev", client.hostPrimaryIfName),
		zap.Int("table", transparentTunnelRouteTable))

	return nil
}

// deleteTransparentTunnelRules removes the per-endpoint iptables and routing-policy rules.
func (client *TransparentTunnelEndpointClient) deleteTransparentTunnelRules(ep *endpoint) {
	hostVeth := ep.HostIfName
	markStr := strconv.Itoa(transparentTunnelFwmark)

	// Remove service CIDR RETURN rules.
	for _, cidr := range client.serviceCIDRs {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		match := "-i " + hostVeth + " -d " + cidr
		if err := client.iptablesClient.DeleteIptableRule(
			iptables.V4, iptables.Mangle, iptables.Prerouting, match, "RETURN",
		); err != nil {
			logger.Error("transparent-tunnel: failed to delete service CIDR RETURN rule",
				zap.String("cidr", cidr), zap.Error(err))
		}
	}

	// Remove fwmark MARK rule.
	markMatch := "-i " + hostVeth
	markTarget := "MARK --set-mark " + markStr
	if err := client.iptablesClient.DeleteIptableRule(
		iptables.V4, iptables.Mangle, iptables.Prerouting, markMatch, markTarget,
	); err != nil {
		logger.Error("transparent-tunnel: failed to delete fwmark MARK rule", zap.Error(err))
	}

	// Best-effort removal of the shared ip rule and route. The ip rule and route
	// table are shared by all transparent-tunnel endpoints on this node — every pod
	// uses the same fwmark (3) and lookup table (101). We only remove them if no
	// other endpoint's fwmark MARK rules remain in mangle PREROUTING.
	// This prevents deleting one pod from breaking routing for all other pods.
	// If checking fails, we skip removal (safe — a stale rule is harmless without
	// any MARK rules to trigger it, and will be cleaned up with the last pod).
	//
	// Note: `iptables -S` normalizes `--set-mark N` to `--set-xmark 0xN/0xffffffff`,
	// so we count the normalized form.
	hexMark := fmt.Sprintf("0x%x", transparentTunnelFwmark)
	out, _ := client.plClient.ExecuteCommand(context.TODO(), "iptables", "-t", "mangle", "-S", "PREROUTING")
	markCount := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "--set-xmark "+hexMark+"/") {
			markCount++
		}
	}

	if markCount == 0 {
		rule := vishnetlink.NewRule()
		rule.Mark = transparentTunnelFwmark
		rule.Table = transparentTunnelRouteTable
		rule.Family = unix.AF_INET
		if err := client.nlPolicyRoute.RuleDel(rule); err != nil && !errors.Is(err, syscall.ENOENT) && !errors.Is(err, syscall.ESRCH) {
			logger.Error("transparent-tunnel: failed to delete ip rule",
				zap.Int("fwmark", transparentTunnelFwmark), zap.Error(err))
		}

		_, defaultDst, _ := net.ParseCIDR("0.0.0.0/0")
		route := &vishnetlink.Route{
			Dst:   defaultDst,
			Table: transparentTunnelRouteTable,
		}
		if err := client.nlPolicyRoute.RouteDel(route); err != nil && !errors.Is(err, syscall.ESRCH) {
			logger.Error("transparent-tunnel: failed to delete route in table",
				zap.Int("table", transparentTunnelRouteTable), zap.Error(err))
		}
	}
}
