# AKS Migrate

A CLI toolkit for planning and executing migrations on Azure Kubernetes Service (AKS) clusters.

## Supported Migrations

### NPM to Cilium

Migrate from Azure NPM (Network Policy Manager) with iptables to Azure CNI powered by Cilium (eBPF).

Azure NPM is being retired (Windows: September 2026, Linux: September 2028). Migrating to Cilium introduces several behavioral breaking changes in network policy enforcement. This toolkit detects incompatibilities, translates policies, validates connectivity, and orchestrates the full migration.

## Commands

```
aksmigrate <command> [flags]
```

| Command | Description |
|---------|-------------|
| `aksmigrate audit` | Analyze NetworkPolicies for migration incompatibilities |
| `aksmigrate translate` | Patch policies and generate Cilium-compatible equivalents |
| `aksmigrate conntest snapshot` | Capture a connectivity baseline |
| `aksmigrate conntest validate` | Compare post-migration connectivity against a baseline |
| `aksmigrate conntest diff` | Offline comparison of two snapshots |
| `aksmigrate discover` | Find and prioritize clusters for migration across subscriptions |
| `aksmigrate migrate` | End-to-end 7-step migration for a single cluster |

## Quick Start

### Prerequisites

- Go 1.22+
- `kubectl` configured with cluster access (for live cluster operations)
- Azure CLI (`az`) 2.61+ (for `discover` and `migrate` commands)

### Build

```bash
go build -o aksmigrate ./cmd/aksmigrate
```

### Audit policies from YAML files (no cluster needed)

```bash
./aksmigrate audit --input-dir ./test/policies --k8s-version 1.29
```

### Audit policies from a live cluster

```bash
./aksmigrate audit --kubeconfig ~/.kube/config
```

### Translate policies

```bash
./aksmigrate translate --input-dir ./test/policies --output-dir ./cilium-patches
```

### Dry-run a full migration

```bash
./aksmigrate migrate \
  --cluster-name my-cluster \
  --resource-group my-rg \
  --dry-run
```

### Discover NPM clusters across a subscription

```bash
./aksmigrate discover --subscription <subscription-id>
```

### Connectivity validation

```bash
# Before migration
./aksmigrate conntest snapshot --phase pre-migration --output ./pre.json

# After migration
./aksmigrate conntest validate --pre-snapshot ./pre.json --output ./post.json
```

## Breaking Changes Detected (NPM to Cilium)

| Rule ID | Severity | Description |
|---------|----------|-------------|
| CILIUM-001 | FAIL | ipBlock catch-all CIDRs (0.0.0.0/0) without selector peers |
| CILIUM-002 | WARN/FAIL | Named ports in NetworkPolicies (not supported by Cilium) |
| CILIUM-003 | FAIL | endPort ranges on Cilium < 1.17 |
| CILIUM-004 | WARN | Implicit local node egress removed in Cilium |
| CILIUM-005 | FAIL | LoadBalancer/NodePort ingress now enforced by Cilium |
| CILIUM-006 | WARN | Host-networked pods lose NetworkPolicy enforcement |
| CILIUM-007 | INFO | kube-proxy removal (Cilium replaces kube-proxy) |
| CILIUM-008 | WARN | Identity exhaustion risk (>50k unique label sets) |
| CILIUM-009 | INFO | Service mesh (Istio/Linkerd) sidecar detected |

## Project Structure

```
aksmigrate/
├── cmd/
│   └── aksmigrate/
│       ├── main.go             # CLI entrypoint
│       └── subcmd/             # Subcommand definitions
│           ├── audit.go
│           ├── translate.go
│           ├── conntest.go
│           ├── discover.go
│           └── migrate.go
├── pkg/
│   ├── types/                  # Shared type definitions
│   ├── utils/                  # Resource loading and output formatting
│   ├── policy/                 # Analysis and translation engines
│   ├── connectivity/           # Connectivity probing and diffing
│   └── cluster/                # Fleet discovery and migration orchestration
├── test/
│   └── policies/               # Sample NetworkPolicy YAML fixtures
├── dashboards/
│   └── migration-monitor.json  # Grafana dashboard
└── docs/
    ├── migration-research-report.md  # Full research report
    └── usage-guide.md                # Detailed CLI reference
```

## Testing

```bash
go test ./...
```

20 unit tests covering all analyzer rules, translator transformations, and YAML rendering.

## Kubernetes / Cilium Version Matrix

The `--k8s-version` flag determines which Cilium version the tools assume:

| K8s Version | Cilium Version | endPort Support |
|-------------|----------------|-----------------|
| 1.27 | 1.13.17 | No |
| 1.28 | 1.14.5 | No |
| 1.29 | 1.14.19 | No |
| 1.30 | 1.15.11 | No |
| 1.31 | 1.16.6 | No |
| 1.32 | 1.17.0 | Yes |

## Documentation

- [Usage Guide](docs/usage-guide.md) - Detailed CLI reference with examples and workflows
- [Migration Research Report](docs/migration-research-report.md) - Full technical analysis of NPM-to-Cilium differences

## License

Copyright (c) Microsoft Corporation. All rights reserved.
