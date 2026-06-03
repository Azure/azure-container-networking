---
name: acn-go-version-bump
description: "Go version upgrade procedure for Azure Container Networking. Use when upgrading Go minor/patch versions, bumping MS Go toolchain, fixing FIPS/systemcrypto configuration, updating Dockerfile templates, or responding to Go CVE patches. Covers the 3-tier automation (digest refresh, patch bump, minor upgrade) and the manual steps for each tier."
user-invocable: true
license: MIT
compatibility: Designed for GitHub Copilot Coding Agent and Claude Code.
metadata:
  author: behzad-mir
  version: "3.0.0"
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
# 1. Version-specific notes (may not exist for all versions)
gh api "repos/microsoft/go/contents/docs/go1.<MINOR>.md?ref=microsoft/main" --jq '.content' | base64 -d

# 2. NocgoOpenSSL — crypto backend selection rules
gh api "repos/microsoft/go/contents/eng/doc/NocgoOpenSSL.md?ref=microsoft/main" --jq '.content' | base64 -d

# 3. Migration Guide — toolchain behavior, breaking changes
gh api "repos/microsoft/go/contents/eng/doc/MigrationGuide.md?ref=microsoft/main" --jq '.content' | base64 -d

# 4. FIPS User Guide — runtime requirements, crypto API behavior
gh api "repos/microsoft/go/contents/eng/doc/fips/UserGuide.md?ref=microsoft/main" --jq '.content' | base64 -d

# 5. Additional Features — all MS-specific patches
gh api "repos/microsoft/go/contents/eng/doc/AdditionalFeatures.md?ref=microsoft/main" --jq '.content' | base64 -d

