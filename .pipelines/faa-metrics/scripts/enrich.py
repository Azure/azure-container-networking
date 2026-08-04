#!/usr/bin/env python3
"""Enrich the raw FAA metrics CSV into an editable workbook + summary.

Adds an AI-generated "Solution / Conclusion / How it helped" narrative per run
(reusing the same Azure OpenAI deployment the FAA itself uses) and writes:

  * ``faa-metrics-organized.xlsx`` - a "Runs" sheet (relevant fields + AI columns
    + blank hand-editable columns) and a "Summary" success-metrics sheet.
  * ``faa-metrics-summary.md`` - the same aggregate metrics as Markdown.

AI enrichment is optional. If the ``AZURE_OPENAI_*`` env vars are absent, or
``--enable-ai false`` is passed, the AI columns are written blank and the step
still succeeds - matching the FAA's own degrade-gracefully behavior.

Env (inherited from the pipeline's linked variable group):
  AZURE_OPENAI_ENDPOINT, AZURE_OPENAI_DEPLOYMENT,
  AZURE_OPENAI_API_KEY, AZURE_OPENAI_API_VERSION
"""
from __future__ import annotations

import argparse
import json
import os
import sys
from collections import Counter

import requests
from openpyxl import Workbook
from openpyxl.styles import Alignment, Font, PatternFill
from openpyxl.utils import get_column_letter

import common

AI_SYSTEM_PROMPT = (
    "You evaluate the output of an automated CI failure-analysis agent (FAA) for "
    "Azure Container Networking. Given one incident's fields, write a concise, factual "
    "assessment of the value the agent provided. Respond ONLY with a JSON object with "
    "keys 'solution', 'conclusion', and 'howItHelped'. 'solution' = the fix or next "
    "action the agent proposed; 'conclusion' = the agent's root-cause verdict in one "
    "sentence; 'howItHelped' = how this analysis saved triage effort or redirected "
    "ownership. Each value <= 400 characters. Use empty strings if unknown."
)

AI_FIELDS = [
    "category", "confidence", "confidenceBand", "analysisStatus", "rootCauseSummary",
    "recommendedOwner", "recommendedAction", "proposedFix", "verdict",
    "falsificationOutcome", "nodeAssessment",
]

HEADER_FILL = PatternFill("solid", fgColor="1F4E78")
HEADER_FONT = Font(bold=True, color="FFFFFF")
WRAP_COLUMNS = {
    "rootCauseSummary", "recommendedAction", "proposedFix", "verdict",
    "nodeAssessment", "aiSolutionSummary", "aiConclusion", "aiHowItHelped",
    "reviewerNotes", "actualResolution",
}


def log(msg: str) -> None:
    print(f"[enrich] {msg}", flush=True)


class AzureOpenAI:
    def __init__(self, endpoint, deployment, api_key, api_version):
        self.url = (
            f"{endpoint.rstrip('/')}/openai/deployments/{deployment}"
            f"/chat/completions?api-version={api_version}"
        )
        self.headers = {"Content-Type": "application/json", "api-key": api_key}

    def summarize(self, row: dict) -> dict:
        facts = {k: row.get(k, "") for k in AI_FIELDS}
        body = {
            "messages": [
                {"role": "system", "content": AI_SYSTEM_PROMPT},
                {"role": "user", "content": json.dumps(facts, ensure_ascii=False)},
            ],
            "temperature": 0.2,
            "response_format": {"type": "json_object"},
        }
        resp = requests.post(self.url, headers=self.headers, json=body, timeout=90)
        resp.raise_for_status()
        content = resp.json()["choices"][0]["message"]["content"]
        parsed = json.loads(content)
        return {
            "aiSolutionSummary": str(parsed.get("solution", "")).strip(),
            "aiConclusion": str(parsed.get("conclusion", "")).strip(),
            "aiHowItHelped": str(parsed.get("howItHelped", "")).strip(),
        }


def build_ai_client(enable_ai: bool):
    if not enable_ai:
        log("AI enrichment disabled via --enable-ai false; AI columns left blank.")
        return None
    endpoint = os.environ.get("AZURE_OPENAI_ENDPOINT", "").strip()
    deployment = os.environ.get("AZURE_OPENAI_DEPLOYMENT", "").strip()
    api_key = os.environ.get("AZURE_OPENAI_API_KEY", "").strip()
    api_version = os.environ.get("AZURE_OPENAI_API_VERSION", "").strip()
    if not (endpoint and deployment and api_key and api_version):
        log("AZURE_OPENAI_* not fully configured; AI columns left blank.")
        return None
    log(f"AI enrichment enabled (deployment '{deployment}').")
    return AzureOpenAI(endpoint, deployment, api_key, api_version)


