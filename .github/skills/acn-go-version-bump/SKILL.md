---
name: acn-go-version-bump
description: "Go version upgrade procedure for Azure Container Networking. Use when upgrading Go minor/patch versions, bumping MS Go toolchain, fixing FIPS/systemcrypto configuration, updating Dockerfile templates, or responding to Go CVE patches. Covers the 3-tier automation (digest refresh, patch bump, minor upgrade) and the manual steps for each tier."
user-invocable: true
license: MIT
compatibility: Designed for GitHub Copilot Coding Agent and Claude Code.
metadata:
  author: behzad-mir
  version: "4.0.0"
allowed-tools: Read Edit Write Glob Grep Bash(go:*) Bash(make:*) Bash(skopeo:*) Bash(git:*) Bash(gh:*) Agent
---

**Persona:** You are a Go platform engineer maintaining the Azure Container Networking build toolchain. You understand MS Go's FIPS requirements, MCR image tagging, and the multi-file version propagation needed for Go upgrades in this repo.

**Modes:**

- **Upgrade mode** — performing a Go version upgrade (minor or patch). Analyze MS Go docs, assess repo impact, then execute changes.
- **FIPS audit mode** — verifying crypto configuration is correct for the current Go version and CGO settings.
- **Workflow debug mode** — troubleshooting the `go-version-check.yaml` automation workflow.

---

# Go Version Upgrade Procedure

## Step 0: Analyze MS Go Documentation (MANDATORY)

**CRITICAL: Before making ANY code changes, you MUST fetch and analyze ALL relevant MS Go documentation for the target version. Do not rely on hardcoded rules — requirements change between versions.**

### Documents to Fetch

Use `gh api` to fetch each document from the `microsoft/go` repo (`ref=microsoft/main`):

```bash
# 1. FIPS README — the AUTHORITATIVE source for crypto configuration
# This is the MOST IMPORTANT doc. It contains "Usage: Common configurations" table.
gh api "repos/microsoft/go/contents/eng/doc/fips/README.md?ref=microsoft/main" --jq '.content' | base64 -d

# 2. NocgoOpenSSL — detailed nocgo backend docs
gh api "repos/microsoft/go/contents/eng/doc/NocgoOpenSSL.md?ref=microsoft/main" --jq '.content' | base64 -d

# 3. Installation guide — how to install MS Go in CI
gh api "repos/microsoft/go/contents/eng/doc/Installation.md?ref=microsoft/main" --jq '.content' | base64 -d

# 4. Version-specific notes (may not exist for all versions)
gh api "repos/microsoft/go/contents/docs/go1.<MINOR>.md?ref=microsoft/main" --jq '.content' | base64 -d

# 5. Migration Guide — toolchain behavior, breaking changes
gh api "repos/microsoft/go/contents/eng/doc/MigrationGuide.md?ref=microsoft/main" --jq '.content' | base64 -d

# 6. FIPS User Guide — runtime requirements, crypto API behavior
gh api "repos/microsoft/go/contents/eng/doc/fips/UserGuide.md?ref=microsoft/main" --jq '.content' | base64 -d

# 7. Additional Features — all MS-specific patches
gh api "repos/microsoft/go/contents/eng/doc/AdditionalFeatures.md?ref=microsoft/main" --jq '.content' | base64 -d

# 8. Upstream Go release notes
# Fetch from https://go.dev/doc/go1.<MINOR>
```

### Analysis Procedure — GOEXPERIMENT Determination

**This is the step the agent failed on previously. Follow it precisely.**

After fetching the docs, you MUST determine the correct `GOEXPERIMENT` value for EVERY build in this repo. The answer depends on THREE things:
1. The target Go version number
2. Whether the build uses `CGO_ENABLED=0` or `CGO_ENABLED=1`
3. The platform (Linux vs Windows)

**How to find the answer in the docs:**

