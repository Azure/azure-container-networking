# AKS Container Networking Performance Investigation

A multi-month effort to characterize and improve pod and node startup
latency in AKS clusters running Azure CNI. This directory consolidates
the experimental record across two distinct workstreams.

## Reading order

1. **[Executive Summary](./00-executive-summary.md)** — high-level
   findings, trends across all experiments, and current
   recommendations. **Start here.**
2. **[Lab 1 — Pod startup latency](./01-pod-slo.md)** — store
   backends, RTNL contention, and the kernel SLI floor.
3. **[Lab 2 — Node readiness](./02-node-readiness.md)** — phase
   decomposition of node-init, the static-pod blocker, and the
   nodeinit-bench tooling.
4. **[Lab 3 — CNS bootstrap metrics](./03-bootstrap-metrics.md)** —
   PR #4398: 16 Prometheus metrics that replaced log-parsing as the
   primary observability path.
5. **[Lab 4 — Embedded CNI POC](./04-embed-cni-poc.md)** — embedding
   the CNI installer in the CNS image as a `cns deploy` subcommand,
   plus a rigorous A/B measurement of the init-container cost
   (2.5 s p50 on a controlled comparison).

Each lab writeup follows the same structure: **hypothesis →
experiment → data → conclusion**, with tables and (where
appropriate) Mermaid charts for visualization.

## Source data

