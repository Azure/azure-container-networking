# failure-agent-teams-bot

Bridges the Failure Analysis Agent (FAA) to Microsoft Teams through the shared
**ACN Pipeline Notifier** bot. Every `(NOTIFY_SOURCE, NOTIFY_RUN_ID)` pair renders
one Adaptive Card that updates in place across stages, plus threaded replies.

`NOTIFY_RUN_ID` is `$(Build.BuildId)`, so every stage's notify step mutates
**one** root card for the whole run.

## Layout

| File | Role |
| --- | --- |
| `scripts/notify-bot.sh` | Transport core: `notify_status` (root card) + `notify_reply` (threaded). Best-effort, never fails the build. |
| `scripts/render-verdict.sh` | Shared `render_verdict <incident.json>` jq helper. Verdict-led: when the incident carries a `finalVerdict`, that self-contained narrative is the reply body (leading verdict + cited source, confirming code fence, symptom-vs-cause, evidence-gap reasoning, cross-node/stage/image table, owner routing and next steps), appended only with the raw evidence snippets and the "capture next run" gap list. Incidents with no `finalVerdict` fall back to the full labeled render (root cause, top anomaly, failing unit, causal chain, symptom-vs-cause, falsification, node assessment, evidence, gaps, recommended action / proposed fix). Each section is type-guarded, so a malformed field skips only its own section. |
| `scripts/notify-incident.sh` | Per-stage bridge: `incident.json` → one card + a verdict reply. Fires on a confident analysis with a `finalVerdict` **or** a `proposedFix` (so regression / infra verdicts ping, not only code fixes). |
| `pipeline-constants.yml` | Notifier endpoint / audience / source + target Teams team & channel ids. |
| `notify-pipeline.yml` | Standalone example pipeline showing the running/succeeded/failed pattern. |

## Wiring in `pipeline.yaml`

- Per stage: `templates/failure-analysis.job.yaml` already runs `notify-incident.sh incident.json 0.65` when analysis is confident.

## Setup

1. `pipeline-constants.yml` → set `notifyTeamId`, `notifyChannelId`.
2. `azureSubscription:` on the notify steps → your WIF service connection (`acn-dalec-test`).
3. In the notifier backend (separate repo): add the WIF service-principal appid to `ALLOWED_CALLERS`, the team groupId to `ALLOWED_TEAMS`, and install the bot in the target Teams team.
4. Agent requirements: Azure CLI and `jq`.
