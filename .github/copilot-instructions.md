Use all `agents.md` files found from the repository root to the current directory as instructions and context, applying them in root-to-leaf order; if instructions conflict, the `agents.md` closest to the current directory takes precedence. Use relevant repo skills from `.github/skills/` when applicable.

# Copilot Agent Instructions for Azure Container Networking

## Go Version Strategy

ACN uses **floating minor version tags** for the Go build image (`build/images.mk`). This means:
- The `GO_IMG` in `build/images.mk` uses a **2-part minor version** tag (e.g., `golang:1.26-azurelinux3.0`)
- The floating tag resolves to the latest patch via SHA digest at `make dockerfiles` time
- **Digest refreshes** (same tag, new SHA): update SHAs in `install-go.sh`, `bpf-prog` Dockerfile, and run `make dockerfiles`. If the Go version inside the image changed, also update `go.mod`/`tools.go.mod`.
- **Patch bumps** (e.g., new `1.26.3` patch available): update `go.mod`, `tools.go.mod`, `npm/` Dockerfiles (which pin exact patch tags), and all SHAs
- **Minor/major version bumps** (e.g., `1.26` → `1.27`) require all file changes below

### Version Sources (ALL must be updated)

The Go version is declared in **multiple places** that must stay in sync:
```
build/images.mk (GO_IMG=golang:1.XX-azurelinux3.0)     ← primary tag
    ├── → go.mod (go 1.XX or 1.XX.Y)                   ← must match
    ├── → tools.go.mod (go 1.XX or 1.XX.Y)             ← must match
    ├── → .devcontainer/Dockerfile (VARIANT="1.XX")
    ├── → .pipelines/build/scripts/install-go.sh (DEFAULT_IMAGE SHA)
    ├── → bpf-prog/ipv6-hp-bpf/linux.Dockerfile (Go image SHA)
    ├── → npm/linux.Dockerfile (tag 1.XX.Y)
    ├── → npm/windows.Dockerfile (tag 1.XX.Y)
    └── → All .tmpl Dockerfiles (via `make dockerfiles`)

Independent modules (check separately):
    ├── → cilium-log-collector/go.mod (may have its own Go version)
    └── → Any other go.mod in subdirectories
```

When upgrading, update `build/images.mk` first, then propagate to ALL downstream files including `tools.go.mod`.

## Go Version Upgrade Procedure

When assigned an issue to upgrade the Go version in this repository, follow this procedure exactly.

### Step 0: Research the Target Version

**CRITICAL: Before making ANY code changes, read these sources:**

1. **MS Go version-specific notes:**
   `https://github.com/microsoft/go/blob/microsoft/main/docs/go<MAJOR>.<MINOR>.md`
   (e.g., `https://github.com/microsoft/go/blob/microsoft/main/docs/go1.27.md`)
   → Contains MS-specific breaking changes, new requirements, removed features

2. **MS Go Migration Guide:**
   `https://github.com/microsoft/go/blob/microsoft/main/eng/doc/MigrationGuide.md`
   → FIPS/crypto policy, systemcrypto requirements, runtime dependencies

3. **MS Go FIPS User Guide:**
   `https://github.com/microsoft/go/blob/microsoft/main/eng/doc/fips/UserGuide.md`
   → Runtime library requirements, which base images are needed

4. **MS Go Additional Features:**
   `https://github.com/microsoft/go/blob/microsoft/main/eng/doc/AdditionalFeatures.md`
   → All MS-specific patches and behaviors

5. **Upstream Go release notes:**
   `https://go.dev/doc/go<MAJOR>.<MINOR>` (e.g., `https://go.dev/doc/go1.27`)
   → Language changes, deprecated APIs, stdlib changes

6. **MS Go releases page:**
   `https://github.com/microsoft/go/releases`
   → Check latest patch version and release notes for the target minor

7. **`.github/go-upgrade-rules.yaml`** (if it exists in this repo)
   → Known transition requirements maintained by the team

**Summarize your findings in the PR description**, especially:
- Any new environment variables or build flags required
- Any removed/deprecated flags (e.g., if GOEXPERIMENT=systemcrypto becomes default)
- Any new runtime dependencies (libraries, base image changes)
- Any deprecated stdlib APIs used in this codebase

### Architecture Overview

This repo uses a **template system** for Dockerfiles:
- `build/images.mk` defines `GO_IMG` (Go builder image) and `MARINER_DISTROLESS_IMG` (runtime base image)
- `.tmpl` files in each component directory are rendered into Dockerfiles by `make dockerfiles`
- `make dockerfiles` uses `renderkit` and `skopeo` to resolve image tags to SHA digests
- The signed binary pipeline uses `.pipelines/build/scripts/install-go.sh` which has a `DEFAULT_IMAGE` fallback SHA

### Files to Update (in order)

1. **`build/images.mk`** — Update `GO_IMG` tag (e.g., `golang:1.27-azurelinux3.0`)
   - ALWAYS use 2-part floating tag: `1.27`, never `1.27.0` or `1.27.2`