1. Open `eng/doc/fips/README.md` and find the section titled **"Usage: Common configurations"**
2. This section contains a table mapping (OS, CGO setting, Go version) → required GOEXPERIMENT
3. Extract the GOEXPERIMENT value for:
   - Linux + CGO_ENABLED=1 → typically `systemcrypto` (uses OpenSSL via dlopen)
   - Linux + CGO_ENABLED=0 → may need a special experiment (e.g., `ms_nocgo_opensslcrypto`)
   - Windows → typically no GOEXPERIMENT needed (CNG backend works without CGO)

**CRITICAL UNDERSTANDING:** In MS Go, the crypto backend is NOT optional — it's mandatory for FIPS compliance. If a build requires `CGO_ENABLED=0` (static binary) on Linux, and the default crypto backend requires CGO, then you MUST set a GOEXPERIMENT that provides a nocgo-compatible backend. Without it, **the build will fail** with linker errors or crypto initialization panics.

**DO NOT** assume that "no GOEXPERIMENT" is safe for CGO=0 builds. Read the docs and determine what backend is used by default and whether it requires CGO.

### Cross-Reference with Repo

After determining the correct GOEXPERIMENT per (CGO, OS) pair, audit EVERY build path:

```bash
# Find all CGO settings in build scripts
grep -rn "CGO_ENABLED" .pipelines/build/scripts/ --include="*.sh"

# Find all CGO settings in Dockerfiles and templates
grep -rn "CGO_ENABLED" . --include="*.Dockerfile" --include="*.Dockerfile.tmpl" --include="Dockerfile.tmpl"

# Find CGO settings in Makefiles
grep -rn "CGO_ENABLED" Makefile */Makefile
```

For EACH file that sets `CGO_ENABLED`, you MUST ensure the correct `GOEXPERIMENT` is set in the same scope.

### Full Analysis Checklist

#### A. Build Environment Requirements

- [ ] Required `GOEXPERIMENT` values per (CGO, OS) combination — **from fips/README.md table**
- [ ] Required build flags (`-buildmode`, `-ldflags`, etc.)
- [ ] `GOTOOLCHAIN` behavior changes
- [ ] Any new env vars introduced or deprecated
- [ ] `go mod tidy` behavior changes (stricter validation, new directives)

#### B. Runtime Dependencies

- [ ] Required system libraries (libc, libdl, libpthread, libcrypto)
- [ ] Base image requirements (what libraries must be present)
- [ ] Architecture-specific limitations

Cross-reference:
- Check `MARINER_DISTROLESS_IMG` in `build/images.mk`
- Verify runtime base images have required crypto libraries

#### C. Crypto/FIPS Changes (CRITICAL)

- [ ] Which crypto backend is selected BY DEFAULT (no GOEXPERIMENT set)?
- [ ] Does the default backend require CGO? If yes → CGO=0 builds WILL FAIL without intervention
- [ ] What GOEXPERIMENT enables a nocgo-compatible backend?
- [ ] Is `MS_GO_NOSYSTEMCRYPTO` deprecated?
- [ ] Any crypto API behavior changes (non-FIPS curves, key sizes)?

#### D. Compatibility & Breaking Changes

- [ ] Deprecated stdlib APIs
- [ ] Module system changes
- [ ] Key dependency support (controller-runtime, client-go, cilium)

### Output: Change Plan

Before making any code changes, produce a change plan:

