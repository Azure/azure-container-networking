# MultiTenantNetworkContainer CRDs

This package contains the CRD definitions for MultiTenantNetworkContainer, which would be consumed in CNS.

## IPv6 status

IPv4-only objects omit all four IPv6 fields. For dual-stack objects, the status
producer supplies `ipv6`, `ipSubnetV6`, and `gatewayV6` together:

```yaml
status:
  ipv6: "fd00:1234::4"
  ipv6Prefix: "fd00:1234::4/128"
  ipSubnetV6: "fd00:1234::/64"
  gatewayV6: "fe80::1"
```

`ipv6` is the assigned address; `ipSubnetV6` is its subnet and supplies the
interface prefix length (64 in this example). The optional `ipv6Prefix` describes
the allocation, not the interface mask. When present, it must contain the address
and fit within `ipSubnetV6`. The gateway may be link-local, outside the subnet.
Addresses and prefixes must be IPv6, without IPv4 mapping or zone identifiers.
CNS rejects incomplete or invalid IPv6 configuration before persisting the NC or
marking it `Succeeded`; it does not silently fall back to IPv4 for partial status.

The same conversion is used for `Initialized` objects and recovery of
`Succeeded` objects whose NC is missing from CNS. Existing NCs retain the current
skip behavior: changing CR status does not retrofit IPv6 into an already persisted
IPv4-only NC. This contract does not enable a host capability or AZR.

## Schema rollout

Regenerate the checked-in schema and deepcopy code with:

```sh
make -C crd/multitenantnetworkcontainer
```

Generation does not install the CRD. The CNS multitenant controller registers the
Go types and watches existing objects; it neither creates nor upgrades this CRD.
The cluster/control-plane component that owns CRD installation must apply
`manifests/networking.azure.com_multitenantnetworkcontainers.yaml` through its
deployment process. That installer is not implemented in this package.

Roll out the additive schema and an IPv6-aware CNS consumer before the status
producer emits IPv6 fields. Verify that the installed CRD's `v1alpha1` status
schema includes all four properties, not just that a new CNS image is deployed.
Old schemas can prune unknown fields on writes or reject them under strict
validation/server-side apply. If all IPv6 fields are pruned, CNS sees an IPv4-only
object and cannot detect the producer's intended dual-stack configuration.
Re-publish the complete status after upgrading the schema; previously pruned
values are not recovered automatically. Existing IPv4-only objects remain valid
under the new schema.
