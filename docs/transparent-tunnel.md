# Transparent-tunnel CNI mode

Transparent-tunnel is a Linux Azure CNI mode that keeps the existing transparent
endpoint setup and adds a small set of kernel rules so same-node pod-to-pod
traffic reaches Azure VFP for NSG / ASG enforcement.

## Deployment model

Transparent-tunnel runs as **stateful CNI** with **`azure-vnet-ipam`** as the IP
provider, inherited from the golden Linux conflist (`cni/azure-linux.conflist`).
CNS is not in the path: addresses come from `azure-vnet-ipam` rather than
`azure-cns`, and endpoint state is persisted by CNI itself rather than read
back from CNS.

This matters for deletion. Because the host veth name and pod IP are restored
from CNI's own state store, DEL can always remove the ipset entry and the
mangle MARK rule, and it does not depend on CNS being reachable. The stateless
CNI path (the separate `azure-vnet-stateless` binary, which sources endpoint
state from CNS) is not used by this mode.

## Problem

In transparent mode, same-node pod-to-pod packets stay in the host Linux
networking path and never reach the Virtual Filtering Platform (VFP) on the
Azure host. VFP is where NSG and NSG-with-ASG rules are enforced, so intra-node
pod traffic can bypass rules that are enforced for cross-node traffic.

## Packet flow

```text
pod A
  |
  | host-side veth: mangle PREROUTING marks packet with fwmark 0x3
  v
Linux routing policy
  |
  | ip rule fwmark 0x3 lookup table 101
  v
table 101 default route via host primary NIC
  |
  v
Azure VFP / NSG enforcement
  |
  | hairpin re-entry on host primary NIC
  v
raw PREROUTING NOTRACK when src and dst are both local pod IPs
  |
  v
pod B
```

## Rules

Transparent-tunnel uses both per-pod state and node-scoped shared state.

### Per-pod state

Each transparent-tunnel endpoint creates:

```sh
ipset add azure-tt-local-pods <podIPv4> -exist
iptables -t mangle -A PREROUTING -i <hostVeth> -j MARK --set-mark 3
```

The ipset entry identifies local pod IPs. The MARK rule identifies packets that
originated from a pod veth and carries that decision into routing policy.

### Shared node state

The shared state is ensured idempotently during endpoint ADD:

```sh
ipset create azure-tt-local-pods hash:ip -exist
iptables -t raw -A PREROUTING -i <hostPrimaryIf> \
  -m set --match-set azure-tt-local-pods src \
  -m set --match-set azure-tt-local-pods dst -j NOTRACK
ip -4 rule add fwmark 0x3 lookup 101
ip route replace default via <gateway> dev <hostPrimaryIf> table 101
```

The shared state is intentionally not removed by pod DEL. Without per-pod MARK
rules and local-pod ipset entries, it is inert; keeping it installed avoids
races where one pod DEL removes shared state while another pod ADD is still in
progress.

## Why fwmark instead of a source CIDR rule

In NodeSubnet mode, pod IPs and node IPs share the same VNet subnet. There is no
distinct pod CIDR that can safely be used in an `ip rule from <cidr>` selector.
Matching the whole subnet would also capture node-originated traffic such as
kubelet or health probes.

The host-side veth match in iptables is the point where pod-originated traffic
is distinguishable from node-originated traffic. The fwmark then carries that
decision into routing policy.

## Why NOTRACK uses a local-pods ipset

Hairpinned same-node packets enter the host twice: first from the pod veth, then
again after returning through the host primary NIC. Conntrack can see two views
of the same flow and drop packets because of tuple collisions.

The raw-table NOTRACK rule applies only when both source and destination are in
`azure-tt-local-pods`. That limits NOTRACK to same-node pod-to-pod hairpin
traffic. Cross-node traffic remains tracked because the remote pod IP is not in
the local-pods set, so NAT / un-DNAT and established-flow matching keep working.

## Gateway selection

The table-101 default route must use a real IPv4 next hop. A zero gateway
(`0.0.0.0`) is treated by the kernel as a link-scoped default route, which only
works for same-subnet destinations and can black-hole off-subnet pod egress.

Transparent-tunnel reads the gateway from exactly one place: the host's IPv4
default route on the host primary interface, queried live from netlink on every
ADD. There is intentionally no fallback chain. The other candidates are not
independent values — `externalInterface.IPv4Gateway` is cached from this same
route by `saveIPConfig` at network-create time, and `EndpointInfo.Gateways` is in
turn populated from `externalInterface`. Consulting them only introduces the risk
of using a stale copy, which is how a persisted `0.0.0.0` gateway could reach the
route table. If the host has no default route with a usable gateway, the ADD
fails rather than installing a route that would black-hole pod traffic.

## Installation

The release image must include the transparent-tunnel conflist payload. There is
no checked-in transparent-tunnel conflist: the build derives it from the golden
Linux conflist with `cni/scripts/gen-transparent-tunnel-conflist.sh`, which
rewrites only the plugin `mode`, and drops the result into the `/dropgz` payload
as `azure-transparent-tunnel.conflist`. Both the container build
(`cni/Dockerfile`) and the dalec build (`.pipelines/build/scripts/cni.sh`) call
the same script, so the tunnel conflist cannot drift from the golden one and
picks up future changes to it automatically. The script fails the build if the
golden conflist has no `transparent` mode to rewrite, rather than silently
emitting a conflist that would select plain transparent mode.

A self-managed installer DaemonSet is not included in this change. It pins a
specific `azure-cni` image tag, and no released image contains this mode yet.
An older image does not fail loudly: the unknown `transparent-tunnel` mode
string falls through the endpoint-client dispatcher to the bridge client, so
pods come up with no tunnel enforcement at all. The installer manifest will
ship in a follow-up change once a CNI image containing this mode is released.

Until then, use a locally built image. The installer is expected to extract the
CNI binaries into `/opt/cni/bin` and write `azure-transparent-tunnel.conflist`
to `/etc/cni/net.d/10-azure.conflist`. New or recreated pods then invoke
`azure-vnet` with `"mode": "transparent-tunnel"`.

## Lifecycle

Endpoint ADD:

1. Run normal transparent endpoint setup.
2. Ensure the shared ipset, NOTRACK rule, fwmark rule, and table-101 route.
3. Add this pod's IPv4 address to the local-pods ipset.
4. Add this pod's mangle MARK rule.

Endpoint DEL:

1. Remove this pod's IPv4 address from the local-pods ipset.
2. Delete this pod's mangle MARK rule.
3. Leave shared node-scoped state in place.

`DeleteEndpointRules` is part of the existing endpoint interface and cannot
return errors, so transparent-tunnel cleanup is exposed through
`DeleteTransparentTunnelRules`. Delete failures are returned to the CNI runtime
so DEL can be retried.

## Validation

On a node with transparent-tunnel pods:

```sh
sudo ipset list azure-tt-local-pods
sudo iptables -t mangle -S PREROUTING | grep MARK
sudo iptables -t raw -S PREROUTING | grep NOTRACK
sudo ip rule show | grep '0x3 lookup 101'
sudo ip route show table 101
```

Expected state:

- one local-pods ipset entry per local transparent-tunnel pod IPv4 address
- one mangle MARK rule per local transparent-tunnel pod host veth
- one raw NOTRACK rule matching local-pods `src` and `dst`
- one fwmark rule for table 101
- one table-101 default route via the host primary interface gateway
