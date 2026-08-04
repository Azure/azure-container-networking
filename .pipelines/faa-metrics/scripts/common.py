"""Shared helpers for the FAA success-metrics scripts.

These three scripts (collect_reports.py, build_dataset.py, enrich.py) run as
sequential steps of the `faa_metrics` stage in .pipelines/pipeline.yaml. They
harvest the Failure Analysis Agent (FAA) outputs published by pipeline
definition 95007 and turn them into success-metrics artifacts.

This module only holds constants and small helpers shared across the steps so
the schema stays defined in one place.
"""
from __future__ import annotations

import csv
import json
import os
from typing import Any

# Columns for the raw CSV, in output order. Kept here so build_dataset.py and
# enrich.py agree on the schema without duplicating the list.
RAW_COLUMNS: list[str] = [
    "buildId",
    "buildNumber",
    "buildDate",
    "pipelineName",
    "stage",
    "job",
    "clusterName",
    "os",
    "cni",
    "region",
    "commit",
    "prNumber",
    "fingerprint",
    "category",
    "confidence",
    "confidenceBand",
    "analysisStatus",
    "classificationSource",
    "rootCauseSummary",
    "recommendedOwner",
    "recommendedAction",
    "proposedFix",
    "retentionDecision",
    "nodeAssessment",
    "verdict",
    "falsificationOutcome",
    "reportUrl",
]

# Extra columns added by enrich.py in the organized workbook. The first three
# are AI-generated; the rest are intentionally blank for the user to fill in.
AI_COLUMNS: list[str] = ["aiSolutionSummary", "aiConclusion", "aiHowItHelped"]
MANUAL_COLUMNS: list[str] = ["reviewerNotes", "actualResolution", "wasHelpful (Y/N)"]

# Filenames the FAA publishes inside each failureAnalysis_* artifact.
REPORT_FILENAME = "report.md"
INCIDENT_FILENAME = "incident.json"

# Sidecar file collect_reports.py writes with per-build ADO metadata, keyed by
# build id (string).
BUILDS_META_FILENAME = "builds.json"


def read_csv_rows(path: str) -> list[dict[str, str]]:
    with open(path, "r", encoding="utf-8", newline="") as fh:
        return list(csv.DictReader(fh))


def write_csv_rows(path: str, columns: list[str], rows: list[dict[str, Any]]) -> None:
    os.makedirs(os.path.dirname(os.path.abspath(path)), exist_ok=True)
    with open(path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.DictWriter(fh, fieldnames=columns, extrasaction="ignore")
        writer.writeheader()
        for row in rows:
            writer.writerow({col: row.get(col, "") for col in columns})


def load_json(path: str) -> Any:
    with open(path, "r", encoding="utf-8") as fh:
        return json.load(fh)


def dump_json(path: str, value: Any) -> None:
    os.makedirs(os.path.dirname(os.path.abspath(path)), exist_ok=True)
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(value, fh, indent=2, sort_keys=True)
