# Pool observability

The bridge serves `/healthz`, `/readyz`, `/status` and `/metrics` on
`127.0.0.1:10093`. Set `AZURE_VNET_POOL_HEALTH_ADDRESS` in its service to change
the bind address. Remote exposure requires a protected scrape path; these
endpoints do not implement authentication. `/status` contains node/resource
identities and diagnostic errors.

`/healthz` is process liveness. `/readyz` reflects the observed allocation
lease, boot identity and persistent fence, not policy or traffic health.
Exhaustion is a separate free-address count. Local state is sampled read-only
every five seconds, with a bounded lock wait. Failed reads make admission
unknown immediately; observations expire after 30 seconds. Lease expiry is
evaluated on each request, even if the sampler stalls. Observations do not
modify the actual CNI admission checks.

## Scraping and dashboards

Use `prometheus.yaml` as a starting configuration and import `grafana.json`.
Run the loopback scrape on each node, or replace it with an explicitly
configured protected remote endpoint. Set stable `cluster` and `node` target
labels; keep node names identical to the operator's `target_node` values.
Do not use a central Prometheus's loopback as a remote-node address.

Replace the placeholders in `operator-targets.json`, `controller-targets.json`, and
`mapping-state-targets.json` with the actual metrics targets. Each uses the
Prometheus file-discovery shape, for example:

```json
[{"targets":["REPLACE_METRICS_HOST:9963"],"labels":{"cluster":"REPLACE_CLUSTER"}}]
```

Operator port 9963 is the pinned chart default. Use the controller's actual
metrics port and authentication configuration rather than assuming it is the
same. Enable the kube-state-metrics custom-resource collector using
`mapping-state.yaml` and grant its service account get/list/watch access to
`networking.azure.com/podasgmappings`. Scrape it under `pod-nsg-mapping-state`.
Adapt authentication in the scrape jobs to the deployed endpoints.

Metric contracts were checked against Cilium
`7d68cfb394f2960e10aa72e76d0d51e66c1b2ebc` and Pod NSG Controller
`b1674fb3b61b5b05b99f9f65d82582335cfd1199`. Verify deployed versions expose
these contracts. In Cilium, Azure `response_code` is `OK` or `Failed`, not an
HTTP status code. Cloud allocation latency is operator/subnet scoped; database
histograms measure bridge operations, not every short-lived CNI invocation.

Pool counts are last observations, not live reads. Pair them with storage
and qualified-pool freshness. Usage confirmation can advance without a PATCH;
`publication` advances only on a real PATCH. Zero timestamps mean never observed.
An idle publication series becoming stale is normal and has no failure alert.
No-data panels remain unknown instead of being filled with healthy values.

Controller `prefix_set_convergence_seconds` measures detection to ARM PUT
success. The `Reconciled` condition and observed generation report controller
progress, not fabric enforcement; they cannot account for pod events the
controller has not yet observed. Keep controller availability, conditions,
ARM errors and actual traffic visible together. The bridge's policy/dataplane
known gauges remain zero because it does not own those observations.

### Bridge and admission

Query `azure_vnet_pool_admission_ready`,
`azure_vnet_pool_fence_reason`, and
`azure_vnet_pool:observation_age_seconds{stage="storage"}`.
Inspect `/status` and `journalctl -u azure-vnet-ipam-bridge`.
Restore the indicated dependency for an expired lease. A persistent fence
requires the existing ownership-preserving recovery procedure; never delete
databases or reset remote usage to silence an alert.

### Dependency and storage

Query `azure_vnet_pool_errors_total` by operation/reason and
`azure_vnet_pool:operation_p99_seconds{operation=~"database_.*"}`.
Use stage errors in `/status` to distinguish unauthorized API calls, guest
metadata errors and unreadable state. For storage, inspect disk capacity,
permissions and concurrent lock holders. The one-second latency threshold
and five-minute sustained interval are starting operational thresholds;
tune against measured latency without extending the admission lease.

### Credentials

Query `azure_vnet_pool_credential_expiry_known`,
`azure_vnet_pool_credential_expires_timestamp_seconds`, and API errors with
`reason="unauthorized"`. Expiry observation is not signature verification or
proof that the API accepts a credential.

