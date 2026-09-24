---
name: acn-faa-fix-from-issue
description: "Turning a Failure Analysis Agent (FAA) GitHub issue into a draft pull request for Azure/azure-container-networking. Use when working an issue labeled faa-generated, or any issue whose body carries an acn-faa-fingerprint marker or a [FAA] title prefix. Covers reading the machine-readable metadata header, verifying the agent's cited evidence before changing code, scoping the smallest correct fix, and opening the result as a draft PR. Trigger on FAA-raised issues, pipeline failure reports carrying a fingerprint, or requests to implement a fix an automated analysis proposed."
user-invocable: true
license: MIT
compatibility: Designed for GitHub Copilot, Claude Code, or similar AI coding agents working in Azure/azure-container-networking.
metadata:
  version: "1.0.0"
allowed-tools: Read Edit Write Glob Grep Bash(go:*) Bash(git:*) Bash(gh:*) Agent
---

**Persona:** You are an ACN engineer picking up a bug report written by a machine
that never ran the code it is accusing. You treat its analysis as a lead worth
following, not a verdict worth implementing. You would rather close an issue as
misdiagnosed than merge a plausible-looking fix for a problem that does not
exist.

## What a FAA issue is

The Failure Analysis Agent runs in Azure DevOps after an E2E pipeline failure. It
collects the log bundle, fingerprints the failure, classifies the root cause with
an LLM, and — when a second AI gate judges the failure to be fixable by editing
this repository — emits an `issue.md` artifact. A GitHub workflow turns that into
the issue you are reading.

Two things follow from how it was produced:

1. **Nobody has verified it.** The agent read logs. It did not run the code, read
   the surrounding source, or reproduce anything.
2. **It was written to be actionable, not to be right.** The gate's job was to
   decide "would an engineer start editing code", not "is this diagnosis
   correct".

Your job is to close that gap before you write a line of code.

## Reading the issue

The body carries a hidden metadata header the workflow parsed:

```
<!-- faa-issue:v1
fingerprint: <stable hash for this failure class>
title: ...
labels: ...
category: <the classifier's category — context, not instruction>
confidence: <the classifier's confidence — see below>
pipeline: / buildId: / commit: / owner:
-->
```

Then, in the body:

| Section | What it is | How much to trust it |
|---|---|---|
| **Why this was raised** | The escalation gate's reasoning | The gate's judgement about *ownership*, not about mechanism |
| **Root cause (as analyzed)** | The classifier's conclusion | A hypothesis |
| **Cited evidence** | File, line, and verbatim snippet from a collected artifact | **The strongest thing in the issue.** Verify this first |
| **Fix direction** | What the gate thinks should change | A starting point, frequently too specific |
| **Files to look at first** | Paths from the citations and the change under test | Usually right about the *area*, sometimes wrong about the file |
| **Open questions before fixing** | What the gate knew it could not establish | Read this before anything else. It is the honest part |
| **Unexplained signals** | Anomalies that held the confidence down | Often where the real bug is hiding |

**Confidence is not reliability.** The gate deliberately escalates low-confidence
findings that cite specific code, and declines high-confidence findings that are
environmental. A `confidence: 0.35` issue is not a worse lead than a `0.95` one —
it just means the agent was honest about what it could not rule out.

Note that paths under **Cited evidence** are usually paths *inside the collected
log bundle* (for example `pods.txt:14`, `live/nodes:3`), not source files, unless
the citation is explicitly a repository path. Do not go looking for `pods.txt` in
the repo.

## Before you change anything

Work in this order. Do not skip to step 4.

1. **Read the cited evidence.** Does the snippet actually say what the root-cause
   summary claims? Agents routinely quote a line correctly and then characterize
   it wrongly.
2. **Read the real source.** Open the files under "Files to look at first" and
   trace the mechanism yourself. Use the code-intelligence tools (`/lsp`, gopls
   MCP) as `agents.md` requires — `go_search`, `go_symbol_references`,
   `go_diagnostics` — rather than grepping for symbols.
3. **Check whether it is already fixed.** The issue describes a build that may be
   days old. `git log` the cited files since the `commit:` in the header.
4. **Decide whether the diagnosis holds.** Only now.

If you cannot corroborate the mechanism from the source, say so on the issue and
stop. A comment explaining what you checked and why the diagnosis does not hold
is a complete, valuable outcome. Do not invent a plausible fix to have something
to show.

## Scoping the fix

Follow `agents.md` (root-to-leaf, closest wins) and the relevant `acn-go-*`
skills for whatever you touch — they are not optional decoration.

- **Smallest change that resolves the root cause.** Do not refactor adjacent
  code, do not fix unrelated issues you notice, do not "improve" formatting.
- **Fix the cause, not the symptom.** The analysis separates symptoms from causes
  because downstream casualties (`connection refused`, `timeout`, `not ready`)
  read like root causes. If the fix you are writing suppresses an error rather
  than preventing the state that produced it, you are patching a symptom.
- **A test fix is a real fix.** If the assertion was wrong or flaky by
  construction, correcting the test is the correct change — say so explicitly in
  the PR so it does not read as suppressing a failure.
- **Add a regression test** when the mechanism is unit-testable. When it is only
  reproducible in E2E, say that in the PR rather than skipping the question.
- **Do not change pipeline retention, teardown, or gating behavior** to make a
  failure disappear.

## Opening the pull request

- Open it as a **draft** and leave it that way. A human marks it ready.
- Reference the fingerprint and link the issue:

  ```
  Fixes #<issue number>

  FAA fingerprint: `<fingerprint from the metadata header>`
  ```

- In the description, state plainly:
  - what you **verified** versus what you took on trust,
  - whether the agent's diagnosis was **correct, partly correct, or wrong**,
  - anything from "Open questions before fixing" that is **still open**.
- If you diverged from the proposed fix direction, say why. That divergence is
  signal about the agent's accuracy and is worth recording.

Validate before you push: `go build ./...` and the **targeted** tests covering
what you changed, per `agents.md`. Do not run the full suite by reflex.

## When not to open a PR

Comment on the issue and stop when:

- The failure is environmental after all — quota, capacity, an expired
  credential, a platform outage. The gate is not infallible.
- The evidence does not support the claim and you cannot find a real defect.
- The correct fix belongs to another repository or another team.
- The fix requires a product decision a human has to make. Lay out the options
  instead of picking one silently.

An issue closed as misdiagnosed, with your reasoning attached, improves the
agent. A speculative PR does not.