```markdown
## MS Go <VERSION> Upgrade — Requirements Analysis

### Source Documents Reviewed
- [ ] eng/doc/fips/README.md: <GOEXPERIMENT table findings>
- [ ] eng/doc/NocgoOpenSSL.md: <nocgo backend details>
- [ ] docs/go1.XX.md: <version-specific changes>
- [ ] eng/doc/MigrationGuide.md: <relevant migration steps>
- [ ] eng/doc/fips/UserGuide.md: <runtime requirements>

### GOEXPERIMENT Determination (from fips/README.md)

| Build Configuration | GOEXPERIMENT Required | Reason |
|---|---|---|
| Linux + CGO_ENABLED=1 | <value from docs> | <why> |
| Linux + CGO_ENABLED=0 | <value from docs> | <why — if blank, explain why safe> |
| Windows (any CGO) | <value from docs> | <why> |

### Files Requiring GOEXPERIMENT Changes

List EVERY file that needs modification:
| File | CGO Setting | Current GOEXPERIMENT | Required GOEXPERIMENT | Action |
|---|---|---|---|---|
| .pipelines/build/scripts/cni.sh | 0 | (none) | <value> | Add export |
| .pipelines/build/scripts/cns.sh | 0 | (none) | <value> | Add export |
| ... | ... | ... | ... | ... |
| Makefile (all CGO=0 targets) | 0 | (none) | <value> | Add inline |

### Risk Assessment
- Breaking changes affecting this codebase: ...
- FIPS compliance impact: ...
```

**Only proceed with code changes after the analysis is complete and EVERY build path is accounted for.**

---

## Step 1: Execute Version Bump

### Go Version Strategy

ACN uses **floating minor version tags** for the Go build image (`build/images.mk`):
- `GO_IMG` uses a 2-part minor version tag (e.g., `golang:1.26-azurelinux3.0`)
- The floating tag resolves to the latest patch via SHA digest at `make dockerfiles` time

### Version Sources (ALL must be updated)

```
build/images.mk (GO_IMG=golang:1.XX-azurelinux3.0)     ← primary tag
    ├── → go.mod (go 1.XX.Y)                           ← must match (use .1+ not .0)
    ├── → tools-go/go.mod (go 1.XX.Y)                  ← must match (formerly tools.go.mod, moved to own dir for Go 1.26 compat)
    ├── → .devcontainer/Dockerfile (VARIANT="1.XX")
    ├── → .pipelines/build/scripts/install-go.sh (DEFAULT_IMAGE SHA)
    ├── → bpf-prog/ipv6-hp-bpf/linux.Dockerfile (Go image SHA)
    ├── → npm/linux.Dockerfile (tag 1.XX.Y)
    ├── → npm/windows.Dockerfile (tag 1.XX.Y)
    └── → All .tmpl Dockerfiles (via `make dockerfiles`)

Independent modules (bump go directive in each):
    ├── → cilium-log-collector/go.mod
    ├── → azure-ipam/go.mod
    ├── → azure-ip-masq-merger/go.mod
    ├── → azure-iptables-monitor/go.mod
    ├── → bpf-prog/ipv6-hp-bpf/go.mod
    ├── → dropgz/go.mod
    ├── → zapai/go.mod
    └── → tools/azure-npm-to-cilium-validator/go.mod
```

### Files to Update (in order)

1. **`build/images.mk`** — Update `GO_IMG` tag
   - ALWAYS use 2-part floating tag: `1.27`, never `1.27.0`
2. **`go.mod`** — Update `go` directive (use `.1` minimum, e.g., `go 1.27.1`)
3. **`tools-go/go.mod`** — Update `go` directive to match
4. **All sub-module `go.mod` files** — Update `go` directive to match
5. **Run `go mod tidy`** on root, tools-go/, and each sub-module
6. **`.pipelines/build/scripts/install-go.sh`** — Update `DEFAULT_IMAGE` SHA
7. **`bpf-prog/ipv6-hp-bpf/linux.Dockerfile`** — Update Go image SHA
8. **`npm/linux.Dockerfile`** and **`npm/windows.Dockerfile`** — Update Go tag
9. **`.devcontainer/Dockerfile`** — Update `VARIANT` arg
10. **Run `make dockerfiles`** — Regenerate all template-based Dockerfiles

### Step 1b: Apply GOEXPERIMENT to ALL Build Paths (CRITICAL)

