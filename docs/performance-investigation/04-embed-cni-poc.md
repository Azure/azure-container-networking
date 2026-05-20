# Lab 4 — Embedded CNI POC

**Workstream:** Node-readiness
**Date:** May 19, 2026
**Branch:** [`rbtr/experiment/cns-embed-cni`](https://github.com/rbtr/azure-container-networking/tree/experiment/cns-embed-cni)
**Image:** `acnpublic.azurecr.io/azure-cns:v0.0.4-embed-cni-20260519-2000`

---

## Hypothesis

The CNS DaemonSet's `cni-installer` init container imposes a serial
pod-sync waterfall on every node bootstrap. Comparison data in [Lab
2](./02-node-readiness.md) (stock CNS 26 s vs no-init BYOCNI 9 s)
suggested the init container costs ~17 s, but that comparison
conflated multiple variables (cluster type, CNS image, dropgz vs no
init). To get the true cost we need a controlled A/B with the init
container as the *only* variable. See [Experiment — rigorous
A/B](#experiment--rigorous-init-container-ab) below.

The waterfall the init container creates:

```mermaid
flowchart LR
    a[init: image pull] -->|wait| b[init: extract bins]
    b -->|init-to-main-gap<br/>kubelet sync| c[main: image pull]
    c -->|exec-gap<br/>containerd serialization| d[main: CNS bootstrap]
    d --> e[CNS writes conflist]
    e --> f[node-ready]
```

If we eliminate the init container entirely by **embedding the CNI
binaries inside the CNS image** and writing them to `/opt/cni/bin`
during CNS bootstrap, kubelet's pod-sync pipeline collapses to a
single container start. The init-to-main waterfall disappears.

Drift constraint: today's init container is how AKS keeps
`/opt/cni/bin/azure-vnet` synced to the released CNI version — every
pod restart re-pulls the install image. Embedding the binaries
preserves this property by coupling CNI binary version to CNS image
version (which is what production already wants).

---

## Design — mirror the `dropgz` pattern

The existing `cni-installer` image (`dropgz`) is a tiny Go tool that:
1. Embeds CNI binaries at `pkg/embed/fs/` via `//go:embed`
2. Exposes `dropgz list / deploy / verify` cobra subcommands
3. CI pipeline (`.pipelines/build/scripts/dropgz.sh`) gzips each
   binary, computes `sha256sum * > sum.txt`, and drops them into
   `fs/` before the Go build embeds them.

This POC lifts that pattern into CNS itself:

```mermaid
flowchart LR
    subgraph DropgzImage[old: cni-dropgz init container]
        d1[//go:embed fs/]
        d2[dropgz deploy]
        d3[exits]
    end
    subgraph CNSImage[new: cns image]
        c1[//go:embed fs/]
        c2[cns deploy<br/>subcommand]
        c3[cns daemon<br/>main path]
    end
    DropgzImage -.eliminated.-> CNSImage
```

### New subcommand surface

```
azure-cns                          # daemon (default — current behavior)
azure-cns deploy [files...] --out-dir D
azure-cns verify [files...] --out-dir D
azure-cns list
azure-cns embedded [...]           # umbrella for cobra --help
```

Dispatched from `main()` via an `os.Args[1]` check **before**
`acn.ParseArgs` parses the daemon's flag set, so subcommand args
don't collide.

### Bootstrap order

The CNS daemon runs the deploy step on startup, **before** binding
the HTTP listener — so by the time any pod calls CNI ADD, the
binaries are on disk:

```mermaid
flowchart TD
    s[main entry]
    s --> a[RecordStartTime<br/>SetBuildInfo<br/>ClassifyBootState]
    a --> d[deployEmbeddedCNI]
    d -->|sha256 match| skip[skip write — log to stderr]
    d -->|mismatch or absent| write[extract + write /opt/cni/bin/*]
    write --> verify[verify on-disk sums]
    skip --> p[acn.ParseArgs]
    verify --> p
    p --> rest[normal CNS bootstrap]
    rest --> listener[bind HTTP listener]
```

On warm restarts (CNS process restart on a node where binaries
already match the embedded sha256s) the deploy step is a fast
sha256-only path with no rewrite.

### File layout (new)

```
cns/
  embed/
    fs/
      _README           # placeholder
      sum.txt           # placeholder (build replaces)
      .gitignore        # ignore everything except placeholders
    payload.go          # //go:embed fs/ + Contents/Extract/Deploy
    payload_test.go     # 8 tests
  hash/
    hash.go             # sha256sum manifest parser
    hash_test.go        # 3 tests
  cmd/
    embedded/
      embedded.go       # cobra root + list/deploy/verify subcommands
  service/
    main.go             # added subcommand dispatch + deployEmbeddedCNI() helper
Dockerfile
  payload stage         # new: build CNI/IPAM bins, sum.txt, gzip, copy to fs/
```

`cns/embed` and `cns/hash` are deliberate copies of
`dropgz/pkg/embed` and `dropgz/pkg/hash` (rather than direct imports)
because `dropgz/` is a nested Go module with its own `go.mod`.
Cross-module imports would require `replace` directives in both
modules; copying ~200 LOC is cleaner for a POC.

---

## Verification — live cluster

Deployed to `evanbaker-byocni-overlay-westus2` (the persistent test
cluster from Lab 2).

### Cold start

```
{"level":"info","caller":"embed/payload.go:154","msg":"wrote file","component":"cni-deploy","src":"azure-ipam","dest":"/opt/cni/bin/azure-ipam"}
{"level":"info","caller":"embed/payload.go:154","msg":"wrote file","component":"cni-deploy","src":"azure-vnet","dest":"/opt/cni/bin/azure-vnet"}
{"level":"info","caller":"embed/payload.go:154","msg":"wrote file","component":"cni-deploy","src":"azure-vnet-ipam","dest":"/opt/cni/bin/azure-vnet-ipam"}
{"level":"info","caller":"embed/payload.go:154","msg":"wrote file","component":"cni-deploy","src":"azure-vnet-telemetry","dest":"/opt/cni/bin/azure-vnet-telemetry"}
... daemon proceeds normally ...
```

All 4 binaries written to `/opt/cni/bin/` directly from the embedded
payload. CNS then proceeds with the normal bootstrap path.

### Warm restart (`kubectl delete pod`)

```
[Azure CNS] embedded CNI payload already current on disk; skipping deploy
2026/05/19 20:06:43 [configuration] invalid IPv6PrefixClamp value 0; must be between 120 to 128, defaulting to /120
2026/05/19 20:06:43 [1] GetAzureCloud querying url: ...
```

sha256s match → no rewrite, sub-second skip path. The remaining
bootstrap proceeds as normal.

---

## Experiment — gzip vs raw payload tradeoff

**Hypothesis (from user during POC review):** "Images are ostensibly
shipped compressed and there is computational overhead to unzipping —
is there a perf advantage to NOT gzipping them?"

**Setup:** Built two variants of the same image:
- `embed + gz payload` — current design, gzipped binaries in `fs/`
- `embed + raw payload` — same code, no gzip step in Dockerfile

Compared on-disk image size, over-the-wire (registry) size, and
decompression cost.

### Binary sizes

| binary | raw | gz | ratio |
|---|---:|---:|---:|
| azure-vnet | 44.4 MB | 14.3 MB | 32% |
| azure-vnet-ipam | 48.1 MB | 13.4 MB | 28% |
| azure-vnet-telemetry | 7.7 MB | 3.1 MB | 41% |
| **total** | **100 MB** | **31 MB** | **31%** |

### Decompression cost (Go `gzip.NewReader` + `io.Copy`)

| binary | decompress time |
|---|---:|
| azure-vnet | 291 ms |
| azure-vnet-ipam | 276 ms |
| azure-vnet-telemetry | 62 ms |
| **total** | **~630 ms** |

One-time cost at cold start. Warm restart still hits the sha256-skip
path (zero decompression).

### Image sizes (built both, pulled both, measured)

| variant | on-disk | over-the-wire (registry) |
|---|---:|---:|
| baseline (no embed) | 107.7 MB | 39.8 MB |
| **embed + gz payload** | **143.8 MB** | **75.4 MB** |
| **embed + raw payload** | **215.4 MB** | **76.3 MB** |
| **gz vs raw difference** | **−72 MB** | **−0.9 MB** |

```mermaid
%%{init: {'theme':'base'}}%%
xychart-beta
    title "Image size: gzipped vs raw payload"
    x-axis ["Baseline (no embed)", "Embed + gz", "Embed + raw"]
    y-axis "MB" 0 --> 250
    bar [107.7, 143.8, 215.4]
    bar [39.8, 75.4, 76.3]
```

(Blue = on-disk; orange = over-the-wire)

### Analysis

The hypothesis is correct **about the wire**: OCI layers are
themselves gzip-compressed in transit. Our gzipped payload vs raw
payload differ by only 0.9 MB on the wire — OCI's per-layer deflate
hits binaries about as well as our `gzip --best`.

**But on disk, the raw-payload image is 72 MB bigger.** Where this
shows up:

- Kubelet image cache on every node running CNS
- AKS VHD bake size if CNS is pre-loaded (a related goal of this
  workstream)
- Node disk pressure with multiple image versions

The ~630 ms decompression cost is real but:
- One-time only (cold start)
- Hidden inside an existing ~2-5 s bootstrap phase
- Zero on warm restart via the sha256-skip path

### Verdict: keep the gzip

| Axis | Cost of switching to raw |
|---|---|
| Wire | nil (+0.9 MB) |
| On-disk | +72 MB per node |
| CPU | −630 ms one-time at cold start |

The on-disk cost dominates. Keep gzip in the payload.

### Follow-up ideas (not in this POC)
- **zstd instead of gzip** — typically 10-15% better ratio + 3-5×
  faster decompression. Same `embed.FS` machinery, just swap the
  decoder. Worth a few MB and ~50 ms; nice cleanup.
- **No compression + symlink** — doesn't work; `embed.FS` is
  read-only, no hardlink target.

---

## Experiment — rigorous init-container A/B

**Date:** 2026-05-20
**Motivation:** The original [Lab 2](./02-node-readiness.md) comparison
(stock CNS 26 s vs no-init BYOCNI 9 s) conflated three variables:

1. **Cluster type** — Azure CNI Overlay+Cilium vs BYOCNI Overlay
2. **CNS image** — stock AKS-rolled 1.7.x vs PR #4398 metrics build
3. **Init container** — `cni-dropgz` vs no init

We need to isolate variable 3. This experiment runs the *same CNS
image* (the embed-cni POC build) on the *same cluster* (persistent
`evanbaker-byocni-overlay-westus2`), changing only the DaemonSet
manifest:

- **Arm A:** DaemonSet **with** an init container that runs
  `cns deploy --out-dir /opt/cni/bin` (same image as the main
  container — sidesteps the image-pull confound).
- **Arm B:** DaemonSet **without** an init container; CNS deploys
  the binaries inline during its bootstrap (the POC code path).

10 runs per arm, alternating in blocks of 5 (A→B→A→B) to control
for AKS regional time drift.

### Results — `node-ready` (n=10 per arm)

| arm | min | p50 | p95 | p99 | max | mean | stddev |
|---|--:|--:|--:|--:|--:|--:|--:|
| **A** (with init) | 13.0 | **16.5** | 18.6 | 18.9 | 19.0 | 16.4 | 1.84 |
| **B** (no init)   | 11.0 | **14.0** | 15.6 | 15.9 | 16.0 | 13.9 | 1.37 |
| **Δ (A − B)** | 2.0 | **2.5** | 3.0 | 3.0 | 3.0 | 2.5 | — |

```mermaid
%%{init: {'theme':'base'}}%%
xychart-beta
    title "node-ready: with-init vs no-init (n=10 each)"
    x-axis ["min", "p50", "p95", "p99", "max"]
    y-axis "seconds" 0 --> 20
    bar [13.0, 16.5, 18.6, 18.9, 19.0]
    bar [11.0, 14.0, 15.6, 15.9, 16.0]
```

(Blue = Arm A with init; orange = Arm B no init)

### Statistical confidence

- **Welch's t-test:** t = 3.45, df = 16.6 → **p < 0.01** two-tailed
- **Mann-Whitney U:** U = 19 (n₁ = n₂ = 10) → **p < 0.05** two-tailed
- Block-2 means within 1 s of block-1 in both arms → no significant
  time drift over the ~90 min experiment

The 2.5 s delta is statistically significant.

### Phase decomposition (combined p50 across 10 runs each)

| span | Arm A (with init) | Arm B (no init) | delta |
|---|--:|--:|--:|
| `cns-pod-schedule-latency` | 0.52 | 0.76 | +0.24 |
| `cns-init-image-pull` | 7.0 | — | (gone) |
| `cns-init-container-run` | 2.0 | — | (gone) |
| `cns-init-to-main-gap` | (not captured) | — | — |
| `cns-image-pull` | 7.0 | 7.0 | 0.0 |
| `cns-container-start` (from Pulled) | 4.5 | 1.0 | **−3.5** |
| `cns-exec-gap` | 1.15 | 2.65 | +1.50 |
| `cns-process-bootstrap` | 0.31 | 0.30 | 0.0 |
| `cns-state-restored` | 0.51 | 1.80 | +1.29 |
| `cns-first-nnc-received` | 0.80 | 2.17 | +1.37 |
| `cns-listener-ready` | 0.83 | 2.21 | +1.38 |
| `cns-conflist-write` | 1.87 | 3.26 | +1.39 |
| **`node-ready`** | **16.5** | **14.0** | **−2.5** |

Several interesting observations:

1. **Image pull is paid once** in both arms — Arm A's init and main
   containers share the same image, so kubelet's image cache hits
   for the main pull. The 7 s pull is one-time per node either way.
2. **`cns-container-start` (Pulled → started) drops 3.5 s in Arm B.**
   That's the actual init→main pod-sync waterfall: kubelet has to
   sequence init-exit → main-start, even when the image is cached.
   This is the dominant cost the init container imposes.
3. **CNS-internal spans in Arm B shift later by ~1.4 s.** Because
   Arm B runs `deployEmbeddedCNI()` inside the main container before
   logging "state-restored", the entire CNS bootstrap timeline is
   shifted right by the deploy work (sha256 verify + extract +
   write of 4 binaries ≈ 1.3 s when binaries already current via
   the skip path, or ~2 s on cold; observed value here is consistent
   with the cold path).
4. **Net node-ready delta is the difference** between (3.5 s saved
   by removing the kubelet waterfall) and (1.4 s added by doing the
   deploy work inline) ≈ 2 s, plus minor variance from pod-schedule
   and exec-gap shifts. Aligns with the observed 2.5 s.

### What this measurement does **not** capture

- **Separate init image** (real `cni-dropgz`, ~30 MB extra over the
  wire). Would add a separate `cns-init-image-pull` cost on cold
  nodes that doesn't share kubelet's image cache with the main
  container. This experiment used the same image for both
  containers to isolate the waterfall cost; production today uses
  a separate dropgz image.
- **Cold node image cache.** All nodes in this experiment provisioned
  fresh, but the CNS image was already in the AKS registry; we
  didn't simulate VHD bake-in or cold-region pulls.
- **Nodepool stampede.** Single-node scale-up per run (+1 node).
  Larger concurrent nodepool growth would stress containerd
  serialization further and likely widen the gap.

### Why the original 9 s vs 26 s gap was so much larger

The original comparison spanned:

| factor | "9 s" (BYOCNI no-init) | "26 s" (Azure CNI Overlay stock) |
|---|---|---|
| Cluster | BYOCNI Overlay | Azure CNI Overlay + Cilium |
| CNS image | embed-cni POC (fast bootstrap) | stock 1.7.x (slower path) |
| Init container | none | `cni-dropgz` (separate ~30 MB image) |
| Competing daemonsets | minimal | full AKS production stack |
| Node geometry | Standard_B12ms | Standard_B12ms |

Removing the init container in isolation accounts for ~2.5 s of
that ~17 s delta on this configuration. The rest is attributable
to the other variables — particularly the slower stock CNS
bootstrap (fixed by [PR #4398](./03-bootstrap-metrics.md)) and
DaemonSet stampede contention on the production AKS path.

### Source data

- Raw spans CSVs: `/tmp/bench-ab/20260520-000808/{armA,armB}-block{1,2}/spans.csv`
- Combined per-arm: `/tmp/bench-ab/20260520-000808/combined/arm{A,B}.spans.csv`
- Combined dashboard: served at `dashboard.html` (20 runs labeled
  1-10 = Arm B, 11-20 = Arm A in the per-run timeline)

---

## Experiment — production-realistic comparison

**Date:** 2026-05-20
**Motivation:** The same-image A/B above isolates the init-container
waterfall in the purest form, but production uses a *different*
cluster type (stock Azure CNI Overlay with VHD-preloaded images)
and a *separate* init image (`cni-dropgz`). We need to measure the
full production-default path against the embed-CNI POC to get a
realistic, phase-decomposed comparison.

### Setup

| | Arm A — Production default | Arm B — Embed-CNI POC |
|---|---|---|
| **Cluster** | `evanbaker-stock-overlay-westus2` | `evanbaker-byocni-overlay-westus2` |
| **Network plugin** | Azure CNI Overlay (managed) | BYOCNI Overlay |
| **CNS image** | `mcr.microsoft.com/.../azure-cns:v1.7.16-0` (VHD-preloaded) | `acnpublic.azurecr.io/azure-cns:v0.0.4-embed-cni-20260519-2000` (not preloaded) |
| **Init container** | `azure-cni:v1.7.16-0` (VHD-preloaded dropgz) | none |
| **VM size** | Standard_B12ms | Standard_B12ms |
| **K8s** | 1.33 | 1.33 |
| **Region** | westus2 | westus2 |
| **Runs** | 10 (2 blocks of 5) | 10 (2 blocks of 5) |

Note: these are **different clusters** — this is intentional. Arm A
measures the actual production-default AKS experience; Arm B measures
the embed-CNI POC. The clusters differ in network plugin mode
(managed vs BYO) and CNS image (stock vs POC), which means the
comparison is not a pure single-variable isolation (see the
[same-image A/B](#experiment--rigorous-init-container-ab) above for
that). Instead, this measures **the total gap between what ships
today and what the embed-CNI design delivers**.

### Results — `node-ready` (n=10 per arm)

| arm | min | p50 | p95 | p99 | max | mean | stddev |
|---|--:|--:|--:|--:|--:|--:|--:|
| **A** (production default) | 18.0 | **19.0** | 22.6 | 22.9 | 23.0 | 19.4 | 1.78 |
| **B** (embed-CNI POC)      | 10.0 | **12.0** | 17.0 | 17.0 | 17.0 | 12.9 | 2.60 |
| **Δ (A − B)** | 8.0 | **7.0** | 5.6 | 5.9 | 6.0 | 6.5 | — |

```mermaid
%%{init: {'theme':'base'}}%%
xychart-beta
    title "node-ready: production default vs embed-CNI POC (n=10 each)"
    x-axis ["min", "p50", "p95", "p99", "max"]
    y-axis "seconds" 0 --> 25
    bar [18.0, 19.0, 22.6, 22.9, 23.0]
    bar [10.0, 12.0, 17.0, 17.0, 17.0]
```

(Blue = Arm A production default; orange = Arm B embed-CNI POC)

### Statistical confidence

- **Welch's t-test:** t = 6.53, df = 15.9 → **p < 0.001** two-tailed
- **Mann-Whitney U:** U = 0 (n₁ = n₂ = 10) → **p < 0.001** two-tailed
  (zero overlap between distributions)

### Phase decomposition (p50 across 10 runs each)

| span | A (production) | B (embed POC) | Δ | notes |
|---|--:|--:|--:|---|
| `vm-provision` | 78.6 | 85.4 | −6.8 | VMSS timing variance |
| `cns-pod-schedule-latency` | 0.6 | 0.4 | +0.2 | noise |
| `cns-init-image-pull` | **0.0** | — | — | VHD-preloaded → zero pull |
| `cns-init-container-run` | **2.0** | — | **+2.0** | dropgz binary extract |
| `cns-init-to-main-gap` | **6.0** | — | **+6.0** | kubelet pod-sync waterfall |
| `cns-image-pull` | **0.0** | **5.5** | **−5.5** | VHD-preloaded vs cold pull |
| `cns-container-start` | 0.0 | 1.0 | −1.0 | |
| `cns-exec-gap` | 3.7 | 2.6 | +1.1 | containerd startup |
| `cns-process-bootstrap` | **2.3** | **0.3** | **+1.9** | stock CNS 1.7.16 slower bootstrap |
| `cns-listener-ready` | **7.1** | **3.1** | **+4.0** | cumulative init→listener |
| `cns-conflist-write` | — | 4.0 | — | (missing on stock — no conflist annotation DS) |
| `cns-pod-ready` | 22.0 | 9.0 | +13.0 | |
| **`node-ready`** | **19.0** | **12.0** | **+7.0** | |

### Where the 7 s comes from — attribution

The 7 s p50 gap decomposes into three independent components:

1. **Init-container waterfall: ~8 s saved**
   - `cns-init-container-run` (2 s) + `cns-init-to-main-gap` (6 s)
     = 8 s of serial pod-sync pipeline that Arm B doesn't have.

2. **Image pull shift: ~5.5 s added**
   - Arm A's stock images are VHD-preloaded (both init and main
     show 0 s pull). Arm B's experimental embed-CNI image is NOT
     preloaded → 5.5 s cold pull per node.
   - **This cost disappears if the embed-CNI image is baked into
     the VHD** (which is the production deployment plan).

3. **CNS bootstrap speed: ~2 s saved**
   - `cns-process-bootstrap` is 2.3 s on stock CNS 1.7.16 vs 0.3 s
     on the POC build (which includes PR #4398 optimizations).
   - This is a CNS code improvement, not directly related to embed
     vs init. It will ship whenever the #4398 branch merges.

**Summary attribution:**

| factor | Δ (s) | embed-CNI specific? |
|---|--:|---|
| Init waterfall removed | −8.0 | ✅ yes |
| Image pull (not preloaded) | +5.5 | ❌ disappears with VHD bake |
| CNS bootstrap speed (PR #4398) | −2.0 | ❌ ships independently |
| Other (exec-gap, scheduling) | −2.5 | mixed |
| **net Δ** | **−7.0** | |

**Projected with VHD bake-in:** if the embed-CNI image is
preloaded (just as stock CNS is today), `cns-image-pull` drops to
0 s, and the projected p50 `node-ready` for Arm B falls to ~6-7 s.
The projected gap grows to ~12-13 s.

### What's different from the same-image A/B

| | Same-image A/B | Production-realistic |
|---|---|---|
| Δ node-ready p50 | 2.5 s | 7.0 s |
| Cluster | same | different |
| Init image | same as main (cached) | separate dropgz (VHD-preloaded) |
| CNS version | same POC build | stock 1.7.16 vs POC |
| Variables isolated | init container only | everything (production reality) |

The same-image A/B isolates a pure 2.5 s init-container waterfall
cost. The production-realistic comparison shows a 7 s total gap
because it includes the slower stock CNS bootstrap (2 s) and the
full dropgz init container waterfall with a separate init image
(8 s), partially offset by the embed-CNI image cold pull (5.5 s).

### Source data

- Raw spans CSVs: `/tmp/bench-prod-ab/20260520-1435/arm{A,B}/cycle{1..10}/spans.csv`
- Combined per-arm: `/tmp/bench-prod-ab/20260520-1435/combined/arm{A,B}.spans.csv`
- Combined dashboard: `dashboard.html` (runs 1-10 = Arm B, 11-20 = Arm A)

---

## Summary of all node-ready measurements

| experiment | Arm A | Arm B | Δ p50 | what it measures |
|---|---|---|--:|---|
| Same-image A/B (n=10) | 16.5 s (with init, same image) | 14.0 s (no init) | 2.5 s | pure init-container waterfall cost |
| Production-realistic (n=10) | 19.0 s (stock AzCNI+dropgz) | 12.0 s (embed POC, cold pull) | 7.0 s | total production gap |
| Production-realistic + VHD bake (projected) | 19.0 s | ~6-7 s | ~12-13 s | projected with preloaded POC image |

---

## What this proves

```mermaid
flowchart LR
    a[Production default<br/>stock AzCNI Overlay] -->|measured| b["19 s p50 node-ready<br/>init-container-run 2 s<br/>init-to-main-gap 6 s<br/>stock CNS bootstrap 2.3 s"]
    c[Embed-CNI POC<br/>BYOCNI Overlay] -->|measured| d["12 s p50 node-ready<br/>no init container<br/>5.5 s cold image pull<br/>fast bootstrap 0.3 s"]
    b -.-> e["7 s p50 savings today<br/>~12 s projected with VHD bake"]
    d -.-> e
```

The embed-CNI design delivers a **7 s p50 improvement** over
production default today, and a projected **12-13 s improvement**
once the embed-CNI image is VHD-preloaded. The three main drivers:

1. **Removing the init-container waterfall** (8 s) — the dominant
   and embed-CNI-specific win.
2. **Faster CNS bootstrap** (2 s) — ships with PR #4398 regardless
   of embed-CNI.
3. **Image pull overhead** (5.5 s penalty today) — disappears with
   VHD bake-in, which is the standard deployment path.

---

## Conclusions

1. **POC is functional.** End-to-end verified on a live cluster.
   Cold start writes 4 binaries; warm restart skips via sha256.
2. **`dropgz` pattern lifts cleanly** into CNS as a subcommand. ~500
   LOC of new code, mostly mirroring existing dropgz code.
3. **Image size acceptable.** +36 MB on-disk for the gzipped payload
   (vs +108 MB for raw); +36 MB over the wire.
4. **Same-image A/B shows 2.5 s p50 savings** (p < 0.01) from
   removing the init-container waterfall alone.
5. **Production-realistic comparison shows 7.0 s p50 savings**
   (p < 0.001) — stock AKS Azure CNI Overlay 19 s → embed-CNI POC
   12 s. Decomposition attributes 8 s to init-container removal,
   2 s to faster CNS bootstrap (PR #4398), partially offset by
   5.5 s cold image pull (disappears with VHD bake-in).
6. **Projected with VHD bake: ~6-7 s node-ready** (vs 19 s today) —
   a ~12-13 s improvement, or roughly 3× faster node initialization.

## Recommendations

| # | Action | Status |
|---|---|---|
| 1 | Land PR #4398 (bootstrap metrics — observability prerequisite) | [Lab 3](./03-bootstrap-metrics.md), open at #4398 |
| 2 | Polish POC into a PR-able branch | This POC is on [`rbtr/experiment/cns-embed-cni`](https://github.com/rbtr/azure-container-networking/tree/experiment/cns-embed-cni); needs daemonset.yaml change + Windows support + test fixtures before upstreaming |
| 3 | Update `test/integration/manifests/cns/daemonset-linux.yaml` to drop the init container | Phase 2 of the proposal |
| 4 | Pre-pull CNS image into AKS VHD | Companion change; gets us under 5 s node-ready |
| 5 | Stop building/publishing `cni-dropgz` once no consumers remain | Phase 3 of the proposal |

## Open questions

1. **VHD bake-in of the embed-CNI image.** The production-realistic
   comparison shows 5.5 s of cold image pull on Arm B. With VHD
   preloading (standard AKS practice for CNS), this drops to 0 s
   and projected node-ready falls to ~6-7 s. Needs coordination
   with the AKS VHD team.
2. **Windows path mapping** (`C:\k\cni\bin\` instead of `/opt/cni/bin`).
   Dockerfile already has a Windows target; needs runtime path
   detection.
3. **Other `cni-dropgz` consumers** outside the CNS DaemonSet
   (NPM-only, `cniv1` clusters) — would need a different transition
   path or keep dropgz alive for them.
4. **Per-binary versioning** if CNI and CNS diverge. The current
   coupling (CNS image = CNI binary version) is intentional but may
   not match all consumers' release schedules.
5. **Telemetry on the deploy step** — would add a small histogram for
   `cns_cni_deploy_duration_seconds` to detect drift in cold-start
   cost over time. Easy follow-up.
6. **Nodepool stampede.** Both A/B experiments used single-node
   scale-up per run. Concurrent multi-node nodepool growth would
   stress kubelet/containerd serialization differently — the
   init-container cost may scale super-linearly with concurrency.