Inline certificates/tokens describe the configuration loaded by this process.
Token/certificate files are reread for expiry observation; a renewed file does
not prove the client used it. Verify successful API observations after renewal.
Replacing an inline kubeconfig token does not change the already-loaded client.
Exec providers, auth providers and opaque credentials have unknown expiry and
need external renewal monitoring. The warning lead time is one hour.

### Operator and capacity

Query `azure_vnet_pool_addresses{kind=~"free|reserve_deficit"}`,
`cilium_operator_ipam_needed_ips`, `cilium_operator_ipam_available_ips`,
`cilium_operator_ipam_nodes{category="at-capacity"}`, and
`cilium_operator_azure_api_duration_seconds_count{response_code="Failed"}`.
Check the operator deployment, managed identity and subnet capacity before
changing reserve sizing. ARM metrics do not identify a particular affected
node; correlate with node demand instead of inventing a per-node attribution.

### Policy

Query `pod_nsg_mapping_reconciled`, `pod_nsg_mapping_observed_generation`,
`pod_nsg_mapping_generation`, and
`pod_nsg_controller_prefix_set_convergence_seconds_count`.
Inspect the matching PodASGMapping conditions and controller ARM errors.
Resolve controller/authorization failures, verify reconciliation to the current
generation, then verify traffic. The two-minute condition alert is a starting
convergence budget, not an atomic policy-admission guarantee.

### Routes and traffic

Query `azure_vnet_pool_observation_healthy{stage="routes"}` and pod probes.
Inspect `ip -4 rule`, `ip -4 route show table all` and `ip -4 addr show`.
The bridge checks the primary gateway, table 101, mark rule ordering and
host-local routes for addresses in the persisted pool. It does not repair
routes or certify the entire dataplane.

Mount `blackbox.yaml` into blackbox-exporter pods on explicit worker nodes
using Azure CNI with `hostNetwork: false`. Use a fixed TCP listener pod on the
same node and another on a different node, and fill in `probe-targets.json`.
The source is the exporter pod, not Prometheus. Apply the intended selectors
to that source pod, retain an unselected control pair, and restrict who can
call the exporter probe endpoint. Record actual pod UIDs, IPs, nodes and target
ports so reused addresses cannot silently change what a probe represents.

For allow/deny/allow qualification use fresh connections and verified listener
health. Unexpected successful access during deny is a failure; a timeout
without healthy controls does not prove NSG enforcement. Compare source-IP
observations and NIC captures when attributing failures.

## Validation and fault injection

```sh
promtool check rules rules.yaml
promtool test rules rules.test.yaml
promtool check config prometheus.yaml
```

Use a separately authorized canary with a verified healthy baseline. Inject
one failure at a time, retain evidence, undo it and demonstrate recovery before
the next injection. Never corrupt the only authoritative production database.

| Injection | Expected distinction and action |
| --- | --- |
| Expired/rejected credential | Expiry/authentication alerts; API success stops while process liveness stays up. Renew and verify actual client recovery. |
| API loss | API observation failure, then lease expiry; existing dataplane probes may still succeed. Restore API access. |
| Operator outage or exhaustion | Operator/deficit signals and falling free reserve; bridge liveness and lease may remain healthy. Restore operator/capacity and observe replenishment. |
| Guest metadata failure | Guest freshness fails independently of local state sampling; lease expires. Restore metadata access. |
| Storage failure or lock contention | Storage becomes unknown, timings/errors identify the failure, and counts retain their last-observed status. Restore storage without resetting ownership. |
| Host-local route for a canary pod IP | Route and traffic alerts while pool synchronization and lease remain healthy. Restore host configuration, then verify probes. |
| ASG controller outage/delayed reconciliation | Controller/condition evidence degrades independently of allocation. Restore it and verify membership and traffic. |

Capture alert firing and clearing, `/status`, metric samples and dashboard
queries for every injection. Local tests validate signal logic; this checklist
is not a claim that Azure fault injection or fabric enforcement has been tested.
