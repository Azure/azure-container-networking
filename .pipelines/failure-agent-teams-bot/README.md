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
| `scripts/notify-incident.sh` | Per-stage bridge: `incident.json` → one card + a verdict reply. Fires on a confident analysis with a `finalVerdict` **or** a `proposedFix` (so regression / infra verdicts ping, not only code fixes). Posts to the channel with **no @mentions** — per-run cards are high-volume, so individual mentions were dropped to avoid notification fatigue. |
| `scripts/collect-week-artifacts.sh` | Walks the last N days of builds for the configured pipeline definition(s) via the ADO REST API and downloads every `failureAnalysis_*` artifact (report.md + incident.json) into one directory, so the weekly digest can aggregate them. Needs `SYSTEM_ACCESSTOKEN`, `curl`, `jq`, `unzip`. |
| `scripts/notify-weekly.sh` | Weekly bridge: `weekly-incident.json` → one trends card + a threaded narrative reply. **This card @mentions the failure-analysis owners** — it is the low-frequency digest meant for their review. |
| `pipeline-constants.yml` | Notifier endpoint / audience / source + target Teams team & channel ids. |
| `notify-pipeline.yml` | Standalone example pipeline showing the running/succeeded/failed pattern. |
| `weekly-trends-pipeline.yml` | Scheduled weekly pipeline: aggregate the week's `failureAnalysis_*` artifacts → `failure-agent --weekly-report` synthesizes a trends digest (Azure OpenAI) → `notify-weekly.sh` posts the card. |

## Mentions

- **Per-run cards** (`notify-incident.sh`) post to the channel and @mention no one.
- **Weekly trends card** (`notify-weekly.sh`) @mentions the failure-analysis
  owners (John Payne, Behzad Mirkhanzadeh) — the one card meant for their review.

## Wiring in `pipeline.yaml`

- Per stage: `templates/failure-analysis.job.yaml` already runs `notify-incident.sh incident.json 0.65` when analysis is confident.
- Weekly: `weekly-trends-pipeline.yml` runs on a schedule (independent of the E2E pipeline) to post the trends digest.

## Setup

1. `pipeline-constants.yml` → set `notifyTeamId`, `notifyChannelId`.
2. `azureSubscription:` on the notify steps → your WIF service connection (`acn-dalec-test`).
3. In the notifier backend (separate repo): add the WIF service-principal appid to `ALLOWED_CALLERS`, the team groupId to `ALLOWED_TEAMS`, and install the bot in the target Teams team.
4. Per-run agent requirements: Azure CLI and `jq`.
5. Weekly digest (`weekly-trends-pipeline.yml`): `parameters.analysisPipelineIds` defaults to `95007` (ACN E2E, msazure/One) — add more comma-separated ADO build-definition id(s) to aggregate additional pipelines' `failureAnalysis_*` artifacts. Enable "Allow scripts to access the OAuth token" (for `SYSTEM_ACCESSTOKEN`), provide the `AZURE_OPENAI_*` variables to the synthesis step, and ensure `Go`, `curl`, `jq`, and `unzip` are on the agent.
