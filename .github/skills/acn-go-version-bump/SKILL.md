---
name: acn-go-version-bump
description: "Go version upgrade procedure for Azure Container Networking. Use when upgrading Go minor/patch versions, bumping MS Go toolchain, fixing FIPS/systemcrypto configuration, updating Dockerfile templates, or responding to Go CVE patches. Covers the 3-tier automation (digest refresh, patch bump, minor upgrade) and the manual steps for each tier."
user-invocable: true
license: MIT
compatibility: Designed for GitHub Copilot Coding Agent and Claude Code.
metadata:
  author: behzad-mir
  version: "4.6.0"
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

**These docs may be pre-cached in `.github/ms-go-docs/` by `copilot-setup-steps.yml` (which runs before the agent firewall activates).** Try reading the cached files first. If they don't exist (e.g., running locally or cache wasn't populated), fall back to `gh api`.

```bash
# 1. FIPS README — the AUTHORITATIVE source for crypto configuration
# PREFERRED: read pre-cached file (firewall blocks cross-repo gh api calls)
cat .github/ms-go-docs/README.md 2>/dev/null || \
  gh api "repos/microsoft/go/contents/eng/doc/fips/README.md?ref=microsoft/main" --jq '.content' | base64 -d

# 2. NocgoOpenSSL — detailed nocgo backend docs
cat .github/ms-go-docs/NocgoOpenSSL.md 2>/dev/null || \
  gh api "repos/microsoft/go/contents/eng/doc/NocgoOpenSSL.md?ref=microsoft/main" --jq '.content' | base64 -d

# 3. Installation guide
cat .github/ms-go-docs/Installation.md 2>/dev/null || \
  gh api "repos/microsoft/go/contents/eng/doc/Installation.md?ref=microsoft/main" --jq '.content' | base64 -d

# 4. Migration Guide — toolchain behavior, breaking changes
cat .github/ms-go-docs/MigrationGuide.md 2>/dev/null || \
  gh api "repos/microsoft/go/contents/eng/doc/MigrationGuide.md?ref=microsoft/main" --jq '.content' | base64 -d

# 5. FIPS User Guide — runtime requirements, crypto API behavior
cat .github/ms-go-docs/UserGuide.md 2>/dev/null || \
  gh api "repos/microsoft/go/contents/eng/doc/fips/UserGuide.md?ref=microsoft/main" --jq '.content' | base64 -d

# 6. Additional Features — all MS-specific patches
cat .github/ms-go-docs/AdditionalFeatures.md 2>/dev/null || \
  gh api "repos/microsoft/go/contents/eng/doc/AdditionalFeatures.md?ref=microsoft/main" --jq '.content' | base64 -d

# 7. Upstream Go release notes (fetch from go.dev — may be blocked)
# curl -sL https://go.dev/doc/go1.<MINOR> || echo "Blocked by firewall — skip"
```

**Image digests are also pre-resolved** into `.github/image-digests/`:
```bash
cat .github/image-digests/go-image.txt        # Go builder SHA
cat .github/image-digests/mariner-core.txt     # Azure Linux base/core SHA
cat .github/image-digests/mariner-distroless.txt  # Azure Linux distroless SHA
cat .github/image-digests/windows-hpc.txt      # Windows HPC base image SHA
```

Use these cached values for Dockerfile updates when `skopeo` is blocked by the firewall.

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