- Source branches on the `rbtr` fork:
  - [`experiment/pod-slo`](https://github.com/rbtr/azure-container-networking/tree/experiment/pod-slo) — pod-SLO workstream index
  - [`experiment/node-readiness`](https://github.com/rbtr/azure-container-networking/tree/experiment/node-readiness) — node-readiness workstream + `tools/nodeinit-bench`
  - [`feat/bolt-store`](https://github.com/rbtr/azure-container-networking/tree/feat/bolt-store) — per-record BoltDB implementation
  - [`performance-research`](https://github.com/rbtr/azure-container-networking/tree/performance-research) — RTNL mitigations + cluster bench harness
  - [`experiment/cns-embed-cni`](https://github.com/rbtr/azure-container-networking/tree/experiment/cns-embed-cni) — embedded CNI POC
- Upstream PRs:
  - **[Azure PR #4398](https://github.com/Azure/azure-container-networking/pull/4398)** — `feat/cns-bootstrap-metrics` (open)
- Tooling:
  - `tools/nodeinit-bench/` on `experiment/node-readiness` — the
    measurement CLI used for all node-init data here.

## Runbook — reproducing experiments from scratch

A fresh operator (human or agent) can reproduce any experiment in
this directory by following these steps. All commands assume you are
in the repo root.

### Prerequisites

- `az` CLI logged in, with the `aks-preview` extension
- `kubectl` on PATH
- `docker` (or `podman`) for image builds
- Go toolchain (for building `nodeinit-bench`)
- Access to `acnpublic.azurecr.io` for pushing experimental images
- A subscription with quota for AKS clusters + VMSS

### Step 1 — Build `nodeinit-bench`

The bench tool lives on the `experiment/node-readiness` branch:

```bash
git fetch rbtr experiment/node-readiness
git checkout rbtr/experiment/node-readiness -- tools/nodeinit-bench

cd tools/nodeinit-bench
go build -o /tmp/bin/nodeinit-bench .
```

Verify: `/tmp/bin/nodeinit-bench --help` should show `run` and
`render` subcommands.

### Step 2 — Create test clusters

Use `hack/aks/Makefile` from the repo root. Two cluster types are
needed for the embed-CNI comparison:

**Arm A — Stock Azure CNI Overlay (production default):**

```bash
make -C hack/aks overlay-up \
  CLUSTER=<user>-stock-overlay-westus2 \
  GROUP=<user>-stock-overlay-westus2 \
  REGION=westus2 \
  VM_SIZE=Standard_B12ms \
  K8S_VER=1.33 \
  NODE_COUNT=2
```

This creates a managed AKS cluster with Azure CNI Overlay, including
the stock `azure-cns` DaemonSet with `cni-dropgz` init container.
Both images are VHD-preloaded.

**Arm B — BYOCNI Overlay (for embed-CNI POC):**

```bash
make -C hack/aks overlay-byocni-up \
  CLUSTER=<user>-byocni-overlay-westus2 \
  GROUP=<user>-byocni-overlay-westus2 \
  REGION=westus2 \
  VM_SIZE=Standard_B12ms \
  K8S_VER=1.33 \
  NODE_COUNT=2
```

BYOCNI clusters have DNC/RC and create NNCs, but no CNI or CNS is
installed — you deploy your own.

**Isolate kubeconfigs** (don't pollute `~/.kube/config`):

```bash
mkdir -p /tmp/kubeconfigs
az aks get-credentials -n <cluster> -g <rg> --file /tmp/kubeconfigs/<cluster>.yaml
export KUBECONFIG=/tmp/kubeconfigs/<cluster>.yaml
```

### Step 3 — Deploy CNS on the BYOCNI cluster

The BYOCNI cluster needs CNS deployed manually. Manifests are in
`test-staticpod/`:

```bash
export KUBECONFIG=/tmp/kubeconfigs/<user>-byocni-overlay-westus2.yaml

# Apply RBAC + config
kubectl apply -f test-staticpod/clusterrole.yaml
kubectl apply -f test-staticpod/rolebinding.yaml
kubectl apply -f test-staticpod/overlayconfigmap.yaml

# Apply the DaemonSet (edit image tag first if needed)
kubectl apply -f test-staticpod/cns-daemonset-A-pre.yaml
```

For the **embed-CNI image**, build and push from the
`experiment/cns-embed-cni` branch:

```bash
git checkout rbtr/experiment/cns-embed-cni

# Build the image (includes embedded CNI binaries)
docker build -t acnpublic.azurecr.io/azure-cns:<tag> \
  -f cns/Dockerfile .
az acr login -n acnpublic
docker push acnpublic.azurecr.io/azure-cns:<tag>

# Update the DaemonSet image
kubectl set image ds/azure-cns -n kube-system \
  cns-container=acnpublic.azurecr.io/azure-cns:<tag>
```

To run **without** an init container (the embed-CNI arm), remove
the `initContainers` section from the DaemonSet manifest, or
patch it:

```bash
kubectl patch ds azure-cns -n kube-system --type json \
  -p '[{"op":"remove","path":"/spec/template/spec/initContainers"}]'
```

### Step 4 — Deploy the conflist-mtime DaemonSet

This is required for the `cns-conflist-write` and
`kubelet-cni-pickup` spans. Deploy on **both** clusters:

```bash
kubectl apply -f tools/nodeinit-bench/deploy/conflist-mtime-daemonset.yaml
```

### Step 5 — Run experiments

**Single run:**

```bash
/tmp/bin/nodeinit-bench run \
  --cluster <cluster-name> \
  --resource-group <rg> \
  --nodepool nodepool1 \
  --runs 1 \
  --delta 1 \
  --cleanup \
  --out /tmp/results/<arm>/<run-id> \
  --kubeconfig /tmp/kubeconfigs/<cluster>.yaml
```

For stock clusters (no PR #4398 metrics), add `--skip-metrics`.

**A/B experiment (10 runs per arm, alternating blocks of 5):**

Run one cycle at a time with explicit waits between, because AKS
nodepool operations race if you don't let the previous op complete:

```bash
for cycle in 1 2 3 4 5; do
  # Wait for nodepool to be Succeeded + 2 nodes
  while true; do
    state=$(az aks nodepool show -n nodepool1 -g <rg> \
      --cluster-name <cluster> --query provisioningState -o tsv)
    nodes=$(kubectl get nodes --no-headers | wc -l)
    [ "$state" = "Succeeded" ] && [ "$nodes" = "2" ] && break
    sleep 20
  done

  # Run
  /tmp/bin/nodeinit-bench run \
    --cluster <cluster> --resource-group <rg> \
    --nodepool nodepool1 --runs 1 --delta 1 \
    --skip-metrics --out /tmp/results/<arm>/cycle${cycle} \
    --kubeconfig /tmp/kubeconfigs/<cluster>.yaml

  # Scale back
  az aks nodepool scale -n nodepool1 -g <rg> \
    --cluster-name <cluster> --node-count 2 --no-wait
done
```

Alternate: run Arm A block 1 (5 cycles) → Arm B block 1 (5 cycles)
→ Arm A block 2 → Arm B block 2 to control for time drift.

### Step 6 — Combine and render results

```bash
# Combine per-arm cycle CSVs (renumber run IDs)
python3 -c "
import csv, glob, os
for arm in ['A', 'B']:
    dirs = sorted(glob.glob(f'/tmp/results/arm{arm}/cycle*/spans.csv'))
    header = None; rows = []
    for rid, path in enumerate(dirs, 1):
        with open(path) as f:
            r = csv.reader(f); h = next(r)
            if not header: header = h
            for row in r: row[0] = str(rid); rows.append(row)
    with open(f'/tmp/results/arm{arm}.spans.csv', 'w') as f:
        w = csv.writer(f); w.writerow(header); w.writerows(rows)
"

# Render per-arm dashboards
for arm in A B; do
  mkdir -p /tmp/results/src${arm} /tmp/results/render-${arm}
  cp /tmp/results/arm${arm}.spans.csv /tmp/results/src${arm}/spans.csv
  /tmp/bin/nodeinit-bench render \
    --out /tmp/results/render-${arm} /tmp/results/src${arm}
done
```

The `summary.md` in each render directory has per-span percentiles.
The `dashboard.html` is a self-contained interactive Plotly page.

### Step 7 — Statistical analysis

With n=10 per arm, compute Welch's t-test and Mann-Whitney U on
the `node-ready` durations from each arm's combined `spans.csv`.
See [Lab 4 — statistical confidence](./04-embed-cni-poc.md#statistical-confidence)
for the Python snippet.

### Gotchas / common issues

- **`OperationNotAllowed` on scale:** AKS nodepool operations are
  serialized. Wait for `provisioningState=Succeeded` before the
  next scale. Post-create extension installs can take 2-5 min.
- **Event GC:** Kubernetes GCs events after ~1 hour. If you need
  raw event evidence (e.g., the init-to-main gap timeline), capture
  events immediately after each run.
- **BYOCNI clusters are reusable.** Don't tear them down between
  runs — they can be reused indefinitely. The bench tool scales
  +1 node and observes the new node.
- **Stock clusters have VHD-preloaded images.** The `cns-image-pull`
  and `cns-init-image-pull` spans will be 0 s on stock. The embed-
  CNI image is NOT preloaded, so `cns-image-pull` will be 3-9 s.
- **`--skip-metrics` is required on stock CNS.** Stock CNS v1.7.x
  doesn't expose the PR #4398 Prometheus metrics; the bench will
  hang trying to scrape them if you don't skip.

---

## Conventions used in these docs

- All durations are **seconds** unless otherwise noted.
- Phase boundaries are anchored at `Node.metadata.creationTimestamp = T0`.
- "p50 / p95 / max" refer to percentiles across N observations
  unless explicitly labeled otherwise.
- "Cold start" = no prior CNS run on the node (no `/var/lib/azure-network`
  persistence). "Warm restart" = CNS pod restart on an existing node.
- "Stock CNS" = the Azure CNI Overlay (Cilium) DaemonSet deployed by AKS
  managed clusters. "BYOCNI" = `--network-plugin none --network-plugin-mode overlay`
  via `make -C hack/aks overlay-byocni-up`.