**This step is where the previous agent failed. Do NOT skip any file.**

Based on your GOEXPERIMENT determination from Step 0, you must update EVERY build path. Here is the COMPLETE list of locations that need GOEXPERIMENT:

#### Pipeline Build Scripts (`.pipelines/build/scripts/*.sh`)

Each script that sets `CGO_ENABLED` MUST also export the correct `GOEXPERIMENT`:

```bash
# For CGO_ENABLED=0 scripts (Linux-only guard if script is multi-platform):
if [[ "$GOOS" == "linux" ]] || [[ -z "$GOOS" ]]; then
  export GOEXPERIMENT=<value_for_cgo0>
fi
export CGO_ENABLED=0

# For CGO_ENABLED=1 scripts:
export GOEXPERIMENT=<value_for_cgo1>
export CGO_ENABLED=1
```

Scripts to update:
- `.pipelines/build/scripts/cni.sh`
- `.pipelines/build/scripts/cns.sh`
- `.pipelines/build/scripts/npm.sh`
- `.pipelines/build/scripts/dropgz.sh`
- `.pipelines/build/scripts/azure-ipam.sh`
- `.pipelines/build/scripts/azure-ip-masq-merger.sh`
- `.pipelines/build/scripts/azure-iptables-monitor.sh`
- `.pipelines/build/scripts/ipv6-hp-bpf.sh`
- `.pipelines/build/scripts/cilium-log-collector.sh`

#### Dockerfile Templates (`*.Dockerfile.tmpl`)

Each template that sets `CGO_ENABLED` in a `RUN go build` must have `ENV GOEXPERIMENT=<value>` set BEFORE the build stage:

```dockerfile
# For CGO_ENABLED=0 stages:
ENV GOEXPERIMENT=<value_for_cgo0>
RUN CGO_ENABLED=0 go build ...

# For CGO_ENABLED=1 stages:
ENV GOEXPERIMENT=<value_for_cgo1>
RUN CGO_ENABLED=1 go build ...
```

Templates to update:
- `cni/Dockerfile.tmpl`
- `cns/Dockerfile.tmpl`
- `azure-ipam/Dockerfile.tmpl`
- `azure-ip-masq-merger/Dockerfile.tmpl`
- `azure-iptables-monitor/Dockerfile.tmpl`
- `cilium-log-collector/Dockerfile.tmpl`

#### Standalone Dockerfiles (not generated from templates)

- `bpf-prog/ipv6-hp-bpf/linux.Dockerfile`
- `npm/linux.Dockerfile`

#### Root Makefile

The root `Makefile` has CGO_ENABLED=0 build lines for local development. Add a variable and apply it inline:

```makefile
# GOEXPERIMENT for CGO_ENABLED=0 Linux builds (MS Go FIPS requirement)
ACN_GOEXPERIMENT ?= <value_for_cgo0>

# Apply to each CGO_ENABLED=0 build line:
GOEXPERIMENT=$(ACN_GOEXPERIMENT) CGO_ENABLED=0 go build ...
```

**Do NOT `export GOEXPERIMENT` globally** — it breaks renderkit/tool builds that don't recognize the experiment.

#### Component-Specific Makefiles

- `cilium-log-collector/Makefile` — Explicit `GOEXPERIMENT=<value_for_cgo1> CGO_ENABLED=1`

### Tools Module

Tools live in `tools-go/go.mod` (module: `github.com/Azure/azure-container-networking/tools-go`):
- Isolates tool dependencies from the main module graph
- Go 1.26's stricter `go mod tidy` requires this separation
- All `-modfile` references point to `tools-go/go.mod`
- Root Makefile: `TOOLS_GO_MOD = $(REPO_ROOT)/tools-go/go.mod`

---

## Step 2: Validate

After making all changes:

