# Transparent-tunnel CNI mode

Transparent-tunnel is a Linux Azure CNI mode that keeps the existing transparent
endpoint setup and adds a small set of kernel rules so same-node pod-to-pod
traffic reaches Azure VFP for NSG / ASG enforcement.

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

Transparent-tunnel prefers the per-pod IPAM gateway from `EndpointInfo.Gateways`,
then the host external-interface gateway, and finally the live default route on
the host primary interface. This keeps bridge-shape NodeSubnet hosts working
when the default route lives on the bridge and the external-interface gateway is
persisted as `0.0.0.0`.

## Installation

The release image must include the transparent-tunnel conflist payload. The CNI
image build copies `cni/azure-linux-transparent-tunnel.conflist` into the
`/dropgz` payload as `azure-transparent-tunnel.conflist`.

To install on a self-managed cluster, deploy the transparent-tunnel installer
DaemonSet:

```sh
kubectl apply -f hack/manifests/cni-installer-transparent-tunnel.yaml
kubectl rollout status ds/azure-cni-transparent-tunnel -n kube-system
```

The DaemonSet extracts the CNI binaries into `/opt/cni/bin` and writes
`azure-transparent-tunnel.conflist` to `/etc/cni/net.d/10-azure.conflist`.
New or recreated pods will then invoke `azure-vnet` with
`"mode": "transparent-tunnel"`.

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
