# Azure Cilium IPAM installation

`azure-cilium-ipam` is a Linux CNI executable with bounded `--register` and
`--reconcile` commands. It consumes `spec.ipam.pool` and records reservations
in `status.ipam.used` on CiliumNode. Each conditional status update preserves
the operator's inventory, interface metadata and diagnostics.

Pod DEL removes only the matching reservation. The NIC secondary remains
available for another pod on the node. There is no excess-IP release,
cross-node transfer, multi-pool request/retention bookkeeping or annotation
ledger. No local allocation database or shared-state publisher is used.

Use clean or drained standalone Linux VMs, one primary NIC and secondary IPv4
addresses. Azure CNI keeps its existing endpoint store and normal cleanup
behavior. Do not install CNS, a Cilium agent or another primary CNI consumer
alongside this plugin.

## Build and configuration

Build with `make azure-cilium-ipam-binary`, or obtain the matching binary from
the Linux Azure CNI image/archive. Install `azure-vnet` and
`azure-cilium-ipam` from the same build into `/opt/cni/bin`.

Replace the node-specific values in `ipam-settings.json`. Supply a protected,
node-scoped kubeconfig at its configured path. The executable reloads that
input for each invocation; the platform owns credential renewal.

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

Inspect and install the generated file as the sole primary CNI configuration
at `/etc/cni/net.d/10-azure.conflist`. CNI and reconciliation use the same
settings. `ipam.type` is the only mode selector. Unknown settings, including
`stateDir`, `releaseExcessIPs` and `coordinatePeers`, are rejected.

## Operator and permissions

Use the Azure-capable Cilium v1.20.1 contract with its matching chart and CRD.
`operator-values.yaml` pins the Azure operator image, disables agent/CNI
installation, excludes primary-address allocation and disables excess release.
Verify initial allocation and usage-driven growth on the selected deployment.
The plugin does not use the multi-pool release protocol.

Use `node-rbac.yaml` as a per-node template. Normal credentials can read their
Node and CiliumNode and patch only the CiliumNode **status subresource**.
They have no CiliumNode create/delete permission or spec/annotation patch
permission. Registration uses a separately authorized bootstrap identity.
No ARM allocation permission is needed by the plugin.

## Registration and reconciliation

Keep the node out of workload scheduling until registration and qualified
supply are ready. Kubernetes must already contain the node, and operator/API
bootstrap must not depend on allocating an ordinary pod IP.

Using a protected copy of the conflist with bootstrap credentials:

```sh
/opt/cni/bin/azure-cilium-ipam --config /secure/bootstrap.conflist --register
```

Registration creates the Node/VM binding and reserve settings only when the
CiliumNode does not exist. Replays validate existing ownership and never
overwrite it. ADD does not register or repair a missing object.

Install the included service and timer for periodic, one-shot reconciliation.
The timer is not an allocation daemon and does not maintain local state.
Reservations carry a versioned owner containing the VM, boot, network, sandbox
and interface identities. After an actual reboot of the same bound VM,
reconciliation retires only recognized previous-boot reservations.

The host must use fresh sandbox identities after reboot and must not restore
stale pod endpoints, per-pod rules or endpoint files. Keep pod secondaries off
the host interface. A successful reconciliation is not a dataplane health
check; verify usable networking before scheduling.

## Failure behavior

ADD confirms a remote claim before returning an address. Duplicate ADD reuses
the same eligible reservation. An ineligible existing claim does not receive
a second IP. Exhaustion returns an error without changing the pool or recording
pending demand; the operator replenishes reserve from the published used map.

DEL can release by exact sandbox identity without endpoint state or current
NMAgent supply. It never frees a different owner at a reused address. API
failures return errors, and a later operation re-reads authoritative state to
resolve uncertain outcomes. No offline allocation or file-deletion fallback
exists.

The plugin rejects incompatible multi-pool state and an existing
`azure-container-networking.io/ipam-state` annotation rather than silently
discarding their ownership. Changing an active consumer requires drain and
resolved reservations; there is no automatic live format conversion.
