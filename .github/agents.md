# CI/CD Workflow Remediation Brief

This brief is for agents resolving failures in GitHub Actions CI/CD workflows for this repo.

## govulncheck (`govulncheck.yaml`)

The govulncheck workflow runs [`govulncheck`](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) across all Go modules in the repo (matrix: `.`, `azure-ip-masq-merger`, `azure-ipam`, `azure-iptables-monitor`, `bpf-prog/ipv6-hp-bpf`, `cilium-log-collector`, `dropgz`, `pkgerrlint`, `tools/azure-npm-to-cilium-validator`, `zapai`). It fails with exit code 3 when the code calls into a vulnerable function in a dependency.

### Diagnosing failures

Workflow run logs are the primary signal. Key fields to extract per failing job:
- **Vulnerability ID** (e.g. `GO-2026-5026`) — look up at `https://pkg.go.dev/vuln/<ID>` for affected packages and fixed versions.
- **Module** — which `go.mod` file controls the affected dependency.
- **Call chain** — the exact call stack (`ipam.go:96 → ... → idna.ToASCII`) confirms the code path is reachable.

govulncheck distinguishes three findings categories:
- `Your code is affected by N vulnerabilities` — these **must** be fixed; they are reachable from your code.
- `vulnerabilities in packages you import` — code imports the package but may not call the vulnerable function; still worth fixing.
- `vulnerabilities in modules you require but your code doesn't appear to call` — lowest priority; the dep is present but the call path is not exercised.

Only the first category causes CI failure (exit code 3).

### Fixing failures

The fix is almost always a dependency upgrade. For each failing module:

```bash
cd <module-dir>          # e.g. "cd azure-ipam" or "cd ." for the root module
go get <dep>@<fixed-ver> # e.g. "go get golang.org/x/net@v0.55.0"
go mod tidy
cd -
```

Run govulncheck locally to verify before pushing:

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck -C <module-dir> ./...
```

Commit all changed `go.mod` and `go.sum` files.

### Current known failures (as of 2026-05-29)

The following vulnerabilities are actively failing CI across 4 modules:

| Vuln | CVE | Package | Fixed in | Affected modules |
|------|-----|---------|----------|-----------------|
| [GO-2026-5026](https://pkg.go.dev/vuln/GO-2026-5026) | — | `golang.org/x/net/idna` (`ToASCII`/`ToUnicode` accepts invalid Punycode → privilege escalation) | `golang.org/x/net` ≥ v0.55.0 | `.`, `azure-ipam`, `azure-iptables-monitor`, `tools/azure-npm-to-cilium-validator` |
| [GO-2026-4918](https://pkg.go.dev/vuln/GO-2026-4918) | CVE-2026-33814 | `golang.org/x/net/http2` (infinite CONTINUATION frame loop on zero `SETTINGS_MAX_FRAME_SIZE` → DoS) | `golang.org/x/net` ≥ v0.53.0 | `.`, `azure-ipam`, `azure-iptables-monitor`, `tools/azure-npm-to-cilium-validator` |

Both are fixed by upgrading to `golang.org/x/net` **v0.55.0** (the higher minimum). Current versions:

| Module | Current version |
|--------|----------------|
| `.` | v0.48.0 |
| `azure-ipam` | v0.48.0 |
| `azure-iptables-monitor` | v0.38.0 |
| `tools/azure-npm-to-cilium-validator` | v0.48.0 |

To fix all four in one pass:

```bash
go get golang.org/x/net@v0.55.0 && go mod tidy
cd azure-ipam && go get golang.org/x/net@v0.55.0 && go mod tidy && cd ..
cd azure-iptables-monitor && go get golang.org/x/net@v0.55.0 && go mod tidy && cd ..
cd tools/azure-npm-to-cilium-validator && go get golang.org/x/net@v0.55.0 && go mod tidy && cd ..
```

### `check-gomod-coverage` job

A second job in the workflow verifies every `go.mod` file in the repo is listed in the matrix. If it fails with `ERROR: The following go.mod files are not in the govulncheck matrix`, add the missing module paths to the `matrix.module` list in `.github/workflows/govulncheck.yaml` (and the corresponding `MATRIX_MODULES` array in the same file's shell check).

---

## Docker Base Images (`baseimages.yaml`)

The baseimages workflow runs `make dockerfiles`, which uses [`renderkit`](https://github.com/orellazri/renderkit) to render `*.Dockerfile.tmpl` templates into committed `Dockerfile` files. It then checks `git status --porcelain` and fails if any files changed.

### Why it fails

The committed Dockerfiles are rendered artifacts. They drift from the templates when:
- A `.Dockerfile.tmpl` template is edited but `make dockerfiles` was not run before committing.
- A base image digest in the template is pinned or auto-resolved and has changed upstream.

The fix is always the same: run `make dockerfiles` and commit the updated output files.

### Templates and output files

| Template | Rendered output |
|----------|----------------|
| `cns/Dockerfile.tmpl` | `cns/Dockerfile`, `.pipelines/build/dockerfiles/cns.Dockerfile` |
| `cni/Dockerfile.tmpl` | `cni/Dockerfile`, `.pipelines/build/dockerfiles/cni.Dockerfile` |
| `azure-ipam/Dockerfile.tmpl` | `azure-ipam/Dockerfile`, `.pipelines/build/dockerfiles/azure-ipam.Dockerfile` |
| `azure-ip-masq-merger/Dockerfile.tmpl` | `azure-ip-masq-merger/Dockerfile`, `.pipelines/build/dockerfiles/azure-ip-masq-merger.Dockerfile` |
| `azure-iptables-monitor/Dockerfile.tmpl` | `azure-iptables-monitor/Dockerfile`, `.pipelines/build/dockerfiles/azure-iptables-monitor.Dockerfile` |
| `cilium-log-collector/Dockerfile.tmpl` | `cilium-log-collector/Dockerfile`, `.pipelines/build/dockerfiles/cilium-log-collector.Dockerfile` |

### Fixing failures

```bash
make dockerfiles
git diff --name-only   # confirm which Dockerfiles changed
git add <changed files>
git commit -m "ci: regenerate Dockerfiles via make dockerfiles"
```

`make dockerfiles` requires `renderkit` on `$PATH` and network access to MCR (Microsoft Container Registry) to resolve base image digests. Run this in a standard development environment with internet access.

To identify exactly which template caused the drift, inspect the diff of the changed Dockerfile against its `.tmpl` source.

### Checklist before pushing

- [ ] `git status --porcelain` is empty after running `make dockerfiles`
- [ ] Only Dockerfile files (no template files) are in the commit
- [ ] All changed Dockerfiles trace to a known template or base image update

---

## General CI/CD agent notes

- Always check the most recent failed run for the specific workflow before acting — the failure reason may have changed.
- For both workflows, the fix should be a single focused commit touching only the generated/dependency files.
- Do not modify workflow YAML files (`.github/workflows/*.yaml`) unless the failure is in the workflow definition itself (e.g., a missing matrix entry).
