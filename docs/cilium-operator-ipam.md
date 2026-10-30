# Cilium Operator pools with Azure CNI

This Linux integration keeps Cilium Operator as the Azure NIC
secondary-IPv4 allocator and replaces the Cilium agent/CNI with stateful Azure
CNI. It targets clean or fully drained standalone Azure VMs with one primary NIC
and a single node/pod subnet. It does not support AKS, VMSS, multiple NICs,
Windows, IPv6, prefixes, active-endpoint migration or automatic cloud shrink.
Operator-only runtime startup and Azure NSG enforcement remain deployment gates;
local contract tests do not establish either.

```text
Cilium Operator -> ARM NIC secondary IP configurations
       -> CiliumNode.spec.ipam.pool + status.azure.interfaces
       -> azure-vnet-ipam-bridge -> existing Azure CNI IPAM store
runtime -> azure-vnet -> azure-vnet-ipam
       -> transparent-tunnel -> primary NIC -> Azure VFP/NSG -> pod
```

CNS is not involved. The bridge has no ARM client, cloud-sizing algorithm,
assignment server or separate reservation database. Cilium agent, Cilium CNI,
eBPF policy/dataplane and CNI chaining must not run alongside it.

## Pool and usage ownership

The operator owns `spec.ipam.pool`, including both unused and used addresses,
and publishes interface metadata under `status.azure.interfaces`. The bridge
creates the node's CiliumNode, validates its Kubernetes Node UID, owner
reference, IMDS VM resource ID and immutable VM ID, primary IP and NIC identity.
`spec.instance-id` is the lower-case standalone VM ARM resource ID, without
`azure://`; `spec.azure.interface-name` is the Azure NIC name, not `eth0`.
The Node's provider ID is validated when present; kubeadm Nodes without a
provider ID are bound using IMDS and the Node's matching internal IP.

An eligible IP must be in that node's operator pool, reference the selected
NIC, appear as a successful secondary in its operator metadata and also appear
on the selected primary guest NIC in a fresh NMAgent response. The primary
address is never admitted. Azure CNI does not independently import all NIC
secondaries in this mode. A delayed metadata/programming observation withholds
the address; an invalid or foreign pool entry rejects synchronization.

Azure CNI ADD/DEL remains the only reservation authority. The bridge reads the
same persisted records under the database's exclusive file lock and publishes records with
`InUse` or a remaining container ID into `status.ipam.used`, retaining the
operator's interface resource and a stable container owner. It never overwrites
the operator's other status fields. Resource-version and UID-tested JSON
patches detect concurrent changes; retries take fresh local and API snapshots.
A single lifetime lock prevents two bridge publishers on the same host.

Usage reporting lets the operator replenish the warm reserve. For example,
minimum 10 and preallocation 8 require additional allocation as actual usage
reduces the free reserve below 8. Merely discovering secondaries does not tell
the operator how many Azure CNI has reserved.

Withdrawn free addresses disappear from local supply. Withdrawn assigned
addresses retain their ownership and remain reported as used, but cannot be
allocated again, including through explicit-IP or duplicate-owner requests.
They become locally reusable only after qualified supply returns. DEL releases
ownership after endpoint teardown. Warm IPs never create tunnel endpoints.

## Durable host configuration

Install root-owned, non-group/other-writable configuration at
`/etc/azure-vnet/cilium-pool.json`, based on
[`cilium-pool.json`](../examples/cilium-operator-ipam/cilium-pool.json).
Replace every node-specific placeholder. Ancestor paths and the configured
state path must also be root-owned, non-writable by others and not symlinks.
The selected state directory must be on persistent host storage, not `/run`,
`/var/run`, tmpfs or a container-only filesystem. Precreate it with mode 0700.
Do not change or remove this configuration while endpoints exist.

Both host CNI executables and the bridge read the same file. Their network and
IPAM state is stored in `azure-vnet.db` and `azure-vnet-ipam.db` under
`stateDir`, using bbolt (the maintained BoltDB implementation). Each CNI
operation holds the database file lock from restore through its final write;
each write is a synchronous database transaction. The bridge uses the same
databases and locks, with no separate reservation database or state lock files.
Its `azure-vnet-ipam-bridge.db` lifetime lock prevents duplicate publishers.
Without the host configuration file, legacy JSON storage, paths and static
Azure IPAM behavior are unchanged.

The pool mode requires clean or drained nodes; legacy JSON state is not
automatically imported into the new databases. Do not switch storage formats
while endpoints exist.

Run the bridge binary on the host using the
[example systemd service](../examples/cilium-operator-ipam/azure-vnet-ipam-bridge.service).
It is included in Linux CNI builds, Docker payloads and archives, or can be built
with `make azure-vnet-ipam-bridge-binary`. Supply a root-readable, node-scoped
kubeconfig at `/etc/azure-vnet/bridge.kubeconfig`; the executable also supports
in-cluster credentials. Do not copy administrative POC credentials into manifests
or source control. Arrange credential renewal for the host service.
The service permits writes to `/var/log` because the existing IPAM library uses
Azure CNI's rotating log file there; bridge synchronization health goes to the
service journal.

Use the generated transparent-tunnel conflist:

```sh
bash cni/scripts/gen-transparent-tunnel-conflist.sh \
  cni/azure-linux.conflist /tmp/azure-transparent-tunnel.conflist
```

After review, install it as the runtime's sole primary CNI configuration.
Keep `type: azure-vnet`, `mode: transparent-tunnel` and
`ipam.type: azure-vnet-ipam`. The host configuration selects the pool mode; no
Cilium chain or CNS entry is added. IPv6 and incompatible modes are rejected.
Use binaries from this source build, not an unverified existing released image.

