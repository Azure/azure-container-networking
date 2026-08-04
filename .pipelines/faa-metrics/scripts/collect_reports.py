#!/usr/bin/env python3
"""Download FAA (Failure Analysis Agent) outputs from ADO pipeline runs.

Lists builds of a pipeline definition (default 95007) within a date range,
finds each build's ``failureAnalysis_*`` artifacts, downloads them, and extracts
``report.md`` / ``incident.json`` into an output tree the next step parses.

Runs as a step of the ``faa_metrics`` stage, so it authenticates to the Azure
DevOps REST API with the pipeline's own ``System.AccessToken`` (exposed as the
``SYSTEM_ACCESSTOKEN`` env var). No PAT or extra wiring is required.

Env (all provided automatically inside the pipeline):
  SYSTEM_ACCESSTOKEN   OAuth token for the ADO REST API (Bearer).
  SYSTEM_COLLECTIONURI  e.g. https://msazure.visualstudio.com/
  SYSTEM_TEAMPROJECT    e.g. One
"""
from __future__ import annotations

import argparse
import io
import os
import sys
import zipfile

import requests

import common

API_VERSION = "7.1"
ARTIFACT_PREFIX = "failureAnalysis"
WANTED_FILES = {common.REPORT_FILENAME, common.INCIDENT_FILENAME}


def log(msg: str) -> None:
    print(f"[collect] {msg}", flush=True)


def make_session(token: str) -> requests.Session:
    session = requests.Session()
    session.headers.update(
        {"Authorization": f"Bearer {token}", "Accept": "application/json"}
    )
    return session


def iter_builds(session, base, definition_id, min_time, max_time):
    """Yield build objects for a definition within [min_time, max_time]."""
    url = f"{base}/_apis/build/builds"
    params = {
        "definitions": str(definition_id),
        "minTime": f"{min_time}T00:00:00Z",
        "maxTime": f"{max_time}T23:59:59Z",
        "queryOrder": "finishTimeDescending",
        "api-version": API_VERSION,
        "$top": "1000",
    }
    while True:
        resp = session.get(url, params=params, timeout=60)
        resp.raise_for_status()
        payload = resp.json()
        yield from payload.get("value", [])
        token = resp.headers.get("x-ms-continuationtoken")
        if not token:
            return
        params["continuationToken"] = token


def get_artifacts(session, base, build_id):
    url = f"{base}/_apis/build/builds/{build_id}/artifacts"
    resp = session.get(url, params={"api-version": API_VERSION}, timeout=60)
    if resp.status_code == 404:
        return []
    resp.raise_for_status()
    return resp.json().get("value", [])


def extract_wanted(session, download_url, dest_dir):
    """Download an artifact zip and extract wanted files into dest_dir.

    Returns the list of extracted basenames.
    """
    resp = session.get(download_url, timeout=180)
    resp.raise_for_status()
    extracted = []
    with zipfile.ZipFile(io.BytesIO(resp.content)) as archive:
        for member in archive.namelist():
            base = os.path.basename(member)
            if base in WANTED_FILES and not member.endswith("/"):
                os.makedirs(dest_dir, exist_ok=True)
                with archive.open(member) as src, open(
                    os.path.join(dest_dir, base), "wb"
                ) as out:
                    out.write(src.read())
                extracted.append(base)
    return extracted


def build_web_url(build):
    links = build.get("_links") or {}
    web = links.get("web") or {}
    return web.get("href", "")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--definition-id", required=True)
    parser.add_argument("--from-date", required=True, help="YYYY-MM-DD (inclusive)")
    parser.add_argument("--to-date", required=True, help="YYYY-MM-DD (inclusive)")
    parser.add_argument("--out-dir", required=True, help="output root directory")
    args = parser.parse_args()

    token = os.environ.get("SYSTEM_ACCESSTOKEN", "").strip()
    collection = os.environ.get("SYSTEM_COLLECTIONURI", "").strip().rstrip("/")
    project = os.environ.get("SYSTEM_TEAMPROJECT", "").strip()
    if not token or not collection or not project:
        log(
            "SYSTEM_ACCESSTOKEN / SYSTEM_COLLECTIONURI / SYSTEM_TEAMPROJECT must be set. "
            "Enable 'Allow scripts to access the OAuth token' (already on for this pipeline)."
        )
        return 2

    base = f"{collection}/{project}"
    raw_dir = os.path.join(args.out_dir, "raw")
    os.makedirs(raw_dir, exist_ok=True)

    session = make_session(token)
    builds_meta: dict[str, dict] = {}
    reports_found = 0

    log(
        f"Listing builds for definition {args.definition_id} "
        f"from {args.from_date} to {args.to_date}"
    )
    for build in iter_builds(session, base, args.definition_id, args.from_date, args.to_date):
        build_id = str(build.get("id"))
        artifacts = get_artifacts(session, base, build_id)
        fa_artifacts = [
            a for a in artifacts if str(a.get("name", "")).startswith(ARTIFACT_PREFIX)
        ]
        if not fa_artifacts:
            continue

        builds_meta[build_id] = {
            "buildId": build_id,
            "buildNumber": build.get("buildNumber", ""),
            "buildDate": build.get("finishTime") or build.get("startTime") or "",
            "pipelineName": (build.get("definition") or {}).get("name", ""),
            "commit": build.get("sourceVersion", ""),
            "sourceBranch": build.get("sourceBranch", ""),
            "result": build.get("result", ""),
            "reportUrl": build_web_url(build),
        }

        for artifact in fa_artifacts:
            name = artifact.get("name", "")
            download_url = (artifact.get("resource") or {}).get("downloadUrl")
            if not download_url:
                continue
            dest = os.path.join(raw_dir, f"{build_id}__{name}")
            try:
                got = extract_wanted(session, download_url, dest)
            except (requests.RequestException, zipfile.BadZipFile) as exc:
                log(f"build {build_id} artifact {name}: download/extract failed: {exc}")
                continue
            if got:
                reports_found += 1
                log(f"build {build_id} artifact {name}: extracted {', '.join(sorted(got))}")

    common.dump_json(os.path.join(args.out_dir, common.BUILDS_META_FILENAME), builds_meta)
    log(
        f"Done. {len(builds_meta)} build(s) had failureAnalysis artifacts; "
        f"{reports_found} artifact folder(s) extracted into {raw_dir}"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
