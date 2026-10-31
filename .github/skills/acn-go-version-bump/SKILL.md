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
grep -rn "CGO_ENABLED" . --include="*.Dockerfile" --include="Dockerfile"

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
- Check the runtime base (`distroless/base`) pinned in the component Dockerfiles
- Verify runtime base images have required crypto libraries

#### C. Crypto/FIPS Changes (CRITICAL)

- [ ] Which crypto backend is selected BY DEFAULT (no GOEXPERIMENT set)?
- [ ] Does the default backend require CGO? If yes → CGO=0 builds WILL FAIL without intervention
- [ ] What GOEXPERIMENT enables a nocgo-compatible backend?
- [ ] Is `MS_GO_NOSYSTEMCRYPTO` deprecated? (As of Go 1.27 it is NOT: it is the supported opt-out. Go 1.27 removed `GOEXPERIMENT=systemcrypto`, `nosystemcrypto` and `ms_nocgo_opensslcrypto`; the toolchain rejects all three.)
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

ACN uses **floating minor version tags** for the Go build image (in each Dockerfile's `FROM`):
- The component Dockerfiles use a 2-part minor version tag (e.g., `golang:1.26-azurelinux3.0`)
- Dependabot keeps the pinned SHA current for this tag; moving to a new minor tag family is a manual edit to the Dockerfile `FROM` pins

**Separate compatibility from the preferred toolchain:**

- The `go` directive is the module's minimum language/toolchain and dependency
  compatibility floor. Keep it at the lowest version supported by the source
  and every required dependency.
- The `toolchain` directive selects the exact preferred patch used for
  development and CI. Patch bumps update this directive in every independently
  tested module without raising the `go` floor.
- Use a full release version for the floor when dependencies require it. For
  example, `go 1.25` sorts before `go 1.25.0` and cannot satisfy dependencies
  that declare `go 1.25.0`.

**Example:** If the compatibility floor is Go 1.25 and the supported build
toolchain is Go 1.26.7, use this in every module:

```go
go 1.25.0

toolchain go1.26.7
```

### Version Sources (ALL must be updated — do NOT skip any)

**⚠️ IMPORTANT: The ROOT `go.mod` is the FIRST file to update. Do NOT only update sub-modules.**

```
Go builder tag: golang:1.XX-azurelinux3.0 (pinned in the component Dockerfile FROM lines)     ← primary image tag
    │
    ├── ROOT MODULE:
    │   └── → go.mod (go floor + exact toolchain)
    │
    ├── BUILD ENVIRONMENT:
    │   ├── → tools.go.mod (same floor + toolchain)
    │   ├── → .devcontainer/Dockerfile (VARIANT="1.XX") ← dev container version
    │   ├── → .pipelines/build/scripts/install-go.sh (DEFAULT_IMAGE SHA)
    │   ├── → bpf-prog/ipv6-hp-bpf/linux.Dockerfile (Go image SHA)
    │   ├── → npm/linux.Dockerfile (tag 1.XX.Y)
    │   ├── → npm/windows.Dockerfile (tag 1.XX.Y)
    │   └── → All component + .pipelines/build/dockerfiles/ Dockerfile FROM pins
    │
    └── INDEPENDENT MODULES (update toolchain in EACH; raise go floor only when required):
        └── → every other go.mod in the repo, found with
              `git ls-files '*go.mod' ':!:**/vendor/**'`
              (e.g. azure-ipam/, dropgz/, zapai/, tools/failure-agent/, tools/release/)
```

> Do NOT maintain a hand-written module list. Modules are added over time; a static list goes stale and leaves toolchains behind.

### Files to Update (in order)

1. **All `go.mod` files and `tools.go.mod`** — REQUIRED: run `hack/scripts/update-go-toolchain.sh 1.XX.Y`.
   It sets the exact `toolchain` in every `go.mod` (root and all submodules, found with `find`) and in `tools.go.mod`.
   Do not edit toolchains by hand.
   - Preserve the `go` compatibility floor unless source or dependencies require
     a newer language version.
2. **Component + `.pipelines/build/dockerfiles/` Dockerfiles** — Update the golang `FROM` pins
   - ALWAYS use 2-part floating tag: `1.27-azurelinux3.0`, never `1.27.0-azurelinux3.0`
3. **`tools.go.mod`** (repo root) — covered by step 1 (`go mod edit -modfile=tools.go.mod -toolchain=go1.XX.Y`)
4. **All sub-module `go.mod` files** — covered by step 1; Step 2 validation checks every module
5. **Do NOT run `go mod tidy`** — it times out in the agent environment
   - Existing `go.sum` files remain valid for pure version bumps (deps don't change)
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
8. **`bpf-prog/ipv6-hp-bpf/linux.Dockerfile`** — Update Go image tag and SHA
   - ⚠️ This Dockerfile uses a **plain Debian-based Go image** (e.g., `golang:1.26.4`), NOT the `-azurelinux3.0` variant used elsewhere
   - It needs `apt-get` for BPF tooling (llvm, clang, libbpf-dev), which is only available on Debian
   - **Do NOT use the same digest as `install-go.sh` or the component Dockerfiles** — those use the Azure Linux image
   - Resolve the correct digest separately:
     ```bash
     skopeo inspect "docker://mcr.microsoft.com/oss/go/microsoft/golang:1.XX.Y" --format "{{.Digest}}"
     ```
     (using the plain patch tag, e.g., `1.26.6`, without `-azurelinux3.0`)
9. **`npm/linux.Dockerfile`** and **`npm/windows.Dockerfile`** — Update Go tag
   - ⚠️ npm Dockerfiles use **plain patch tag** (e.g., `golang:1.26.4`), NOT the `-azurelinux3.0` suffixed tag
   - The npm builder uses Ubuntu as runtime base, not Azure Linux
   - `npm/windows.Dockerfile` builds on a **Linux builder** (`--platform=linux/amd64`) cross-compiling with `GOOS=windows`
   - ⚠️ **KEEP `MS_GO_NOSYSTEMCRYPTO=1`** in BOTH files. Do NOT remove it or replace it with a `GOEXPERIMENT`. npm runs on Ubuntu, which does not ship Microsoft's FIPS OpenSSL. Building it with systemcrypto makes it crash-loop on FIPS-enabled clusters. `MS_GO_NOSYSTEMCRYPTO=1` is still the supported opt-out in Go 1.27+.
10. **Update the `FROM ...golang:...@sha` pins directly** in every component Dockerfile and `.pipelines/build/dockerfiles/*.Dockerfile` (the Dockerfiles are the source of truth). Resolve each digest with `skopeo inspect docker://<image> --format "{{.Digest}}"` or read the pre-cached `.github/image-digests/*.txt`; never hand-write a digest

### Step 1b: Apply GOEXPERIMENT to ALL Build Paths (CRITICAL)

> **Go 1.27+: do NOT follow the GOEXPERIMENT instructions in this step.** In Go 1.27, `systemcrypto` is the default for `CGO_ENABLED=0` and `CGO_ENABLED=1`, and `GOEXPERIMENT=systemcrypto` / `nosystemcrypto` are rejected by the toolchain. Instead:
> - **Remove** every `GOEXPERIMENT=systemcrypto`, `GOEXPERIMENT=nosystemcrypto` and `GOEXPERIMENT=ms_nocgo_opensslcrypto` assignment (scripts, Dockerfiles, Makefiles).
> - **Keep** `MS_GO_NOSYSTEMCRYPTO=1` where it already exists (npm Dockerfiles, `npm.sh`, root Makefile `all-binaries`).
> - Validate with the Go 1.27+ check in Step 2.
>
> The rest of this step applies to **Go 1.26 only**.

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
- `.pipelines/build/scripts/npm.sh` ← **exception:** keeps `export MS_GO_NOSYSTEMCRYPTO=1`, so add no GOEXPERIMENT
- `.pipelines/build/scripts/dropgz.sh`
- `.pipelines/build/scripts/azure-ipam.sh`
- `.pipelines/build/scripts/azure-ip-masq-merger.sh`
- `.pipelines/build/scripts/azure-iptables-monitor.sh`
- `.pipelines/build/scripts/ipv6-hp-bpf.sh`
- `.pipelines/build/scripts/cilium-log-collector.sh`

#### Component + pipeline Dockerfiles

Each Dockerfile that sets `CGO_ENABLED` in a `RUN go build` must have `ENV GOEXPERIMENT=<value>` set BEFORE the build stage:

```dockerfile
# For CGO_ENABLED=0 stages:
ENV GOEXPERIMENT=<value_for_cgo0>
RUN CGO_ENABLED=0 go build ...

# For CGO_ENABLED=1 stages:
ENV GOEXPERIMENT=<value_for_cgo1>
RUN CGO_ENABLED=1 go build ...
```

Dockerfiles to update (component dir + matching `.pipelines/build/dockerfiles/*.Dockerfile`):
- `cni/Dockerfile`
- `cns/Dockerfile`
- `azure-ipam/Dockerfile`
- `azure-ip-masq-merger/Dockerfile`
- `azure-iptables-monitor/Dockerfile`
- `cilium-log-collector/Dockerfile`

#### Standalone Dockerfiles

- `bpf-prog/ipv6-hp-bpf/linux.Dockerfile`
- `npm/linux.Dockerfile` and `npm/windows.Dockerfile` ← **exception:** keep `MS_GO_NOSYSTEMCRYPTO=1` on the `go build` line, so add no GOEXPERIMENT

#### Root Makefile

⚠️ **Keep `all-binaries: export MS_GO_NOSYSTEMCRYPTO := 1`.** Those release binaries run on arbitrary hosts, so they must not depend on Microsoft's FIPS OpenSSL.

The root `Makefile` has CGO_ENABLED=0 build lines for local development. Add a variable and apply it inline:

```makefile
# GOEXPERIMENT for CGO_ENABLED=0 Linux builds (MS Go FIPS requirement)
ACN_GOEXPERIMENT ?= <value_for_cgo0>

# Apply to each CGO_ENABLED=0 build line:
GOEXPERIMENT=$(ACN_GOEXPERIMENT) CGO_ENABLED=0 go build ...
```

**Do NOT `export GOEXPERIMENT` globally** -- it breaks tool builds that don't recognize the experiment.

#### Component-Specific Makefiles

- **`cilium-log-collector/Makefile`** — The `cilium-log-collector-binary` target uses `-buildmode=c-shared` which **implicitly requires CGO_ENABLED=1**. You MUST add explicit `CGO_ENABLED=1 GOEXPERIMENT=<value_for_cgo1>` to the build command:
  ```makefile
  # BEFORE (missing GOEXPERIMENT):
  cd $(CILIUM_LOG_COLLECTOR_DIR) && go build -buildmode=c-shared ...
  
  # AFTER (correct):
  cd $(CILIUM_LOG_COLLECTOR_DIR) && CGO_ENABLED=1 GOEXPERIMENT=<value_for_cgo1> go build -buildmode=c-shared ...
  ```
  **Do NOT skip this file** — `-buildmode=c-shared` implies CGO but doesn't explicitly set `CGO_ENABLED=1`, so grep for `CGO_ENABLED` won't find it. Search for `-buildmode=c-shared` as well.

### Tools Module (`tools.go.mod`)

The tools module lives at the repo root as `tools.go.mod` / `tools.go.sum`, used with `-modfile=tools.go.mod`.
**Do NOT move it to `tools-go/`.** That migration is not required: Go 1.26 and 1.27 both resolve `go tool -modfile=tools.go.mod ...` and `go mod tidy -modfile=tools.go.mod` correctly. Moving it adds a new module that CI's govulncheck matrix check rejects, and it changes many unrelated Makefiles.

- Update only the `toolchain` directive: `go mod edit -modfile=tools.go.mod -toolchain=go1.XX.Y`. `hack/scripts/update-go-toolchain.sh` already does this.
- Do NOT run `go mod tidy` (it times out in the agent environment).

---

## Step 2: Validate

After making all changes:

1. **Root go.mod check** — Verify the root `go.mod` preserves the compatibility
   floor and selects the intended toolchain:
   ```bash
   head -7 go.mod
   ```
2. `go build ./...` — Verify compilation succeeds (all binaries)
3. `go vet ./...` — Check for deprecated API usage
4. `docker build` (spot-check) — Verify at least one container image builds:
   ```bash
   # Quick validation that Dockerfiles + GOEXPERIMENT produce working images
   docker build -f cni/Dockerfile -t acn-cni-test --build-arg VERSION=test .
   ```
5. **Update the image pins directly** in every component Dockerfile (`cni/Dockerfile`, `cns/Dockerfile`, `azure-ipam/Dockerfile`, etc.) and `.pipelines/build/dockerfiles/*.Dockerfile` -- the Dockerfiles are the source of truth. Keep the form `image:tag@sha` (not `image@sha`) so Dependabot can update them. Resolve each digest with `skopeo inspect docker://<image> --format "{{.Digest}}"`, or read the pre-cached `.github/image-digests/*.txt` (`go-image.txt`, `mariner-core.txt`, `mariner-distroless.txt`, `windows-hpc.txt`). Never hand-write or guess a digest.
5. Do NOT run `go mod tidy` — it times out. Existing go.sum files remain valid for version bumps.
6. Verify no new `replace` directives are needed
7. **Dev environment check:**
   ```bash
   grep "VARIANT" .devcontainer/Dockerfile  # Must show target version
   ```
8. **FIPS validation** — run the check that matches the target version.

**Go 1.27+:** no crypto GOEXPERIMENT may remain in effective (non-comment) build configuration. This must print nothing:

```bash
grep -R -nE '^[^#]*GOEXPERIMENT=(systemcrypto|nosystemcrypto|ms_nocgo_opensslcrypto)' \
  --include='*.sh' --include='Makefile' --include='*.Dockerfile' --include='Dockerfile' \
  --exclude-dir=.git --exclude-dir=vendor .
```

**Go 1.26 only:**

```bash
# Verify ALL CGO_ENABLED=0 scripts have the correct GOEXPERIMENT
# (scripts that opt out with MS_GO_NOSYSTEMCRYPTO=1, e.g. npm.sh, are exempt)
for script in .pipelines/build/scripts/*.sh; do
  if grep -q "CGO_ENABLED=0" "$script" && ! grep -q "MS_GO_NOSYSTEMCRYPTO=1" "$script"; then
    if ! grep -q "GOEXPERIMENT=<value_for_cgo0>" "$script"; then
      echo "MISSING GOEXPERIMENT in: $script"
    fi
  fi
done

# Verify ALL CGO_ENABLED=0 Dockerfiles have it
for df in $(find . \( -name 'Dockerfile' -o -name '*.Dockerfile' \) | grep -v vendor); do
  if grep -q "CGO_ENABLED=0" "$df"; then
    if ! grep -q "GOEXPERIMENT=<value_for_cgo0>" "$df"; then
      echo "MISSING GOEXPERIMENT in: $df"
    fi
  fi
done
```

7. Cross-check every item in your Change Plan was actually applied

8. **Completeness validation** — verify ALL version sources updated:
   ```bash
   TARGET_TOOLCHAIN="1.XX.Y"
   TARGET_MINOR="1.XX"
   
   # Root and every submodule must select the target toolchain.
   grep "^toolchain go$TARGET_TOOLCHAIN" go.mod || echo "FAIL: root toolchain not updated!"
   
   # .devcontainer must reference new version
   grep -q "VARIANT=\"$TARGET_MINOR\"" .devcontainer/Dockerfile || echo "FAIL: .devcontainer not updated!"
   
   # Every go.mod in the repo (discovered, not hand-listed)
   git ls-files '*go.mod' ':!:**/vendor/**' | while read -r mod; do
     grep -q "^toolchain go$TARGET_TOOLCHAIN$" "$mod" || echo "FAIL: $mod toolchain not updated!"
   done
   
   # tools module
   grep "^toolchain go$TARGET_TOOLCHAIN" tools.go.mod || echo "FAIL: tools.go.mod toolchain not updated!"
   
   # component Dockerfile golang pin
   grep -q "golang:$TARGET_MINOR" cni/Dockerfile || echo "FAIL: cni/Dockerfile golang pin not updated!"
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

### Backport to `release/v1.8`

**Every Go version change on master MUST be backported to `release/v1.8`.**

1. Check out `release/v1.8`
2. Apply same version/SHA changes
3. If release branch is missing GOEXPERIMENT prerequisites, add those too
4. Do NOT run `go mod tidy` — existing go.sum remains valid for version bumps
5. Update the `FROM ...@sha` pins directly in the component and `.pipelines/build/dockerfiles/` Dockerfiles (resolve digests with `skopeo`; never guess)
6. Title: `chore(release/v1.8): upgrade Go <OLD> → <NEW>`

---

## Architecture Notes

### Image Pinning
- `go-version-check.yaml` and `copilot-setup-steps.yml` read the current Go builder tag from `cni/Dockerfile`
- Dockerfiles are the source of truth; base images are pinned as `image:tag@sha` directly in each `FROM`
- Dependabot keeps the pinned SHAs (and clean-semver tags) current; resolve a digest manually with `skopeo inspect docker://<image> --format "{{.Digest}}"`
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
- Cross-reference with release branches (`release/v1.8`, etc.) that map to AKS release trains
### Important Notes

- **ALWAYS use `1.XX.1` in go.mod** — NOT the latest patch. The container image provides the actual binary version.
- The `npm/` component is released as **npm-lite** — ensure Dockerfiles build correctly
- npm Dockerfiles use **plain Go tags** (e.g., `golang:1.26.4`) without `-azurelinux3.0` suffix
- npm Dockerfiles and `npm.sh` keep `MS_GO_NOSYSTEMCRYPTO=1` (Ubuntu runtime, no Microsoft FIPS OpenSSL). Never remove it.
- Base image tags/SHAs in the Dockerfiles are kept current by Dependabot
- ALWAYS use 2-part floating tags for the azurelinux Go builder in the component + pipeline Dockerfiles
- **Windows builds**: CNG backend typically works without CGO or GOEXPERIMENT — verify per version
- **Do NOT assume "no GOEXPERIMENT" is safe** — always verify the default backend's CGO requirements

### Working Directory (MANDATORY — read before you edit anything)

**If `$GITHUB_WORKSPACE` is set, work there instead of a worktree.** Only that path is committed and pushed; edits made anywhere else are silently discarded.

Verify before you start, and run every later command from there:

```bash
set -euo pipefail
: "${GITHUB_WORKSPACE:?not set — do not fall back to \$HOME}"
cd "$GITHUB_WORKSPACE"
[ "$(git rev-parse --show-toplevel)" = "$PWD" ] || {
  echo "FATAL: $PWD is not the git toplevel"; exit 1
}
```

### Pre-Submit Cleanup (MANDATORY)

**Before committing or updating a PR, you MUST audit your working tree for build artifacts and binaries.** Go builds can produce binaries in module directories that must NOT be committed.

```bash
# Check for untracked or modified binary files
git status --short | grep -E '^\?\?' | while read -r _ path; do
  if file "$path" | grep -qE 'ELF|executable|shared object'; then
    echo "ERROR: Build artifact detected: $path"
    rm -f "$path"
  fi
done

# Verify no binaries are staged
git diff --cached --name-only --diff-filter=A | while read -r path; do
  if [ -f "$path" ] && file "$path" | grep -qE 'ELF|executable|shared object'; then
    echo "ERROR: Binary staged for commit: $path — unstage it"
    git reset HEAD "$path"
    rm -f "$path"
  fi
done
```

**Known binary locations to NEVER commit:** `azure-ip-masq-merger/azure-ip-masq-merger`, `azure-iptables-monitor/azure-iptables-monitor`, `cilium-log-collector/cilium-log-collector`, `tools/azure-npm-to-cilium-validator/azure-npm-to-cilium-validator`, and any other ELF binary produced by `go build`.

### Final Verification Before Reporting Done (MANDATORY)

**Never write a PR description describing changes you have not committed.** Before your final `report_progress` / PR update, prove the branch is non-empty:

```bash
set -euo pipefail
: "${GITHUB_WORKSPACE:?not set — do not fall back to \$HOME}"
cd "$GITHUB_WORKSPACE"
[ "$(git rev-parse --show-toplevel)" = "$PWD" ] || {
  echo "FATAL: $PWD is not the git toplevel"; exit 1
}

# Compare against the branch you are targeting, not always master — a
# release/v1.7 backport must be diffed against release/v1.7.
BASE="origin/${GITHUB_BASE_REF:-master}"

git status --short
git diff --stat "$BASE"...HEAD

if git diff --quiet "$BASE"...HEAD; then
  echo "FATAL: branch has no changes vs $BASE — do not claim the upgrade is done."
  echo "Re-check that you edited files under \$GITHUB_WORKSPACE and committed them."
  exit 1
fi
```

A Go minor upgrade touches roughly 40-50 files. If `git diff --stat` shows zero (or only a handful), something went wrong — **say so explicitly in the PR description instead of describing work that was not committed.** If a command failed due to a network/DNS block, report the exact blocked command rather than silently giving up.
