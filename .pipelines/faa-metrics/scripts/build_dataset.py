#!/usr/bin/env python3
"""Turn downloaded FAA outputs into a raw success-metrics CSV.

Reads the ``raw/`` tree produced by ``collect_reports.py`` (one folder per
build+artifact, each holding ``incident.json`` and/or ``report.md``) plus the
``builds.json`` metadata sidecar, and emits one CSV row per analyzed failure.

``incident.json`` is the primary, structured source. ``report.md`` is used only
for the two narrative signals it carries that the incident does not: the final
**Verdict** line and the **Falsification -> Outcome** (``refuted``/``confirmed``),
which indicate when the AI overturned the deterministic pre-match. If a folder
has only ``report.md`` (older runs), a best-effort header parse fills the gaps.
"""
from __future__ import annotations

import argparse
import os
import re
import sys

import common

# ``## Verdict: <text>`` under the "Final verdict" section.
VERDICT_RE = re.compile(r"^##\s*Verdict:\s*(.+?)\s*$", re.MULTILINE)
# ``**Outcome:** `refuted` `` under the "Falsification" section.
OUTCOME_RE = re.compile(r"\*\*Outcome:\*\*\s*`?([\w-]+)`?", re.IGNORECASE)
# Header: ``**Category:** `x`  |  **Confidence:** band (0.72)  |  **Fingerprint:** `hash` ``
CATEGORY_RE = re.compile(r"\*\*Category:\*\*\s*`?([\w-]+)`?")
CONFIDENCE_RE = re.compile(r"\*\*Confidence:\*\*\s*([\w-]+)\s*\(([0-9.]+)\)")
FINGERPRINT_RE = re.compile(r"\*\*Fingerprint:\*\*\s*`?([0-9a-fA-F]+)`?")


def parse_report_md(text: str) -> dict:
    """Extract the narrative + fallback fields from a report.md body."""
    out: dict[str, str] = {}

    m = VERDICT_RE.search(text)
    if m:
        out["verdict"] = m.group(1).strip()

    m = OUTCOME_RE.search(text)
    if m:
        out["falsificationOutcome"] = m.group(1).strip().lower()

    m = CATEGORY_RE.search(text)
    if m:
        out["category"] = m.group(1).strip()
    m = CONFIDENCE_RE.search(text)
    if m:
        out["confidenceBand"] = m.group(1).strip()
        out["confidence"] = m.group(2).strip()
    m = FINGERPRINT_RE.search(text)
    if m:
        out["fingerprint"] = m.group(1).strip()

    # "Where" table: | Field | Value |
    for field, value in re.findall(r"^\|\s*([^|]+?)\s*\|\s*([^|]+?)\s*\|\s*$", text, re.MULTILINE):
        key = field.strip().lower()
        val = value.strip()
        if val in ("Value", "---", "—", ""):
            continue
        if key == "pipeline":
            out.setdefault("pipelineName", val)
        elif key in ("stage / job", "stage/job"):
            out.setdefault("stage", val)
        elif key == "cluster":
            out.setdefault("clusterName", val)
        elif key == "scenario":
            out.setdefault("scenario", val)
        elif key == "region" and val != "—":
            out.setdefault("region", val)
        elif key == "commit":
            out.setdefault("commit", val)
    return out


def row_from_incident(incident: dict) -> dict:
    """Map an incident.json object onto the raw CSV columns."""
    return {
        "buildId": str(incident.get("buildId", "")),
        "buildNumber": incident.get("buildNumber", ""),
        "pipelineName": incident.get("pipelineName", ""),
        "stage": incident.get("stage", ""),
        "job": incident.get("job", ""),
        "clusterName": incident.get("clusterName", ""),
        "os": incident.get("os", ""),
        "cni": incident.get("cni", ""),
        "region": incident.get("region", ""),
        "commit": incident.get("commit", ""),
        "prNumber": incident.get("pullRequestNumber", ""),
        "fingerprint": incident.get("fingerprint", ""),
        "category": incident.get("category", ""),
        "confidence": incident.get("confidence", ""),
        "confidenceBand": incident.get("confidenceBand", ""),
        "analysisStatus": incident.get("analysisStatus", ""),
        "classificationSource": incident.get("classificationSource", ""),
        "rootCauseSummary": incident.get("rootCauseSummary", ""),
        "recommendedOwner": incident.get("recommendedOwner", ""),
        "recommendedAction": incident.get("recommendedAction", ""),
        "proposedFix": incident.get("proposedFix", ""),
        "retentionDecision": incident.get("retentionDecision", ""),
        "nodeAssessment": incident.get("nodeAssessment", ""),
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--in-dir", required=True, help="output root from collect step")
    parser.add_argument("--out-csv", required=True)
    args = parser.parse_args()

    raw_dir = os.path.join(args.in_dir, "raw")
    if not os.path.isdir(raw_dir):
        print(f"[dataset] no raw dir at {raw_dir}; writing empty CSV", flush=True)
        common.write_csv_rows(args.out_csv, common.RAW_COLUMNS, [])
        return 0

    builds_meta = {}
    meta_path = os.path.join(args.in_dir, common.BUILDS_META_FILENAME)
    if os.path.isfile(meta_path):
        builds_meta = common.load_json(meta_path)

    rows = []
    for folder in sorted(os.listdir(raw_dir)):
        folder_path = os.path.join(raw_dir, folder)
        if not os.path.isdir(folder_path):
            continue
        build_id = folder.split("__", 1)[0]
        incident_path = os.path.join(folder_path, common.INCIDENT_FILENAME)
        report_path = os.path.join(folder_path, common.REPORT_FILENAME)

        if os.path.isfile(incident_path):
            row = row_from_incident(common.load_json(incident_path))
        else:
            row = {}

        if os.path.isfile(report_path):
            with open(report_path, "r", encoding="utf-8", errors="replace") as fh:
                report_text = fh.read()
            report_fields = parse_report_md(report_text)
            # incident.json wins; report.md fills blanks and adds narrative.
            for key, val in report_fields.items():
                if not row.get(key):
                    row[key] = val

        if not row:
            print(f"[dataset] {folder}: no parseable incident/report, skipping", flush=True)
            continue

        meta = builds_meta.get(build_id, {})
        row.setdefault("buildId", build_id)
        for key in ("buildNumber", "buildDate", "pipelineName", "reportUrl", "commit"):
            if not row.get(key) and meta.get(key):
                row[key] = meta[key]

        rows.append(row)

    rows.sort(key=lambda r: r.get("buildDate", ""), reverse=True)
    common.write_csv_rows(args.out_csv, common.RAW_COLUMNS, rows)
    print(f"[dataset] wrote {len(rows)} row(s) to {args.out_csv}", flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