# 6. Upstream Go release notes
# Fetch from https://go.dev/doc/go1.<MINOR>
```

### Analysis Procedure

After fetching the docs, produce a **Requirements Matrix** before making changes:

#### A. Build Environment Requirements

Analyze docs for:
- [ ] Required `GOEXPERIMENT` values (and under what CGO conditions)
- [ ] Required build flags (`-buildmode`, `-ldflags`, etc.)
- [ ] `GOTOOLCHAIN` behavior changes
- [ ] Any new env vars introduced or deprecated
- [ ] `go mod tidy` behavior changes (stricter validation, new directives)
- [ ] Toolchain download/selection changes

Cross-reference with our repo:
- Check every `.pipelines/build/scripts/*.sh` for CGO_ENABLED value
- Check every `Dockerfile` and `.tmpl` for CGO_ENABLED and build flags
- Determine which components use CGO=0 vs CGO=1

#### B. Runtime Dependencies

Analyze docs for:
- [ ] Required system libraries (libc, libdl, libpthread, libcrypto)
- [ ] Base image requirements (what libraries must be present)
- [ ] Architecture-specific limitations
- [ ] Any new runtime behaviors (telemetry, crypto provider selection)

Cross-reference with our repo:
- Check `MARINER_DISTROLESS_IMG` in `build/images.mk`
- Check all runtime base images in Dockerfiles
- Verify required libraries are present in the base image

#### C. Crypto/FIPS Changes

Analyze docs for:
- [ ] Which crypto backend is selected by default
- [ ] When cgo is required vs optional for crypto
- [ ] Any new FIPS compliance requirements
- [ ] Changes to crypto API behavior (non-FIPS curves, key sizes, etc.)
- [ ] Whether `MS_GO_NOSYSTEMCRYPTO` or similar env vars are deprecated/added

Cross-reference with our repo:
- Map each component to its CGO setting
- Determine correct GOEXPERIMENT per component
- Check if any crypto APIs we use have changed behavior

#### D. Compatibility & Breaking Changes

Analyze docs for:
- [ ] Deprecated stdlib APIs
- [ ] Removed undocumented Windows APIs
- [ ] Module system changes
- [ ] Any changes to `go generate`, `go test`, or tooling

Cross-reference with our repo:
- Run `go vet ./...` after upgrade to catch deprecated API usage
- Check if any `replace` directives need updating
- Verify key dependencies support the new version (controller-runtime, client-go, etc.)

### Output: Change Plan

Before making any code changes, produce a change plan in this format:

```markdown
## MS Go <VERSION> Upgrade — Requirements Analysis

### Source Documents Reviewed
- [ ] docs/go1.XX.md: <summary of key changes>
- [ ] NocgoOpenSSL.md: <crypto backend rules for this version>
- [ ] MigrationGuide.md: <relevant migration steps>
- [ ] fips/UserGuide.md: <runtime requirements>
- [ ] AdditionalFeatures.md: <relevant new features>

### Determinations

| Requirement | Current State | Required State | Action |
|-------------|--------------|----------------|--------|
| GOEXPERIMENT (CGO=1) | ... | ... | ... |
| GOEXPERIMENT (CGO=0) | ... | ... | ... |
| Base image | ... | ... | ... |
| go.mod version | ... | ... | ... |
| Runtime deps | ... | ... | ... |
| New flags | ... | ... | ... |

### Risk Assessment
- Breaking changes that affect this codebase: ...
- Dependencies that may not support new version: ...
- FIPS compliance impact: ...
```

**Only proceed with code changes after the analysis is complete.**

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

Independent modules (check separately):
    ├── → cilium-log-collector/go.mod
    └── → Any other go.mod in subdirectories
```

### Files to Update (in order)

1. **`build/images.mk`** — Update `GO_IMG` tag
   - ALWAYS use 2-part floating tag: `1.27`, never `1.27.0`
2. **`go.mod`** — Update `go` directive (use `.1` minimum, e.g., `go 1.27.1`)
3. **`tools-go/go.mod`** — Update `go` directive to match
4. **Run `go mod tidy`** on both root and `tools-go/`
5. **`.pipelines/build/scripts/install-go.sh`** — Update `DEFAULT_IMAGE` SHA
6. **`bpf-prog/ipv6-hp-bpf/linux.Dockerfile`** — Update Go image SHA
7. **`npm/linux.Dockerfile`** and **`npm/windows.Dockerfile`** — Update Go tag
8. **`.devcontainer/Dockerfile`** — Update `VARIANT` arg
9. **Run `make dockerfiles`** — Regenerate all template-based Dockerfiles

### Apply FIPS/Crypto Changes (from Step 0 analysis)

Based on your Requirements Matrix:
- Set/remove GOEXPERIMENT in each script and Dockerfile per CGO setting
- Update base images if runtime dependencies changed
- Add/remove any new environment variables

### Tools Module

Tools live in `tools-go/go.mod` (module: `github.com/Azure/azure-container-networking/tools-go`):
- Isolates tool dependencies from the main module graph
- Go 1.26's stricter `go mod tidy` requires this separation
- All `-modfile` references point to `tools-go/go.mod`
- Root Makefile: `TOOLS_GO_MOD = $(REPO_ROOT)/tools-go/go.mod`

---

## Step 2: Validate

After making all changes:

1. `go build ./...` — Verify compilation
2. `go vet ./...` — Check for issues (catches deprecated API usage)
3. `make dockerfiles` — Ensure templates render correctly
4. `go mod tidy` — Ensure deps are clean (root and tools-go/)
5. Verify no `replace` directives are needed
6. Check ARM and AMD builds (Dockerfile multi-arch support)
7. Cross-check against Risk Assessment from Step 0

---

## Step 3: PR and Backport

### PR Guidelines

- Title: `chore: upgrade Go <OLD> → <NEW>`
- Reference the tracking issue in the PR body
- **Include the Requirements Matrix from Step 0** in the PR description
- List all files modified
- Highlight any FIPS/crypto requirement changes prominently

### Backport to `release/v1.7`

**Every Go version change on master MUST be backported to `release/v1.7`.**

1. Check out `release/v1.7`
2. Apply same version/SHA changes
3. If release branch is missing prerequisites, add those too
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

### Component CGO Map (current as of Go 1.26)
| Component | CGO_ENABLED | GOEXPERIMENT | Build Mode | Notes |
|-----------|:-----------:|:------------:|:----------:|-------|
| cni, cns, npm, dropgz, azure-ipam, azure-ip-masq-merger, azure-iptables-monitor, ipv6-hp-bpf | 0 | ms_nocgo_opensslcrypto | static binary | Nocgo OpenSSL backend (systemcrypto requires CGO) |
| cilium-log-collector | 1 | systemcrypto | c-shared (.so) | Fluent Bit plugin, requires CGO |

### Important Notes

- Use `.1` as minimum patch version (`.0` is pre-release/stabilization)
- The `npm/` component is no longer released — update but don't worry about testing
- The `baseimages.yaml` CI workflow fails if `make dockerfiles` output doesn't match committed files
- ALWAYS use 2-part floating tags in `build/images.mk`
