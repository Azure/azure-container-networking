# failure-agent-teams-bot

Bridges the Failure Analysis Agent (FAA) to Microsoft Teams through the shared
**ACN Pipeline Notifier** bot. Every `(NOTIFY_SOURCE, NOTIFY_RUN_ID)` pair renders
one Adaptive Card that updates in place across stages, plus threaded replies.

`NOTIFY_RUN_ID` is `$(Build.BuildId)`, so every stage's notify step and the final
run-summary stage all mutate **one** root card for the whole run.

## Layout

| File | Role |
| --- | --- |
| `scripts/notify-bot.sh` | Transport core: `notify_status` (root card) + `notify_reply` (threaded). Best-effort, never fails the build. |
| `scripts/render-verdict.sh` | Shared `render_verdict <incident.json>` jq helper — renders the full FAA verdict (final verdict, top anomaly, failing unit, causal chain, symptom-vs-cause, falsification, node assessment, evidence, gaps, recommended action / proposed fix). |
| `scripts/notify-incident.sh` | Per-stage bridge: `incident.json` → one card + a verdict reply. Fires on a confident analysis with a `finalVerdict` **or** a `proposedFix` (so regression / infra verdicts ping, not only code fixes). |
| `scripts/notify-run-summary.sh` | Fans in every stage's `incident.json`, groups by a stage-independent signature (failing unit / root-cause summary — the fingerprint hash embeds the stage by design), posts one consolidated card, and calls out any signature that failed across **multiple E2E stages** this run. |
| `failure-run-summary.stage.yaml` | Reusable stage that downloads `failureAnalysis_*` artifacts and runs `notify-run-summary.sh` last (`condition: always()`, `dependsOn` the E2E stages). |
| `pipeline-constants.yml` | Notifier endpoint / audience / source + target Teams team & channel ids. |
| `notify-pipeline.yml` | Standalone example pipeline showing the running/succeeded/failed pattern. |

## Wiring in `pipeline.yaml`

- Per stage: `templates/failure-analysis.job.yaml` already runs `notify-incident.sh incident.json 0.65` when analysis is confident.
- Once per run: the `failure_run_summary` stage (from `failure-run-summary.stage.yaml`) runs after the E2E matrix and writes the authoritative consolidated card.

## Setup

1. `pipeline-constants.yml` → set `notifyTeamId`, `notifyChannelId`.
2. `azureSubscription:` on the notify steps → your WIF service connection (`acn-dalec-test`).
3. In the notifier backend (separate repo): add the WIF service-principal appid to `ALLOWED_CALLERS`, the team groupId to `ALLOWED_TEAMS`, and install the bot in the target Teams team.
4. Agent requirements: Azure CLI and `jq`.
