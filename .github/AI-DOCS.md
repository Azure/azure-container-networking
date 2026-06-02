# AI Documentation Usage in This Repository

This document describes how AI instruction files are structured in this repository,
their purpose, and guidelines for maintaining them.

## Architecture

This repo uses a **skills-based approach** to AI instructions, following the
[Agent Skills specification](https://agentskills.io/home). This ensures:

- Instructions are **only loaded when relevant** (not polluting every agent interaction)
- Each skill is **focused on a single domain** (Go version bumps, error handling, etc.)
- Skills are **discoverable** by agents and invocable via slash commands

## Directory Structure

```
.github/
├── copilot-instructions.md          ← Minimal global config (points to skills)
├── AI-DOCS.md                       ← This file (human reference)
├── skills/
│   ├── acn-go-version-bump/         ← Go version upgrade procedure + FIPS rules
│   │   └── SKILL.md
│   ├── acn-go-errors-logging/       ← Error/logging discipline
│   │   └── SKILL.md
│   ├── acn-go-context-lifecycle/    ← Context propagation
│   │   └── SKILL.md
│   ├── acn-go-design-boundaries/    ← Package design
│   │   └── SKILL.md
│   └── ...                          ← Other domain-specific skills
└── workflows/
    └── go-version-check.yaml        ← Automation that references skills
```

## How It Works

### Skills (`.github/skills/*/SKILL.md`)

Each skill:
- Has a **name** and **description** in frontmatter for agent discovery
- Contains **detailed, task-specific instructions** (the "how")
- Is only loaded when an agent determines the skill is relevant to the current task
- Can be invoked manually via `/skill-name` in VS Code Chat

**Example:** The `acn-go-version-bump` skill contains the full Go upgrade procedure,
FIPS/GOEXPERIMENT rules, file update order, and validation steps. It's only loaded when
the agent is working on a Go version task — not when debugging a CNI bug.

### Global Instructions (`.github/copilot-instructions.md`)

Minimal file that:
- Lists available skills and when to use them
- Provides repo-wide context (build system, module structure)
- Does NOT contain task-specific procedures

### Workflow Integration

The `go-version-check.yaml` workflow:
1. **Fetches MS Go docs at runtime** from `microsoft/go` repo
2. Determines version-specific FIPS requirements dynamically
3. For Tier 3 (minor upgrades): creates an Issue that references the skill
4. The Copilot Coding Agent reads the skill when executing the issue

## When to Create a New Skill

Create a new skill when:
- A recurring task requires domain-specific knowledge
- The instructions are substantial enough to warrant isolation
- You don't want the instructions loaded for unrelated agent tasks

## When to Update a Skill

- A Go version introduces new MS-specific requirements
- The repo's build system changes
- FIPS/crypto requirements change
- A workflow change affects how agent tasks are executed

## Key Design Decisions

1. **Skills over global instructions**: Prevents context pollution. The Go FIPS rules
   shouldn't be loaded when an agent is fixing a CNI bug.

2. **Runtime doc fetching**: The workflow fetches `microsoft/go` docs dynamically rather
   than hardcoding rules. This means the workflow adapts to new Go versions without
   needing skill updates for every release.

3. **Version-aware FIPS logic**: GOEXPERIMENT requirements differ by Go version AND by
   CGO setting. The workflow and skill both encode this nuance.