1. `go build ./...` — Verify compilation succeeds
2. `go vet ./...` — Check for deprecated API usage
3. `make dockerfiles` — Ensure templates render correctly (output must match committed files)
4. `go mod tidy` — Ensure deps are clean (root and tools-go/)
5. Verify no new `replace` directives are needed
6. **FIPS validation** — run this check:

```bash
# Verify ALL CGO_ENABLED=0 scripts have the correct GOEXPERIMENT
for script in .pipelines/build/scripts/*.sh; do
  if grep -q "CGO_ENABLED=0" "$script"; then
    if ! grep -q "GOEXPERIMENT=<value_for_cgo0>" "$script"; then
      echo "MISSING GOEXPERIMENT in: $script"
    fi
  fi
done

# Verify ALL CGO_ENABLED=0 Dockerfile templates have it
for tmpl in $(find . -name '*.Dockerfile.tmpl' -o -name 'Dockerfile.tmpl' | grep -v vendor); do
  if grep -q "CGO_ENABLED=0" "$tmpl"; then
    if ! grep -q "GOEXPERIMENT=<value_for_cgo0>" "$tmpl"; then
      echo "MISSING GOEXPERIMENT in: $tmpl"
    fi
  fi
done
```

7. Cross-check every item in your Change Plan was actually applied

---

## Step 3: PR and Backport

### PR Guidelines

- Title: `chore: upgrade Go <OLD> → <NEW>`
- Reference the tracking issue in the PR body
- **Include the Requirements Matrix from Step 0** in the PR description
- **Include the GOEXPERIMENT Determination table** prominently
- List all files modified
- Highlight FIPS/crypto requirement changes

### Backport to `release/v1.7`

**Every Go version change on master MUST be backported to `release/v1.7`.**

1. Check out `release/v1.7`
2. Apply same version/SHA changes
3. If release branch is missing GOEXPERIMENT prerequisites, add those too
4. Run `go mod tidy` separately (release branch may have different deps)
5. Run `make dockerfiles`
6. Title: `chore(release/v1.7): upgrade Go <OLD> → <NEW>`

---

## Architecture Notes

### Template System
- `build/images.mk` defines `GO_IMG` and `MARINER_DISTROLESS_IMG`
- `.tmpl` files are rendered into Dockerfiles by `make dockerfiles`
- Uses `renderkit` and `skopeo` to resolve image tags to SHA digests
- Pipeline uses `.pipelines/build/scripts/install-go.sh`

### Component CGO Map

> **GOEXPERIMENT values are version-dependent.** Always determine the correct value from
> `eng/doc/fips/README.md` for the target version. The CGO settings below are fixed per component.

| Component | CGO_ENABLED | Platform | Build Mode | Notes |
|-----------|:-----------:|:--------:|:----------:|-------|
| cni | 0 | linux | static binary | Network plugin |
| cns | 0 | linux/windows | static binary | Node daemon |
| npm | 0 | linux/windows | static binary | Network policy |
| dropgz | 0 | linux | static binary | Installer wrapper |
| azure-ipam | 0 | linux | static binary | IP allocator |
| azure-ip-masq-merger | 0 | linux | static binary | IPtables helper |
| azure-iptables-monitor | 0 | linux | static binary | IPtables monitor |
| ipv6-hp-bpf | 0 | linux | static binary | BPF health probe |
| cilium-log-collector | 1 | linux | c-shared (.so) | Fluent Bit plugin, requires CGO |

### Important Notes

- Use `.1` as minimum patch version (`.0` is pre-release/stabilization)
- The `npm/` component is no longer released — update but don't worry about testing
- The `baseimages.yaml` CI workflow fails if `make dockerfiles` output doesn't match committed files
- ALWAYS use 2-part floating tags in `build/images.mk`
- **Windows builds**: CNG backend typically works without CGO or GOEXPERIMENT — verify per version
- **Do NOT assume "no GOEXPERIMENT" is safe** — always verify the default backend's CGO requirements