def enrich_rows(rows: list[dict], client) -> None:
    for row in rows:
        for col in common.AI_COLUMNS:
            row.setdefault(col, "")
        for col in common.MANUAL_COLUMNS:
            row[col] = ""
        if client is None:
            continue
        try:
            row.update(client.summarize(row))
        except (requests.RequestException, KeyError, ValueError) as exc:
            log(f"build {row.get('buildId', '?')}: AI enrichment failed: {exc}")


def compute_summary(rows, from_date, to_date) -> list[tuple[str, object]]:
    total = len(rows)
    status = Counter(r.get("analysisStatus", "") or "unknown" for r in rows)
    categories = Counter(r.get("category", "") or "unknown" for r in rows)
    bands = Counter(r.get("confidenceBand", "") or "unknown" for r in rows)
    with_fix = sum(1 for r in rows if (r.get("proposedFix", "") or "").strip())
    refuted = sum(
        1 for r in rows if (r.get("falsificationOutcome", "") or "").lower() == "refuted"
    )
    analyzed = status.get("analyzed", 0)

    summary: list[tuple[str, object]] = [
        ("Date range", f"{from_date} to {to_date}"),
        ("Total failures analyzed", total),
        ("Analyzed (LLM produced classification)", analyzed),
        ("Analysis failed (evidence preserved for human)", status.get("analysis_failed", 0)),
        ("Runs with a proposed fix", with_fix),
        ("Proposed-fix rate", f"{(with_fix / total * 100):.0f}%" if total else "0%"),
        ("Falsification 'refuted' (AI overturned pre-match)", refuted),
        ("", ""),
        ("By category", ""),
    ]
    summary += [(f"  {name}", count) for name, count in categories.most_common()]
    summary += [("", ""), ("By confidence band", "")]
    summary += [(f"  {name}", count) for name, count in bands.most_common()]
    return summary


def style_header(ws, ncols) -> None:
    for col in range(1, ncols + 1):
        cell = ws.cell(row=1, column=col)
        cell.fill = HEADER_FILL
        cell.font = HEADER_FONT
        cell.alignment = Alignment(vertical="center")
    ws.freeze_panes = "A2"


def write_workbook(path, rows, columns, summary) -> None:
    wb = Workbook()
    runs = wb.active
    runs.title = "Runs"
    runs.append(columns)
    for row in rows:
        runs.append([row.get(col, "") for col in columns])
    style_header(runs, len(columns))
    for idx, col in enumerate(columns, start=1):
        letter = get_column_letter(idx)
        if col in WRAP_COLUMNS:
            runs.column_dimensions[letter].width = 48
            for cell in runs[letter][1:]:
                cell.alignment = Alignment(wrap_text=True, vertical="top")
        else:
            runs.column_dimensions[letter].width = max(12, min(len(col) + 4, 28))

    summary_ws = wb.create_sheet("Summary")
    summary_ws.append(["Metric", "Value"])
    for name, value in summary:
        summary_ws.append([name, value])
    style_header(summary_ws, 2)
    summary_ws.column_dimensions["A"].width = 46
    summary_ws.column_dimensions["B"].width = 28

    os.makedirs(os.path.dirname(os.path.abspath(path)), exist_ok=True)
    wb.save(path)


def write_markdown(path, summary) -> None:
    lines = ["# FAA Success Metrics", "", "| Metric | Value |", "| --- | --- |"]
    for name, value in summary:
        if not name and value == "":
            continue
        lines.append(f"| {name.strip() or '&nbsp;'} | {value} |")
    os.makedirs(os.path.dirname(os.path.abspath(path)), exist_ok=True)
    with open(path, "w", encoding="utf-8") as fh:
        fh.write("\n".join(lines) + "\n")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--in-csv", required=True)
    parser.add_argument("--out-xlsx", required=True)
    parser.add_argument("--out-md", required=True)
    parser.add_argument("--from-date", default="")
    parser.add_argument("--to-date", default="")
    parser.add_argument("--enable-ai", default="true")
    args = parser.parse_args()

    enable_ai = str(args.enable_ai).strip().lower() in ("true", "1", "yes")
    rows = common.read_csv_rows(args.in_csv) if os.path.isfile(args.in_csv) else []
    log(f"loaded {len(rows)} row(s) from {args.in_csv}")

    client = build_ai_client(enable_ai)
    enrich_rows(rows, client)

    columns = common.RAW_COLUMNS + common.AI_COLUMNS + common.MANUAL_COLUMNS
    summary = compute_summary(rows, args.from_date, args.to_date)

    write_workbook(args.out_xlsx, rows, columns, summary)
    write_markdown(args.out_md, summary)
    log(f"wrote {args.out_xlsx} and {args.out_md}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