# ALSO find implicit CGO=1 via -buildmode=c-shared (doesn't explicitly set CGO_ENABLED)
grep -rn "buildmode=c-shared" Makefile */Makefile .pipelines/build/scripts/
```

For EACH file that sets `CGO_ENABLED` OR uses `-buildmode=c-shared`, you MUST ensure the correct `GOEXPERIMENT` is set in the same scope.

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

**go.mod version rule (minor upgrades only):** When performing a **minor** version upgrade (Tier 3), the `go` directive in `go.mod` files MUST use `1.XX.1` (the `.1` patch), NOT the latest patch (e.g., NOT `1.26.4`). This is because:
- `.0` releases are pre-release/stabilization — avoid them
- `.1` is the first stable patch — use this as the minimum
- The actual Go binary version comes from the container image (floating tag), not go.mod
- Using the latest patch in go.mod forces all developers to have that exact patch locally

**Note:** For **patch** bumps (Tier 2), the workflow correctly sets `go.mod` to the target patch version (e.g., `1.26.3` → `1.26.4`). This rule only applies to the initial minor upgrade.

**Example:** If upgrading to Go 1.26, set `go 1.26.1` in all go.mod files, even if the latest available patch is `1.26.4`.

### Version Sources (ALL must be updated — do NOT skip any)

**⚠️ IMPORTANT: The ROOT `go.mod` is the FIRST file to update. Do NOT only update sub-modules.**

```
build/images.mk (GO_IMG=golang:1.XX-azurelinux3.0)     ← primary image tag
    │
    ├── ROOT MODULE (update FIRST):
    │   └── → go.mod (go 1.XY.Z)                       ← must match (use .1+ not .0)
    │
    ├── BUILD ENVIRONMENT:
    │   ├── → tools-go/go.mod (go 1.XY.Z)              ← must match root
    │   ├── → .devcontainer/Dockerfile (VARIANT="1.XX") ← dev container version
    │   ├── → .pipelines/build/scripts/install-go.sh (DEFAULT_IMAGE SHA)
    │   ├── → bpf-prog/ipv6-hp-bpf/linux.Dockerfile (Go image SHA)
    │   ├── → npm/linux.Dockerfile (tag 1.XX.Y)
    │   ├── → npm/windows.Dockerfile (tag 1.XX.Y)
    │   └── → All .tmpl Dockerfiles (via `make dockerfiles`)
    │
    └── INDEPENDENT MODULES (bump go directive in EACH):
        ├── → azure-ipam/go.mod
        ├── → azure-ip-masq-merger/go.mod
        ├── → azure-iptables-monitor/go.mod
        ├── → bpf-prog/ipv6-hp-bpf/go.mod
        ├── → cilium-log-collector/go.mod
        ├── → cni/go.mod
        ├── → crd/go.mod
        ├── → dropgz/go.mod
        ├── → npm/go.mod
        ├── → pkgerrlint/go.mod
        ├── → tools/azure-npm-to-cilium-validator/go.mod
        └── → zapai/go.mod
