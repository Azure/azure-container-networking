package network

import (
	"context"
	stderrors "errors"
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

	// transparentTunnelLocalPodsSet is the ipset (hash:ip) holding the IPv4
	// addresses of every transparent-tunnel pod running on this node. It is
	// matched bidirectionally (src AND dst) in the raw-table NOTRACK rule
	// below so that only same-node VFP-hairpinned pod-to-pod packets bypass
	// conntrack, leaving cross-node traffic and node-originated traffic
	// fully tracked (un-DNAT, NPM ESTABLISHED, etc. continue to work).
	transparentTunnelLocalPodsSet = "azure-tt-local-pods"

	// transparentTunnelLocalPodsSetType is the ipset type used for the
	// local-pods set. hash:ip stores individual IPv4 addresses (no CIDRs).
	transparentTunnelLocalPodsSetType = "hash:ip"
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

// AddEndpointRules sets up the base transparent rules (host route + ARP proxy)
// and then adds transparent-tunnel-specific iptables and ip-rule entries that tunnel pod traffic
// through VFP.
func (client *TransparentTunnelEndpointClient) AddEndpointRules(epInfo *EndpointInfo) error {
	if err := client.TransparentEndpointClient.AddEndpointRules(epInfo); err != nil {
		return err
	}

	if err := client.addTransparentTunnelRules(epInfo); err != nil {
		return errors.Wrap(err, "failed to add tunnel rules")
	}

	return nil
}

// DeleteEndpointRules satisfies the EndpointClient interface (void). It only
// delegates to the base transparent client — transparent-tunnel-specific
// cleanup is NOT done here because the EndpointClient interface cannot
// return an error, and unlike other modes, leftover transparent-tunnel state
// (iptables MARK rules / fwmark ip rule / custom route table / ipset entry)
// actively breaks subsequent pods on the node.
//
// Callers in endpoint_linux.go must call DeleteTransparentTunnelRules
// (declared below) BEFORE invoking DeleteEndpointRules so that delete
// failures are surfaced to containerd, which will then retry the CNI DEL.
func (client *TransparentTunnelEndpointClient) DeleteEndpointRules(ep *endpoint) {
	client.TransparentEndpointClient.DeleteEndpointRules(ep)
}

// DeleteTransparentTunnelRules removes the per-endpoint iptables, ipset, and
// routing-policy rules installed by AddEndpointRules, and (if this is the
// last transparent-tunnel endpoint on the node) the shared ip rule, route,
// raw-table NOTRACK rule, and ipset. All cleanup steps are attempted
// regardless of intermediate failures so the node is left in the
// most-clean state possible; any errors encountered are aggregated and
// returned so the CNI runtime can retry.
func (client *TransparentTunnelEndpointClient) DeleteTransparentTunnelRules(ep *endpoint) error {
	return client.deleteTransparentTunnelRules(ep)
}

// addTransparentTunnelRules installs per-endpoint and shared rules required
// to force same-node pod-to-pod traffic through VFP for NSG enforcement.
//
// Per-pod (added on every endpoint create):
//
//  1. ipset entry — every IPv4 pod IP is added to the shared local-pods set.
//     Equivalent CLI:
//     ipset add azure-tt-local-pods <podIPv4> -exist
//
//  2. mangle MARK rule — stamps fwmark 0x3 on traffic from the pod's
//     host-side veth so the kernel routes it via the custom table 101 (and
//     therefore out the physical interface where VFP runs).
//     Equivalent CLI:
//     iptables -t mangle -A PREROUTING -i <hostVeth> -j MARK --set-mark 3
//
// Shared (added once on first endpoint, idempotent on later endpoints):
//
//  3. ipset definition — created on demand if missing.
//     Equivalent CLI:
//     ipset create azure-tt-local-pods hash:ip -exist
//
//  4. raw-table NOTRACK rule — bypasses conntrack for hairpinned packets
//     re-entering on the physical interface where BOTH src and dst are local
//     pods. This is what closes the same-node ClusterIP NSG gap: traffic
//     stays on the VFP path (so NSG deny rules are honoured) without
//     suffering conntrack tuple collisions that previously caused ~50%
//     packet loss for UDP services (e.g. CoreDNS). Cross-node flows are
//     unaffected because the remote pod IP is never in this set, so the
//     reply path still gets un-DNATed by conntrack as usual.
//     Equivalent CLI:
//     iptables -t raw -A PREROUTING -i <hostPrimaryIf> \
//     -m set --match-set azure-tt-local-pods src \
//     -m set --match-set azure-tt-local-pods dst -j NOTRACK
//
//  5. ip rule — directs fwmark-3 packets at the custom routing table.
//     Equivalent CLI:
//     ip -4 rule add fwmark 0x3 lookup 101
//
//  6. default route in custom table — points at the host's physical
//     interface so marked packets leave via VFP.
//     Equivalent CLI:
//     ip route replace default via <gateway> dev <hostPrimaryIf> table 101
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
// The MARK target is only valid in the mangle (and raw) tables. The mangle
// PREROUTING chain runs before the kernel routing decision (chain order:
// raw → mangle → nat → filter), so the fwmark is set before the kernel
// consults the routing table — exactly what we need for policy routing via
// table 101.
// Ref: iptables(8) man page, Netfilter Packet Traversal documentation.
func (client *TransparentTunnelEndpointClient) addTransparentTunnelRules(epInfo *EndpointInfo) error {
	// Gateway is required — without it the custom routing table would have no default
	// route, and all fwmarked packets would be black-holed. Fail early before creating
	// any iptables rules to avoid leaving the node in a partially-configured state.
	if client.gateway == nil {
		return errors.New("cannot add tunnel rules: host gateway is nil")
	}

	hostVeth := client.hostVethName
	markStr := strconv.Itoa(transparentTunnelFwmark)

	// 1. Ensure the local-pods ipset exists (shared, idempotent).
	// Equivalent CLI:
	//   ipset create azure-tt-local-pods hash:ip -exist
	if err := client.ipsetClient.Create(transparentTunnelLocalPodsSet, transparentTunnelLocalPodsSetType); err != nil {
		return errors.Wrap(err, "failed to create local-pods ipset")
	}

	// 2. Add this pod's IPv4 address(es) to the local-pods ipset.
	// Equivalent CLI:
	//   ipset add azure-tt-local-pods <podIPv4> -exist
	// Example: ipset add azure-tt-local-pods 10.224.0.46 -exist
	//
	// IPv6 is currently out of scope for the NOTRACK rule (which is IPv4-only).
	// hash:ip can be created with `family inet6` for IPv6 support; revisit when
	// dual-stack transparent-tunnel is wired up.
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

	// 3. Raw-table NOTRACK rule (shared, idempotent via RuleExists).
	// Equivalent CLI:
	//   iptables -t raw -A PREROUTING -i <hostPrimaryIf> \
	//     -m set --match-set azure-tt-local-pods src \
	//     -m set --match-set azure-tt-local-pods dst -j NOTRACK
	// Example: iptables -t raw -A PREROUTING -i eth0 \
	//            -m set --match-set azure-tt-local-pods src \
	//            -m set --match-set azure-tt-local-pods dst -j NOTRACK
	notrackMatch := buildTransparentTunnelNotrackMatch(client.hostPrimaryIfName)
	if err := client.iptablesClient.AppendIptableRule(
		iptables.V4, iptables.Raw, iptables.Prerouting, notrackMatch, iptables.Notrack,
	); err != nil {
		return errors.Wrap(err, "failed to append NOTRACK rule")
	}
	logger.Info("transparent-tunnel: ensured NOTRACK rule",
		zap.String("dev", client.hostPrimaryIfName),
		zap.String("set", transparentTunnelLocalPodsSet))

	// 4. Fwmark MARK rule (per-pod).
	// Equivalent CLI:
	//   iptables -t mangle -A PREROUTING -i <hostVeth> -j MARK --set-mark <fwmark>
	// Example: iptables -t mangle -A PREROUTING -i azv1234 -j MARK --set-mark 3
	markMatch := "-i " + hostVeth
	markTarget := "MARK --set-mark " + markStr
	if err := client.iptablesClient.AppendIptableRule(
		iptables.V4, iptables.Mangle, iptables.Prerouting, markMatch, markTarget,
	); err != nil {
		return errors.Wrap(err, "failed to append fwmark MARK rule")
	}
	logger.Info("transparent-tunnel: added fwmark MARK rule",
		zap.String("veth", hostVeth), zap.String("mark", markStr))

	// 5. IP rule: fwmark → custom routing table (via netlink).
	// Equivalent CLI:
	//   ip -4 rule add fwmark <fwmark> lookup <table>
	// Example: ip -4 rule add fwmark 0x3 lookup 101
	//
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

	// 6. Default route in custom table via physical interface → VFP (via netlink).
	// Equivalent CLI:
	//   ip route replace default via <gateway> dev <hostPrimaryIfName> table <table>
	// Example: ip route replace default via 10.224.0.1 dev eth0 table 101
	//
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

// buildTransparentTunnelNotrackMatch returns the iptables match expression for
// the bidirectional ipset NOTRACK rule. Centralised so add and delete paths
// always use the byte-identical match string (required for RuleExists /
// iptables-D matching).
func buildTransparentTunnelNotrackMatch(hostPrimaryIf string) string {
	return "-i " + hostPrimaryIf +
		" -m set --match-set " + transparentTunnelLocalPodsSet + " src" +
		" -m set --match-set " + transparentTunnelLocalPodsSet + " dst"
}

// deleteTransparentTunnelRules removes the per-endpoint iptables, ipset, and
// routing-policy rules. It attempts every cleanup step independently and
// aggregates failures via errors.Join so that one transient failure does not
// leave behind unrelated state. Idempotency: iptables rules are pre-checked
// with RuleExists so an absent rule is not treated as a delete failure;
// netlink RuleDel / RouteDel tolerate ENOENT / ESRCH for the same reason;
// ipset Del uses `-exist` so a missing entry is success.
//
// All other errors are surfaced so containerd retries the CNI DEL — leaving
// a stale fwmark MARK rule, ipset entry, or table-101 entry behind would
// break routing or NSG enforcement for later pods on this node.
//
// Sequencing note: this function runs BEFORE IPAM release in the CNI DEL
// path (see deleteEndpointImpl), so the ipset entry is always removed
// while the IP is still "owned" by this pod — eliminating the risk of a
// reassigned IP being unexpectedly NOTRACK'd by a stale set entry.
func (client *TransparentTunnelEndpointClient) deleteTransparentTunnelRules(ep *endpoint) error {
	hostVeth := ep.HostIfName
	markStr := strconv.Itoa(transparentTunnelFwmark)

	var errs []error

	// 1. Remove this pod's IPv4 address(es) from the local-pods ipset.
	// Equivalent CLI:
	//   ipset del azure-tt-local-pods <podIPv4> -exist
	// Example: ipset del azure-tt-local-pods 10.224.0.46 -exist
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

	// 2. Remove fwmark MARK rule (per-pod).
	markMatch := "-i " + hostVeth
	markTarget := "MARK --set-mark " + markStr
	// Pre-check existence — equivalent CLI:
	//   iptables -t mangle -C PREROUTING -i <hostVeth> -j MARK --set-mark <fwmark>
	// Example: iptables -t mangle -C PREROUTING -i azv1234 -j MARK --set-mark 3
	if client.iptablesClient.RuleExists(iptables.V4, iptables.Mangle, iptables.Prerouting, markMatch, markTarget) {
		// Equivalent CLI:
		//   iptables -t mangle -D PREROUTING -i <hostVeth> -j MARK --set-mark <fwmark>
		// Example: iptables -t mangle -D PREROUTING -i azv1234 -j MARK --set-mark 3
		if err := client.iptablesClient.DeleteIptableRule(
			iptables.V4, iptables.Mangle, iptables.Prerouting, markMatch, markTarget,
		); err != nil {
			logger.Error("transparent-tunnel: failed to delete fwmark MARK rule", zap.Error(err))
			errs = append(errs, errors.Wrap(err, "delete fwmark MARK rule"))
		}
	} else {
		logger.Info("transparent-tunnel: fwmark MARK rule already absent, skipping",
			zap.String("veth", hostVeth))
	}

	// 3. Refcount the shared state. The ip rule, route table, NOTRACK rule,
	// and ipset are all shared by every transparent-tunnel endpoint on this
	// node. We only tear them down when no other endpoint's fwmark MARK
	// rules remain in mangle PREROUTING.
	//
	// Equivalent CLI for the refcount check:
	//   iptables -t mangle -S PREROUTING | grep -c -- '--set-xmark 0x3/'
	// Note: `iptables -S` normalises `--set-mark N` to
	// `--set-xmark 0xN/0xffffffff`, so we count the normalised form.
	hexMark := fmt.Sprintf("0x%x", transparentTunnelFwmark)
	out, listErr := client.plClient.ExecuteCommand(context.TODO(), "iptables", "-t", "mangle", "-S", "PREROUTING")
	if listErr != nil {
		// If we can't list, we don't know whether other pods are still using
		// the shared state. Skip the shared teardown (safe — stale shared
		// rules are harmless once all per-pod MARK rules are gone) and
		// surface the error so the runtime retries DEL and eventually drives
		// the count back to zero.
		logger.Error("transparent-tunnel: failed to list mangle PREROUTING for refcount, skipping shared teardown",
			zap.Error(listErr))
		errs = append(errs, errors.Wrap(listErr, "list mangle PREROUTING for refcount"))
		return stderrors.Join(errs...)
	}

	markCount := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "--set-xmark "+hexMark+"/") {
			markCount++
		}
	}

	if markCount == 0 {
		// 4. Delete shared NOTRACK rule in raw PREROUTING.
		// Equivalent CLI:
		//   iptables -t raw -D PREROUTING -i <hostPrimaryIf> \
		//     -m set --match-set azure-tt-local-pods src \
		//     -m set --match-set azure-tt-local-pods dst -j NOTRACK
		notrackMatch := buildTransparentTunnelNotrackMatch(client.hostPrimaryIfName)
		if client.iptablesClient.RuleExists(iptables.V4, iptables.Raw, iptables.Prerouting, notrackMatch, iptables.Notrack) {
			if err := client.iptablesClient.DeleteIptableRule(
				iptables.V4, iptables.Raw, iptables.Prerouting, notrackMatch, iptables.Notrack,
			); err != nil {
				logger.Error("transparent-tunnel: failed to delete NOTRACK rule", zap.Error(err))
				errs = append(errs, errors.Wrap(err, "delete NOTRACK rule"))
			}
		} else {
			logger.Info("transparent-tunnel: NOTRACK rule already absent, skipping",
				zap.String("dev", client.hostPrimaryIfName))
		}

		// 5. Delete shared ip rule (fwmark → table 101).
		// Equivalent CLI:
		//   ip -4 rule del fwmark <fwmark> lookup <table>
		// Example: ip -4 rule del fwmark 0x3 lookup 101
		// (ENOENT/ESRCH from netlink == rule already gone; treated as success.)
		rule := vishnetlink.NewRule()
		rule.Mark = transparentTunnelFwmark
		rule.Table = transparentTunnelRouteTable
		rule.Family = unix.AF_INET
		if err := client.nlPolicyRoute.RuleDel(rule); err != nil {
			if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ESRCH) {
				logger.Info("transparent-tunnel: ip rule already absent, skipping",
					zap.Int("fwmark", transparentTunnelFwmark))
			} else {
				logger.Error("transparent-tunnel: failed to delete ip rule",
					zap.Int("fwmark", transparentTunnelFwmark), zap.Error(err))
				errs = append(errs, errors.Wrapf(err, "delete ip rule fwmark %d", transparentTunnelFwmark))
			}
		}

		// 6. Delete shared default route in custom table.
		// Equivalent CLI:
		//   ip route del default table <table>
		// Example: ip route del default table 101
		// (ENOENT/ESRCH from netlink == route already gone; treated as success.)
		_, defaultDst, _ := net.ParseCIDR("0.0.0.0/0")
		route := &vishnetlink.Route{
			Dst:   defaultDst,
			Table: transparentTunnelRouteTable,
		}
		if err := client.nlPolicyRoute.RouteDel(route); err != nil {
			if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ESRCH) {
				logger.Info("transparent-tunnel: route already absent, skipping",
					zap.Int("table", transparentTunnelRouteTable))
			} else {
				logger.Error("transparent-tunnel: failed to delete route in table",
					zap.Int("table", transparentTunnelRouteTable), zap.Error(err))
				errs = append(errs, errors.Wrapf(err, "delete route in table %d", transparentTunnelRouteTable))
			}
		}

		// 7. Destroy the shared local-pods ipset.
		// Equivalent CLI:
		//   ipset destroy azure-tt-local-pods
		// `ipset destroy` errors if the set doesn't exist; we log+continue
		// for that case (the rest of the cleanup already succeeded).
		if err := client.ipsetClient.Destroy(transparentTunnelLocalPodsSet); err != nil {
			// "set <name> doesn't exist" is benign; surface anything else.
			lc := strings.ToLower(err.Error())
			if strings.Contains(lc, "does not exist") || strings.Contains(lc, "doesn't exist") {
				logger.Info("transparent-tunnel: local-pods ipset already absent, skipping",
					zap.String("set", transparentTunnelLocalPodsSet))
			} else {
				logger.Error("transparent-tunnel: failed to destroy local-pods ipset", zap.Error(err))
				errs = append(errs, errors.Wrap(err, "destroy local-pods ipset"))
			}
		}
	}

	return stderrors.Join(errs...)
}
