# Azure Cilium IPAM installation

`azure-cilium-ipam` is a Linux CNI executable with bounded `--register` and
`--reconcile` commands. It keeps reservations and pending demand in the
`azure-container-networking.io/ipam-state` annotation on CiliumNode and updates
the default multi-pool request and retained prefixes in the same conditional
API transaction. `status.ipam.used` must remain empty.

Use a clean or drained standalone VM, one primary NIC and secondary IPv4
addresses. Azure CNI keeps its existing endpoint store and normal ADD/DEL
cleanup behavior. Do not install CNS, a Cilium agent or a second primary CNI
alongside this consumer.

## Build and configuration

Build with `make azure-cilium-ipam-binary`, or obtain the matching binary from
the Linux Azure CNI image/archive. Install `azure-vnet` and
`azure-cilium-ipam` from the same build into `/opt/cni/bin`.

Replace all node-specific values in `ipam-settings.json`. Supply a protected,
node-scoped kubeconfig at the configured path; the executable reads it for each
invocation. The platform owns renewal and atomic replacement of credentials.

Derive the conflist from the golden Linux configuration:

```sh
bash cni/scripts/gen-transparent-tunnel-conflist.sh \
  cni/azure-linux.conflist /tmp/azure-transparent-tunnel.conflist
jq --slurpfile settings examples/cilium-operator-ipam/ipam-settings.json \
  '.plugins |= map(if .type == "azure-vnet" then
    .ipam = {"type":"azure-cilium-ipam","cilium":$settings[0]}
    else . end)' \
  /tmp/azure-transparent-tunnel.conflist > /tmp/10-azure.conflist
```

Inspect the generated configuration before installing it as the sole primary
CNI configuration at `/etc/cni/net.d/10-azure.conflist`. Both CNI operations and
the reconciliation command read the same settings. No host file's existence
selects the allocator. Unknown settings, including a local `stateDir`, fail.

## Operator requirements and release gate

Use an unmodified Azure-capable operator that consumes
`spec.ipam.pools.requested` and publishes Azure interface metadata. Pin and
qualify its image, chart and CRD together. The legacy Azure v1.20.1 allocator
does not implement this consumer contract; an image that only allocates an
initial pool is insufficient. Verify actual usage-driven growth.

Render the chart with `operator-values.yaml` and explicit qualified Azure
image inputs. The values deliberately do not choose an unverified image.
Keep operator excess-IP release **disabled**. The consumer retains qualified
supply, publishes demand, and preserves any withdrawal exclusions it reads.
It never writes the legacy release handshake or calls ARM.

Cloud shrink and cross-node coordination remain disabled until release,
cancellation and rebound-at-capacity qualification succeeds against the
unmodified operator. Setting `releaseExcessIPs` or `coordinatePeers` to true
returns an explicit qualification error. There is no unsafe override.

## Register and reconcile

Keep the node out of workload scheduling until registration and qualified
supply are ready. Kubernetes must already contain the node. Operator/API
bootstrap must not depend on obtaining a pod address from this pool.

Run registration once using a separately authorized bootstrap kubeconfig in
a protected copy of the same conflist:

```sh
/opt/cni/bin/azure-cilium-ipam --config /secure/bootstrap.conflist --register
```

Registration creates the Node/VM binding and an empty ledger only when the
CiliumNode does not exist. Replays validate existing ownership and never
overwrite it. Ordinary ADD does not register or repair a missing object.

Use `node-rbac.yaml` as a per-node template for normal credentials. Main
resource patch permission is required because the annotation and pool spec
are updated together. The normal identity has no create/delete permission
or access to other nodes. Use platform admission controls for field-level
restrictions; RBAC alone does not provide them.

Install the included service and timer for periodic, one-shot reconciliation.
The timer is not an allocation daemon and does not maintain local state.
The platform must qualify fresh-sandbox reboot behavior: do not restore stale
pod endpoints, per-pod rules or endpoint files; keep pod secondaries off the
host interface. Kernel boot identity retires only previous-boot reservations
and pending requests on the same bound VM.

Reconciliation preserves current-boot ownership and release exclusions,
expires only pending requests and skips unchanged API writes. A successful
service invocation is not a dataplane health check. Verify guest-qualified
supply and real pod networking before scheduling.

## Failure behavior

ADD confirms a remote claim before returning its address. Duplicate ADD reuses
the same eligible reservation. An ineligible existing claim does not receive
a second address. Exhaustion records one pending request per sandbox; retries
refresh it and unrefreshed pending demand expires after five minutes.

DEL can release by exact sandbox identity without endpoint state or current
NMAgent supply. It never frees another owner at a reused address. API failures
return errors, and a later request re-reads authoritative state to resolve
uncertain outcomes. No offline allocation or file-deletion fallback exists.

The ledger is bounded by node address/pending limits and the Kubernetes total
annotation limit. Corruption, incompatible profiles and identity changes are
errors, not an empty pool. Do not remove the annotation or reset the retained
set on an active node.
