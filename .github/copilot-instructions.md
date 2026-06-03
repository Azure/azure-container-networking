Use all `agents.md` files found from the repository root to the current directory as instructions and context, applying them in root-to-leaf order; if instructions conflict, the `agents.md` closest to the current directory takes precedence. Use relevant repo skills from `.github/skills/` when applicable.

# Copilot Instructions for Azure Container Networking

## Available Skills

For task-specific guidance, use the appropriate skill:

| Skill | When to use |
|-------|-------------|
| `acn-go-version-bump` | Go version upgrades, FIPS/systemcrypto config, Dockerfile template updates, CVE patches |
| `acn-go-errors-logging` | Error string formatting, zap logging discipline |
| `acn-go-context-lifecycle` | Context propagation and lifecycle management |
| `acn-go-design-boundaries` | Package boundaries and design patterns |
| `acn-go-interfaces-dependencies` | Interface design and dependency injection |
| `acn-go-types-parsing` | Type definitions and parsing logic |
| `acn-go-http-api-contracts` | HTTP API contracts and handlers |
| `acn-go-platform-abstraction` | Platform-specific abstractions |
| `acn-go-control-plane-contracts` | Control plane contracts |

## General Guidelines

- This repository builds container networking components for Azure (CNI, CNS, NPM, etc.)
- Build system uses `make dockerfiles` with template rendering via `renderkit`
- Tools module lives at `tools-go/go.mod` (separate from root `go.mod`)
- CI uses both GitHub Actions and Azure Pipelines (`.pipelines/`)
- `cilium-log-collector` is the only component using `CGO_ENABLED=1` (with `GOEXPERIMENT=systemcrypto`)
- All other components use `CGO_ENABLED=0` with `GOEXPERIMENT=ms_nocgo_opensslcrypto` (Go 1.26)

## AI Documentation

See `.github/AI-DOCS.md` for details on how AI instruction files are structured in this repo.
