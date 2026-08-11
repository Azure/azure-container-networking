# FAA Success Metrics

Harvests the **Failure Analysis Agent (FAA)** outputs published by ADO pipeline
definition **95007** (`Azure Container Networking PR`) across a date range and
turns them into success-metrics artifacts.

The FAA runs per CI failure and publishes a `failureAnalysis_*` build artifact
containing `report.md` and a structured `incident.json`
(see `.pipelines/templates/failure-analysis.job.yaml` and
`tools/failure-agent`). This job collects those across many runs so you can
measure how often and how well the agent helped.

## What it produces

Published as the **`faa-metrics`** build artifact:

| File | Contents |
| --- | --- |
| `faa-metrics-raw.csv` | One row per analyzed failure, machine-parsed. |
| `faa-metrics-organized.xlsx` | **Real Issues** curated proof sheet, an **All Runs** backing sheet, and a **Summary** metrics sheet. |
| `faa-metrics-real-issues.md` | Paste-ready highlights doc for a manager (drops into Word / Confluence / a wiki). Also surfaced on the run's Extensions tab. |
| `faa-metrics-summary.md` | The aggregate metrics as Markdown. |
| `raw/` | The downloaded `report.md` / `incident.json` per build, for auditing. |

### The "Real Issues" view

The curated sheet + highlights doc are the evidence that FAA catches **real**
issues. A row is included only if it is a **substantive find**: FAA produced a
classification, landed a root cause, and carries at least one impact signal —
a concrete proposed fix, a high-confidence verdict, or an overturned pre-match
(`refuted` falsification). All categories qualify, including a **flake** the
agent correctly identified.

For each curated row an LLM combs the **full `report.md`** and writes three
narrative columns — `whatHappened`, `faaFinding`, `faaValue` (what failed →
what FAA found → why it mattered) — alongside a deterministic `valueTag` and a
blank `managerNotes` column for you to annotate.

Tune the curation in `enrich.py` (`is_substantive`) and the category labels in
`common.py` (`CATEGORY_LABELS`).

## How to run it

The metrics run is an **independent, opt-in stage** (`faa_metrics`) inside the
main pipeline `.pipelines/pipeline.yaml`. It `dependsOn: []` and is gated by the
`runFaaMetrics` parameter, so it never runs on normal PR or nightly builds.

To run it in Azure DevOps:

1. **Run pipeline** on definition 95007.
2. Set the parameter **`runFaaMetrics` = true** (and adjust `faaFromDate` /
   `faaToDate` / `faaDefinitionId` / `faaEnableAI` if needed).
3. Under **Stages to run**, select **only** `FAA Success Metrics`.
4. Run. Download the `faa-metrics` artifact when it completes.

Because the stage lives in the main pipeline, it inherits that pipeline's
linked variable group (the `AZURE_OPENAI_*` variables the FAA already uses) and
its `System.AccessToken` — **no extra ADO wiring is required.**

## Parameters (defined in `.pipelines/pipeline.yaml`)

| Parameter | Default | Purpose |
| --- | --- | --- |
| `runFaaMetrics` | `false` | Turns the stage on. |
| `faaDefinitionId` | `95007` | Pipeline definition whose runs are harvested. |
| `faaFromDate` | `2026-06-15` | Start date, inclusive (`YYYY-MM-DD`). |
| `faaToDate` | `2026-08-04` | End date, inclusive (`YYYY-MM-DD`). |
| `faaEnableAI` | `true` | When `false`, the AI columns are left blank. |

## AI enrichment

`enrich.py` calls the same Azure OpenAI deployment the FAA uses, via these env
vars (sourced from the pipeline's variable group):
`AZURE_OPENAI_ENDPOINT`, `AZURE_OPENAI_DEPLOYMENT`, `AZURE_OPENAI_API_KEY`,
`AZURE_OPENAI_API_VERSION`. It runs only on the curated "Real Issues" rows,
combing each full `report.md`.

If any are missing, or `faaEnableAI=false`, enrichment is skipped and the
narrative columns are written blank — the stage still succeeds. You can fill or
re-run them later.

## Layout

```
.pipelines/faa-metrics/
  faa-metrics.job.yaml     # job template invoked by the faa_metrics stage
  README.md                # this file
  scripts/
    common.py              # shared CSV schema + helpers
    collect_reports.py     # ADO REST: list builds in range, download failureAnalysis_* artifacts
    build_dataset.py       # parse incident.json (+report.md) -> faa-metrics-raw.csv
    enrich.py              # curate "real issues" + AOAI narrative -> xlsx + docs
    requirements.txt       # requests, openpyxl
```

## Running the scripts locally

```bash
pip install -r scripts/requirements.txt

# 1. Download (needs ADO OAuth token + collection/project env):
SYSTEM_ACCESSTOKEN=<token> \
SYSTEM_COLLECTIONURI=https://msazure.visualstudio.com/ \
SYSTEM_TEAMPROJECT=One \
python scripts/collect_reports.py --definition-id 95007 \
  --from-date 2026-06-15 --to-date 2026-08-04 --out-dir ./out

# 2. Build the raw CSV:
python scripts/build_dataset.py --in-dir ./out --out-csv ./out/faa-metrics-raw.csv

# 3. Enrich (set AZURE_OPENAI_* to enable AI, or pass --enable-ai false):
python scripts/enrich.py --in-csv ./out/faa-metrics-raw.csv \
  --raw-dir ./out/raw \
  --out-xlsx ./out/faa-metrics-organized.xlsx \
  --out-md ./out/faa-metrics-summary.md \
  --out-issues-md ./out/faa-metrics-real-issues.md \
  --from-date 2026-06-15 --to-date 2026-08-04 --enable-ai false
```