```

### Files to Update (in order)

1. **`go.mod` (ROOT)** — Update `go` directive FIRST. **Use `1.XX.1` (NOT latest patch)**
   - ⚠️ This is the most important file — ALL other modules inherit from this
   - Example: for Go 1.26 upgrade → set `go 1.26.1` (NOT `go 1.26.4`)
2. **`build/images.mk`** — Update `GO_IMG` tag
   - ALWAYS use 2-part floating tag: `1.27-azurelinux3.0`, never `1.27.0-azurelinux3.0`
3. **`tools-go/go.mod`** — Update `go` directive to match root
4. **All sub-module `go.mod` files** — Update `go` directive to match (see full list above)
5. **Do NOT run `go mod tidy`** — it times out in the agent environment
   - Existing `go.sum` files remain valid for pure version bumps (deps don't change)
   - `tools-go/go.sum` is handled by the migration step (copy from `tools.go.sum`)
   - CI or a follow-up commit will reconcile any checksum drift if needed
6. **`.devcontainer/Dockerfile`** — Update `VARIANT` arg to `"1.XX"`
7. **`.pipelines/build/scripts/install-go.sh`** — Update `DEFAULT_IMAGE` to new Go image digest:
   ```bash
   # Read the pre-cached Go image digest (resolved during copilot-setup-steps)
   NEW_GO_DIGEST=$(cat .github/image-digests/go-image.txt 2>/dev/null)
   # If cache exists, use it to update install-go.sh:
   if [ -n "$NEW_GO_DIGEST" ]; then
     sed -i "s|DEFAULT_IMAGE=\".*\"|DEFAULT_IMAGE=\"${NEW_GO_DIGEST}\"|" .pipelines/build/scripts/install-go.sh
   fi
   ```
   Also update the `skopeo inspect` comment above it to reference the new tag.
8. **`bpf-prog/ipv6-hp-bpf/linux.Dockerfile`** — Update Go image SHA (use same digest from step 7)
9. **`npm/linux.Dockerfile`** and **`npm/windows.Dockerfile`** — Update Go tag
   - ⚠️ npm Dockerfiles use **plain patch tag** (e.g., `golang:1.26.4`), NOT the `-azurelinux3.0` suffixed tag
   - The npm builder uses Ubuntu as runtime base, not Azure Linux
   - `npm/windows.Dockerfile` builds on a **Linux builder** (`--platform=linux/amd64`) cross-compiling with `GOOS=windows` — it still needs `GOEXPERIMENT` for CGO_ENABLED=0 on the Linux build stage
   - Replace `MS_GO_NOSYSTEMCRYPTO=1` with appropriate `GOEXPERIMENT=<value>` in BOTH files
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
- `npm/windows.Dockerfile` ← **builds on Linux** (`--platform=linux/amd64`), needs GOEXPERIMENT for CGO=0

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

- **`cilium-log-collector/Makefile`** — The `cilium-log-collector-binary` target uses `-buildmode=c-shared` which **implicitly requires CGO_ENABLED=1**. You MUST add explicit `CGO_ENABLED=1 GOEXPERIMENT=<value_for_cgo1>` to the build command:
  ```makefile
  # BEFORE (missing GOEXPERIMENT):
  cd $(CILIUM_LOG_COLLECTOR_DIR) && go build -buildmode=c-shared ...
  
  # AFTER (correct):
  cd $(CILIUM_LOG_COLLECTOR_DIR) && CGO_ENABLED=1 GOEXPERIMENT=<value_for_cgo1> go build -buildmode=c-shared ...
  ```
  **Do NOT skip this file** — `-buildmode=c-shared` implies CGO but doesn't explicitly set `CGO_ENABLED=1`, so grep for `CGO_ENABLED` won't find it. Search for `-buildmode=c-shared` as well.

### Tools Module Migration (Go 1.26+ requirement)

Go 1.26's stricter `go mod tidy` rejects root-level modfiles (`tools.go.mod`) that share the same module path as `go.mod`. The tools module MUST live in its own directory.

**If `tools.go.mod` exists at the repo root (not yet migrated):**

1. Create `tools-go/` directory
2. Move `tools.go.mod` → `tools-go/go.mod`
3. Move `tools.go.sum` → `tools-go/go.sum`
4. Change module name in `tools-go/go.mod`:
   ```
   - module github.com/Azure/azure-container-networking
   + module github.com/Azure/azure-container-networking/tools-go
   ```
5. Find and update ALL references:
   ```bash
   grep -rn "tools\.go\.mod" . --include="*.go" --include="Makefile" --include="*.sh" --include="*.yaml" --include="*.yml" | grep -v vendor
   ```
   Common locations that reference `-modfile=tools.go.mod`:
   - `crd/clustersubnetstate/Makefile`
   - `crd/multitenancy/Makefile`
   - `crd/multitenantnetworkcontainer/Makefile`
   - `crd/nodenetworkconfig/Makefile`
   - `crd/overlayextensionconfig/Makefile`
   - `cns/multitenantcontroller/mockclients/Makefile`
   - `npm/pkg/dataplane/Makefile`
   - `platform/Makefile`
   - `scripts/install-protoc.sh`
   - Root `Makefile` (`TOOLS_GO_MOD` variable)

   Replace all: `tools.go.mod` → `tools-go/go.mod`

6. Do NOT run `go mod tidy` — just copy the sum file (deps don't change for version bumps)

**If `tools-go/go.mod` already exists (already migrated):**

- Just update the `go` directive to match root
- Do NOT run `go mod tidy` (times out in agent environment)
- Verify all `-modfile` references point to `tools-go/go.mod` (not old `tools.go.mod`)

---

## Step 2: Validate

After making all changes:

1. **Root go.mod check** — Verify the root `go.mod` has the correct version:
   ```bash
   head -5 go.mod  # MUST show "go 1.XX.Y" — if still old version, the upgrade is INCOMPLETE
   ```
2. `go build ./...` — Verify compilation succeeds (all binaries)
3. `go vet ./...` — Check for deprecated API usage
4. `docker build` (spot-check) — Verify at least one container image builds:
   ```bash
   # Quick validation that Dockerfiles + GOEXPERIMENT produce working images
   docker build -f cni/Dockerfile -t acn-cni-test --build-arg VERSION=test .
   ```
5. **`make dockerfiles`** — Regenerate ALL template-based Dockerfiles. This resolves:
   - `{{.GO_PIN}}` → current Go image SHA
   - `{{.MARINER_CORE_PIN}}` → current azurelinux/base/core SHA
   - `{{.MARINER_DISTROLESS_PIN}}` → current azurelinux/distroless/base SHA
   
   The generated files live in TWO locations:
   - Component directories: `cni/Dockerfile`, `cns/Dockerfile`, `azure-ipam/Dockerfile`, etc.
   - Pipeline directory: `.pipelines/build/dockerfiles/*.Dockerfile`
   
   **If `make dockerfiles` fails** (e.g., skopeo blocked by firewall or MCR auth issues), use the **pre-cached digests**:
   ```bash
   # Read pre-resolved digests from setup steps
   GO_PIN=$(cat .github/image-digests/go-image.txt 2>/dev/null)
   MARINER_CORE_PIN=$(cat .github/image-digests/mariner-core.txt 2>/dev/null)
   MARINER_DISTROLESS_PIN=$(cat .github/image-digests/mariner-distroless.txt 2>/dev/null)
   WINDOWS_HPC_PIN=$(cat .github/image-digests/windows-hpc.txt 2>/dev/null)

   # If cached files don't exist, try skopeo directly (may fail behind firewall)
   if [ -z "$GO_PIN" ]; then
     GO_PIN=$(skopeo inspect docker://mcr.microsoft.com/oss/go/microsoft/golang:1.XX-azurelinux3.0 \
       --format "{{.Name}}@{{.Digest}}" 2>/dev/null)
   fi
   ```
   Then use `sed` to update SHA digests in ALL generated `.Dockerfile` files (not `.tmpl`).
   
   **IMPORTANT:** Both `.pipelines/build/dockerfiles/*.Dockerfile` AND component `*/Dockerfile` files must be updated — they are ALL generated from templates.
5. Do NOT run `go mod tidy` — it times out. Existing go.sum files remain valid for version bumps.
6. Verify no new `replace` directives are needed
7. **Dev environment check:**
   ```bash
   grep "VARIANT" .devcontainer/Dockerfile  # Must show target version
   ```
8. **FIPS validation** — run this check:

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

8. **Completeness validation** — verify ALL version sources updated:
   ```bash
   TARGET="1.XX"  # Replace with actual target minor version
   
   # Root go.mod MUST be updated
   grep "^go " go.mod | grep -q "$TARGET" || echo "FAIL: root go.mod not updated!"
   
   # .devcontainer must reference new version
   grep -q "VARIANT=\"$TARGET\"" .devcontainer/Dockerfile || echo "FAIL: .devcontainer not updated!"
   
   # All sub-module go.mod files
   for mod in azure-ipam cni crd dropgz npm zapai azure-ip-masq-merger azure-iptables-monitor \
              bpf-prog/ipv6-hp-bpf cilium-log-collector pkgerrlint tools/azure-npm-to-cilium-validator; do
     if [ -f "$mod/go.mod" ]; then
       grep "^go " "$mod/go.mod" | grep -q "$TARGET" || echo "FAIL: $mod/go.mod not updated!"
     fi
   done
   
   # tools-go module
   grep "^go " tools-go/go.mod | grep -q "$TARGET" || echo "FAIL: tools-go/go.mod not updated!"
   
   # build/images.mk
   grep "GO_IMG" build/images.mk | grep -q "$TARGET" || echo "FAIL: build/images.mk not updated!"
   ```

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
4. Do NOT run `go mod tidy` — existing go.sum remains valid for version bumps
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


### AKS Release Train Awareness

When upgrading Go, verify compatibility with AKS supported Kubernetes versions:
- Reference: https://learn.microsoft.com/en-us/azure/aks/supported-kubernetes-versions
- Ensure controller-runtime and client-go versions support the target Go version
- Cross-reference with release branches (`release/v1.7`, etc.) that map to AKS release trains
### Important Notes

- **ALWAYS use `1.XX.1` in go.mod** — NOT the latest patch. The container image provides the actual binary version.
- The `npm/` component is released as **npm-lite** — ensure Dockerfiles build correctly
- npm Dockerfiles use **plain Go tags** (e.g., `golang:1.26.4`) without `-azurelinux3.0` suffix
- `npm/windows.Dockerfile` builds on a Linux builder (`--platform=linux/amd64`) — still needs GOEXPERIMENT for CGO=0
- The `baseimages.yaml` CI workflow fails if `make dockerfiles` output doesn't match committed files
- ALWAYS use 2-part floating tags in `build/images.mk`
- **Windows builds**: CNG backend typically works without CGO or GOEXPERIMENT — verify per version
- **Do NOT assume "no GOEXPERIMENT" is safe** — always verify the default backend's CGO requirements
