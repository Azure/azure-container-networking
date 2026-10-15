#!/usr/bin/env python3
"""Fail stage-bundle validation if required stages did not succeed."""

import json
import os
from pathlib import Path
from urllib.request import Request, urlopen


def summarize(suites, companions, records):
    attempts = {}
    for row in records:
        if row["type"] != "Stage":
            continue
        name = row["identifier"]
        if row.get("attempt", 1) >= attempts.get(name, {}).get("attempt", 0):
            attempts[name] = row
    stages = {name: row.get("result") for name, row in attempts.items()}
    required = {
        suite["parameters"][key]
        for suite in suites for key in ("name", "clusterName")
    } | {
        "setup", "test", "test_windows", "code_coverage", "delete_clusters",
    }
    if companions:
        required |= {"containerize", "publish", "sbom"}
    errors = [
        f"{stage}: {stages.get(stage, 'missing')}"
        for stage in sorted(required) if stages.get(stage) != "succeeded"
    ]
    return {
        "suites": suites, "companions": companions,
        "stageResults": stages, "errors": errors,
    }


def main():
    suites = json.loads(os.environ["SELECTED_SUITES"])
    companions = json.loads(os.environ["COMPANION_IMAGES"])
    base = os.environ["SYSTEM_COLLECTIONURI"]
    project = os.environ["SYSTEM_TEAMPROJECTID"]
    build_id = os.environ["BUILD_BUILDID"]
    request = Request(
        f"{base}{project}/_apis/build/builds/{build_id}/timeline?api-version=7.1",
        headers={"Authorization": "Bearer " + os.environ["SYSTEM_ACCESSTOKEN"]},
    )
    with urlopen(request, timeout=60) as response:
        records = json.load(response)["records"]
    report = summarize(suites, companions, records)
    report["supplied"] = {
        key: value for key, value in json.loads(os.environ["IMAGE_INPUTS"]).items()
        if value not in {"", "__use-default__"}
    }
    report["buildId"] = build_id
    output = Path(os.environ["REPORT_DIRECTORY"])
    output.mkdir(parents=True, exist_ok=True)
    (output / "summary.json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report, indent=2))
    if report["errors"]:
        raise SystemExit("Image validation is incomplete; see summary.json")


if __name__ == "__main__":
    main()
