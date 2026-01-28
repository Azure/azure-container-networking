# AI Agent Instructions (GitHub Copilot)

Use this file together with existing project guidance. The baseline behavioral rules are in [CLAUDE.md](CLAUDE.md) and must be followed.

## Audience and goals
- Primary audience: GitHub Copilot.
- Goal: make minimal, correct changes with high confidence and strong verification.

## Scope and ownership map
- Active modules: cni, cns, azure-ipam.
- Deprecated: npm.
- Libraries (not deployed): zapai, dropgz.
- Most other areas are legacy; avoid touching unless explicitly required.

## Coding guidelines
- Follow the Uber Go style guide.
- Prefer `zap` for structured logging.
- Prefer `pkg/errors` for error handling.
- Keep changes surgical and aligned to the request; do not refactor unrelated code.

## Testing and validation
- Always run `go test` after changes.
- Run `golangci-lint` when changes are in Go.
- If you cannot run tests, say so and explain why.

## Build and tooling
- Build commands are documented in [README.md](README.md) (see Make targets such as `make all-binaries`).

## Repo hygiene and compliance
- Do not modify .github or .pipelines unless the user explicitly asks.
- Respect contribution policies in [CONTRIBUTING.md](CONTRIBUTING.md) and related docs in docs/contributing/.

## PR hygiene (when applicable)
- Keep PRs small and focused on a single change.
- Include clear descriptions and link issues when available.
- Do not reference private/internal resources.

## When in doubt
- Ask clarifying questions before coding.
- Prefer the simplest workable approach.

## Component briefs
- CNS: cns/agents.md