## Operator-only installation and permissions

The wire contract is pinned to Cilium v1.20.1 source commit
`7d68cfb394f2960e10aa72e76d0d51e66c1b2ebc`. Use its CRD schema and a verified
operator image built from matching source. The example pins the chart-published
Azure operator digest
`sha256:13746a479ca60395df8d83580b49464c6c975cca6fdb21ba4791a076ab01b031`.
The downloaded `cilium-1.20.1.tgz` chart SHA-256 is
`06210eef7c23d15f7699c79e2fe3a1ec9c389024c5c5c006ea04022d322449a2`.
The operator registers its embedded CRDs (`skipCRDCreation: false`); verify the
installed schema and runtime image/source provenance before admission. These
artifact pins are not evidence of what an existing cluster is running.

Render, inspect and record the output before any approved installation:

```sh
helm template cilium cilium --repo https://helm.cilium.io --version 1.20.1 \
  --namespace kube-system \
  -f examples/cilium-operator-ipam/operator-values.yaml \
  > operator-rendered.yaml
```

The [values example](../examples/cilium-operator-ipam/operator-values.yaml)
disables the agent, CNI installation, Envoy, Hubble, load-balancer IPAM,
Cilium-specific node taint/status changes and unmanaged-pod restarts. It selects
Azure IPAM and explicitly excludes primary-address allocation.
Verify the pinned distribution supports all rendered settings and
does not reinstall a Cilium DaemonSet or rewrite the Azure conflist.
The operator uses host networking and requires Azure IMDS access and its own
managed identity/ARM NIC-write permissions. The bridge does not receive those
permissions. Operator startup without an agent, CRD registration and successful
pool growth must be demonstrated on a separately approved canary.

The [RBAC example](../examples/cilium-operator-ipam/bridge-rbac.yaml) uses a
separate identity per node, named-object Node reads and CiliumNode
reads/watches/status patches. Name-filtered watches satisfy `resourceNames`
authorization. Kubernetes RBAC cannot constrain `create` by resource name:
enforce the expected name, Node owner and consumer annotation with admission
policy before granting create access in a shared cluster. A shared DaemonSet
service account is not a node-isolation boundary.

## Recovery and health

Fresh bootstrap is permitted only when the CiliumNode and local network/IPAM
state are absent. Existing CiliumNode objects are not silently adopted. The
bridge persists the new UID/VM binding before admitting supply and restores and
publishes usage before enabling admissions. A crash after API creation but before
that initial durable write deliberately requires fenced recovery.

Healthy synchronization grants a 30-second admission lease tied to the kernel
boot ID. During a bridge/API outage, existing confirmed supply can be allocated
only until this lease expires; existing endpoints and DEL continue working.
Reboot requires a new bridge observation. The bridge watches its CiliumNode,
periodically re-GETs it and the Node, and reopens the databases under their
exclusive file locks to avoid overwriting intervening CNI changes.
No network call is made under the local state locks. Recovery checks acquire
network then IPAM locks, in the same order as CNI.

Missing/corrupt IPAM state, replacement Node/CiliumNode/VM identity, or missing
network state with reservations stops admission. Restoration checks the bound
address space and pool, address/NIC metadata and unique container ownership
before rebuilding indexes; partial records are not treated as an empty pool.
Identity changes and unexpected
release requests persist a fence that normal reconciliation cannot clear.
Do not delete database files or reset remote `used` to force recovery. Fence/drain
the VM, establish authoritative endpoint teardown and reconcile local/remote
ownership before an administrator replaces state or reinstalls a consumer.
The old empty-pool state-deletion fallback is disabled in pool mode.

Structured `pool synchronized` logs expose pool, eligible, used and quarantined
counts, free reserve, reserve deficit and successful observation/publication
timestamps. `usageConfirmedAt` advances on matching remote usage without issuing
a redundant patch; `lastUsagePublication` advances only on an actual status
write and is unknown after startup until the first write.
Watch/API/discovery/identity failures are logged explicitly. An absence of
recent success logs indicates stale health; operator ARM-operation metrics
remain the operator's responsibility.

The pinned Azure operator does not implement NIC IP release. Pod DEL returns
IPs to the local warm pool, not the Azure subnet. Any `release-ips` handshake
permanently fences new admissions; the bridge never sends `ready-for-release`.
A release-capable operator is a separate compatibility change.

## Enforcement gate

[Transparent-tunnel](transparent-tunnel.md) remains unchanged: pod-veth mark 3,
table 101 via the live primary-NIC gateway, active-local-pod ipset membership,
and scoped re-entry NOTRACK. It preserves VNet pod source addresses and avoids
marking physical-NIC re-entry back out again.

Before rollout, demonstrate operator-only bootstrap, real usage-driven pool
growth, local reuse and restart/outage safety on clean standalone VM canaries.
Then test same-node and cross-node allow -> ASG-backed deny -> allow using a
fixed TCP listener and placement, fresh connections, confirmed ASG membership
and an unselected control pair. Record endpoint IPs, routes/marks, NIC captures
and listener-observed source IPs. A missing response, Ready pod or routing
rule alone does not establish an NSG drop.

Also cover DNS, API access, kube-proxy services, egress and host probes.
Pod NSG Controller membership updates remain asynchronous; established NSG
flows and IP reuse across labels have convergence windows. No atomic policy
admission guarantee is provided. Rollback drains Azure CNI endpoints before
stopping the bridge and restoring one compatible consumer; never run both
consumers or bulk-delete secondary IPs.
