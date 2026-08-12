# Transparent-tunnel CNI installer

Installs the `azure-vnet` CNI plugin in **transparent-tunnel** mode on every
Linux node. In this mode the plugin forces every pod-to-pod packet onto the
host's physical NIC so the Azure VFP layer enforces NSG / ASG rules even for
same-node flows. See `docs/transparent-tunnel.md` (in PR #4319) for the
datapath design.

## What gets installed

On each node, the DaemonSet's init container extracts files out of the
`mcr.microsoft.com/containernetworking/v2/azure-cni` image and writes them to
the host:

| Payload entry                       | Host path                                 |
|-------------------------------------|-------------------------------------------|
| `azure-vnet`                        | `/opt/cni/bin/azure-vnet`                 |
| `azure-vnet-ipam`                   | `/opt/cni/bin/azure-vnet-ipam`            |
| `azure-vnet-telemetry`              | `/opt/cni/bin/azure-vnet-telemetry`       |
| `azure-transparent-tunnel.conflist` | `/etc/cni/net.d/10-azure.conflist`        |

The conflist name `10-azure.conflist` is intentional &mdash; it replaces
whatever stock conflist is in `/etc/cni/net.d` so kubelet picks transparent-tunnel
unambiguously on the next pod sandbox creation.

## Prerequisites

- **An `azure-cni` image containing transparent-tunnel mode.** The mode was added
  in PR #4319; images built before that release do not recognize
  `"mode": "transparent-tunnel"` and silently fall back to bridge mode, giving no
  tunnel enforcement. The manifest therefore ships with an unresolvable placeholder
  tag that **must** be replaced before install:

  ```sh
  # pick the first release containing transparent-tunnel
  curl -s https://mcr.microsoft.com/v2/containernetworking/v2/azure-cni/tags/list

  sed -i 's|<TRANSPARENT_TUNNEL_RELEASE>|vX.Y.Z|' \
    hack/manifests/cni-installer-transparent-tunnel.yaml
  ```

  Images are published under the dalec path
  `mcr.microsoft.com/containernetworking/v2/azure-cni`; the older
  `containernetworking/azure-cni` path is deprecated and will not receive the mode.
- Linux nodes with `ipset` (userspace + `ip_set` / `xt_set` modules) and
  `iptables` installed. On Ubuntu / Debian: `sudo apt-get install -y ipset`.
- Pod-pool secondary IPs must **not** be assigned to the host primary NIC
  by anything other than `azure-vnet`. AKS handles this via CNS; on
  self-managed clusters using cloud-init or netplan to claim secondaries
  will short-circuit pod traffic via the host's `local` table.
- No legacy host bridge should keep pod traffic off the physical NIC.
  Transparent-tunnel needs pod packets to reach the physical NIC so VFP sees
  same-node traffic.

## Install

```sh
kubectl apply -f hack/manifests/cni-installer-transparent-tunnel.yaml
```

After every DaemonSet pod reaches `Running`, kill any leftover pods on
the node (or just wait &mdash; new pods will be scheduled with the new
conflist):

```sh
kubectl rollout status ds/azure-cni-transparent-tunnel -n kube-system
```

## Verify

On any worker:

```sh
ls -l /opt/cni/bin/azure-vnet
cat /etc/cni/net.d/10-azure.conflist           # must contain "mode": "transparent-tunnel"
sudo ipset list azure-tt-local-pods            # one entry per local pod
sudo iptables -t mangle -S PREROUTING | grep MARK | head
sudo iptables -t raw     -S PREROUTING | grep NOTRACK
sudo ip rule  show | grep '0x3 lookup 101'
sudo ip route show table 101
```

A correctly programmed host shows: 1 MARK rule per local pod veth (`mark 0x3`),
1 NOTRACK rule referencing `azure-tt-local-pods src,dst`, 1 `fwmark 0x3 lookup 101`
ip rule, and `default via <gw> dev eth0` in table 101.

## Uninstall

```sh
kubectl delete -f hack/manifests/cni-installer-transparent-tunnel.yaml
```

This stops shipping the conflist + binary to new nodes but does **not**
roll back existing shared kernel state (ipset, NOTRACK rule, ip rule, table
101). To restore a stock CNI on an existing node, drop a different conflist
into `/etc/cni/net.d` and reboot the node, or manually remove the shared
transparent-tunnel rules after all transparent-tunnel pods are gone.

## Image version

The DaemonSet ships with the unresolvable placeholder tag
`mcr.microsoft.com/containernetworking/v2/azure-cni:<TRANSPARENT_TUNNEL_RELEASE>`.
Replace it with whatever azure-cni release first includes the
`azure-transparent-tunnel.conflist` payload (added to `cni/Dockerfile`,
`cni/Dockerfile.tmpl` and `.pipelines/build/scripts/cni.sh` in PR #4319).

The placeholder is intentional: pinning a currently-published tag would install
an azure-vnet that does not know the mode, and it would fall back to bridge mode
without enforcing anything. A pull failure is the safer default.