2. **`go.mod`** — Update `go` directive to the new version (e.g., `go 1.27` for minor bump, or `go 1.27.3` for a specific patch)
3. **`tools.go.mod`** — Update `go` directive to match `go.mod`
4. **Run `go mod tidy`** — Fix any dependency issues
5. **`.pipelines/build/scripts/install-go.sh`** — Update `DEFAULT_IMAGE` SHA
   - Get new SHA: `skopeo inspect docker://mcr.microsoft.com/oss/go/microsoft/golang:<VERSION>-azurelinux3.0 --format "{{.Digest}}"`
6. **`bpf-prog/ipv6-hp-bpf/linux.Dockerfile`** — Update Go image SHA (NOT template-managed)
   - Uses MCR image: `mcr.microsoft.com/oss/go/microsoft/golang@sha256:...`
7. **`npm/linux.Dockerfile`** and **`npm/windows.Dockerfile`** — Update Go tag (NOT template-managed)
8. **`.devcontainer/Dockerfile`** — Update `VARIANT` arg
9. **Run `make dockerfiles`** — Regenerate all template-based Dockerfiles

### FIPS / System Crypto

Check the MS Go docs (Step 0) for the current FIPS requirements for your target version.

**As of Go 1.26**, Microsoft's Go fork requires:

- **GOEXPERIMENT=systemcrypto** must be set in:
  - All `.tmpl` Dockerfile templates (as `ENV GOEXPERIMENT=systemcrypto` after the builder FROM line)
  - All `.pipelines/build/scripts/*.sh` pipeline scripts (as `export GOEXPERIMENT=systemcrypto`)
  - `bpf-prog/ipv6-hp-bpf/linux.Dockerfile`
  - `npm/linux.Dockerfile`

- **Runtime base image** must include crypto libraries:
  - `MARINER_DISTROLESS_IMG` in `build/images.mk` must be `distroless/base` (NOT `distroless/minimal`)
  - `bpf-prog` runtime stage must use `azurelinux/distroless/base:3.0`

- **Remove** `MS_GO_NOSYSTEMCRYPTO=1` if present in any Dockerfile

**IMPORTANT:** These requirements may change in future versions. For example:
- Go 1.27+ might make `systemcrypto` the default (remove the explicit GOEXPERIMENT)
- New env vars might be introduced
- Different crypto libraries might be required

**Always check the MS Go docs for your target version before assuming the current FIPS setup is correct.**

### Validation Steps

After making all changes:

1. `go build ./...` — Verify compilation
2. `go vet ./...` — Check for issues
3. `make dockerfiles` — Ensure templates render correctly
4. `go mod tidy` — Ensure deps are clean
5. Verify no `replace` directives are needed for incompatible deps
6. Check that ARM and AMD builds both work (inspect Dockerfile multi-arch support)

### PR Guidelines

- Title: `chore: upgrade Go <OLD> → <NEW>`
- Reference the tracking issue in the PR body
- Include a summary of findings from Step 0 (MS Go docs research)
- List all files modified
- If FIPS/crypto changes were needed, call them out explicitly
- If FIPS/crypto requirements CHANGED from the previous version, highlight this prominently

### Backport to `release/v1.7`

**Every Go version change on master MUST be backported to `release/v1.7`.**

After the master PR is complete:
1. Check out `release/v1.7`
2. Apply the same version/SHA changes
3. If `release/v1.7` is missing prerequisites (GOEXPERIMENT, distroless/base), add those too
4. Run `go mod tidy` — the release branch may have different deps, resolve conflicts separately
5. Run `make dockerfiles`
6. Create a separate PR: `chore(release/v1.7): upgrade Go <OLD> → <NEW>`

**Prerequisites check:** Before backporting, verify `release/v1.7` has:
- `GOEXPERIMENT=systemcrypto` in templates and scripts (for Go 1.26+)
- `distroless/base` as the runtime image (for Go 1.26+)
- If missing, include these in the backport PR as well

### Important Notes

- The `npm/` component is no longer being released — update its Dockerfiles but don't worry about testing
- `cilium-log-collector` uses `CGO_ENABLED=1` (exception to the norm)
- The `baseimages.yaml` CI workflow will fail if `make dockerfiles` output doesn't match committed Dockerfiles
- ALWAYS use 2-part floating tags in `build/images.mk` (e.g., `1.27`, not `1.27.0`) — patches come via digest refresh
- The `go.mod` and `tools.go.mod` directives should use the Go version that matches the toolchain being built (check what the automation workflow sets — currently uses the patch version when doing patch bumps, minor version for minor upgrades)

## Maintaining These Instructions

This file should be updated when:
- A new Go version introduces new MS-specific requirements
- The repo's build system changes (new files, new tools)
- FIPS requirements change (new flags, removed flags, new base images)

The automation workflow (`.github/workflows/go-version-check.yaml`) creates issues
that reference this file. Keep it accurate so the Copilot agent can execute upgrades correctly.
