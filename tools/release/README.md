# ACN Release CLI

A Go CLI tool (`release-cli`) used by the [Scheduled Release workflow](../../.github/workflows/scheduled-release.yaml) to automate monthly ACN releases.

## Architecture

```
.github/workflows/scheduled-release.yaml     ← Orchestrator (cron + manual dispatch)
         │
         │  builds & invokes
         ▼
tools/release/cmd/release-cli/main.go        ← CLI entrypoint (7 commands)
         │
         ├── internal/github/github.go       ← PR collection + wait logic
         ├── internal/version/version.go     ← Semver + binary change detection
         ├── internal/pipeline/pipeline.go   ← ADO pipeline monitoring
         └── internal/notify/notify.go       ← Teams notifications (OIDC)
```

## Workflow

```
┌───────────────────────────────────────────────────────────┐
│  check_cadence                                            │
│  Is this the first Friday of the month? (skip otherwise)  │
└────────────────────────────┬──────────────────────────────┘
                             ▼
┌───────────────────────────────────────────────────────────┐
│  Step 1: wait_dependabot                                  │
│                                                           │
│  CLI commands used:                                       │
│    • collect-prs  → find open dependabot/Go upgrade PRs   │
│    • next-version → calculate upcoming tag                │
│    • detect-binaries → list binary tags to create         │
│    • wait-prs    → poll until PRs merge or are skipped    │
│    • notify      → post to Teams CNI channel              │
│                                                           │
│  Creates a tracking issue with skip/pause/resume commands │
└────────────────────────────┬──────────────────────────────┘
                             ▼
            ┌────────────────┴────────────────┐
            ▼                                 ▼
┌────────────────────┐    ┌─────────────────────────────────┐
│ Step 2: vuln_check │    │ Step 2b-d: binary tagging       │
│                    │    │                                 │
│ govulncheck on all │    │ detect-binaries → tag_binaries  │
│ go.mod directories │    │ (dropgz, azure-ipam, etc.)      │
│                    │    │ Auto-bump patch + push tags      │
│ notify on failure  │    │ Post summary to Teams            │
└─────────┬──────────┘    └─────────────────────────────────┘
          ▼
┌───────────────────────────────────────────────────────────┐
│ Step 3: create_tag                                        │
│                                                           │
│ CLI: next-version → dispatches create-release-tag.yml     │
│ (requires tag-approval environment gate)                  │
└────────────────────────────┬──────────────────────────────┘
                             ▼
┌───────────────────────────────────────────────────────────┐
│ Step 4: validate_pipelines                                │
│                                                           │
│ CLI: wait-pipeline (×2)                                   │
│   • Waits for tag to appear (human approval gate)         │
│   • Discovers ADO runs auto-triggered by tag push         │
│   • Polls until pipelines complete (Bearer token, OIDC)   │
│   • Posts results to Teams                                │
└────────────────────────────┬──────────────────────────────┘
                             ▼
┌───────────────────────────────────────────────────────────┐
│ Step 5: release_channel_post                              │
│                                                           │
│ CLI: notify                                               │
│   • Posts release thread (signed/unsigned checklists)     │
│   • Generates R2D drafts (signed + unsigned paths)        │
│   • GitHub step summary with remaining human actions      │
└───────────────────────────────────────────────────────────┘
```

## CLI Commands

| Command | Description | Used by |
|---------|-------------|---------|
| `collect-prs` | Lists open dependabot + Go upgrade PRs on a branch | Step 1 |
| `next-version` | Finds latest semver tag, bumps patch (e.g. v1.7.21 → v1.7.22) | Steps 1, 3 |
| `detect-binaries` | Checks which binaries have changes since last tag | Steps 1, 2b |
| `wait-prs` | Polls PRs until all merge or are skipped via tracking issue | Step 1 |
| `wait-pipeline` | Discovers + waits for ADO pipeline runs triggered by a tag | Step 4 |
| `notify` | Posts/updates Teams adaptive card via acn-notifier-bot | All steps |
| `notify-reply` | Posts a reply under an existing release card | Step 5 |

## File Descriptions

### `cmd/release-cli/main.go`

CLI entrypoint using [cobra](https://github.com/spf13/cobra). Defines all 7 commands, validates inputs, and delegates to internal packages. Outputs JSON to stdout for consumption by workflow steps.

### `internal/github/github.go`

Interacts with GitHub via the `gh` CLI (no SDK dependency). Key logic:
- **CollectPRs**: Lists dependabot PRs (`--author app/dependabot`) and Go upgrade PRs (detected by title/branch/label patterns)
- **WaitPRs**: Polling loop that checks PR states + parses tracking issue comments for `skip`, `pause`, `resume` commands
- Supports: `skip #N`, `skip all`, `pause`, `resume`, `pause #N`, `unpause #N`

### `internal/version/version.go`

Git-based version detection. Key logic:
- **NextVersion**: Finds latest `vX.Y.Z` tag merged into the branch, bumps patch
- **DetectBinaryChanges**: For each binary (dropgz, azure-ipam, etc.), checks if files changed since its last tag
- **resolveRef**: Handles CI checkout quirk where local branches don't exist (falls back to `origin/<ref>`)
- Uses `git tag --merged <SHA>` with resolved refs for CI compatibility

### `internal/pipeline/pipeline.go`

ADO pipeline monitoring via REST API. Key logic:
- **WaitForRun**: Two-phase polling — first discovers a run matching `refs/tags/<tag>`, then waits for completion
- Uses Bearer token from OIDC (`az account get-access-token --resource 499b84ac-...`)
- Polls builds list every 30s, checks status every 2min
- Token field has `json:"-"` to prevent accidental logging

### `internal/notify/notify.go`

Teams notification client. Key logic:
- Posts to `acn-notifier-bot` Azure Function (Adaptive Cards in Teams channels)
- Authenticates via OIDC: acquires token for the Function App's audience
- Env vars: `NOTIFIER_URL`, `NOTIFIER_AUDIENCE`, `TEAMS_TEAM_ID`

## Authentication

All authentication uses **OIDC federated credentials** — no PATs or long-lived secrets:

| Service | Method | Identity |
|---------|--------|----------|
| GitHub API | `GITHUB_TOKEN` (automatic) | Workflow token |
| Teams notifications | OIDC → acn-notifier-bot audience | `acn-release-bot` SP |
| ADO pipeline API | OIDC → ADO resource ID | `acn-release-bot` SP |

## Building

```bash
cd tools/release
go build -o release-cli ./cmd/release-cli/
```

## Dependencies

- `github.com/spf13/cobra` — CLI framework
- No other external dependencies
- Relies on `gh` CLI and `az` CLI being available at runtime (both pre-installed on GitHub Actions runners)
