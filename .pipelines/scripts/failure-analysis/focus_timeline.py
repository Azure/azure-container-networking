#!/usr/bin/env python3
"""Focus the failure-analysis evidence bundle on the tasks that actually failed.

The pipeline-wide (non-E2E) failure analyzer runs after any pipeline failure, so
the interesting evidence is buried in one or two failed Azure DevOps tasks among
dozens of successful ones. This script reads the ADO build timeline for the
current run, downloads only the logs for records that failed (preferring leaf
Task records), reconstructs the Stage / Job / Task path for each, and writes a
compact focus.txt plus failed-tasks.json into the evidence bundle so the agent
analyzes the real failure instead of the analysis job itself.

It also emits ##vso[task.setvariable] for FAILED_STAGE / FAILED_JOB so the
calling job can pass them to the agent as RunContext overrides.

Environment:
  EVIDENCE            evidence bundle directory (required)
  SYSTEM_ACCESSTOKEN  ADO OAuth access token (required)
  ADO_BUILD_BASE_URL  .../_apis/build/builds/<buildId> (required)
  MAX_LOGS            max failed-task logs to download (default 40)

Best-effort: any network/parse failure degrades to a bounded bulk log download
and never raises, so the analysis step still runs.
"""
import json
import os
import re
import sys
import urllib.request

EVIDENCE = os.environ.get("EVIDENCE", "")
TOKEN = os.environ.get("SYSTEM_ACCESSTOKEN", "")
BASE = os.environ.get("ADO_BUILD_BASE_URL", "").rstrip("/")
try:
    MAX_LOGS = int(os.environ.get("MAX_LOGS", "40"))
except ValueError:
    MAX_LOGS = 40


def fetch(url, raw=False):
    req = urllib.request.Request(url, headers={"Authorization": "Bearer " + TOKEN})
    with urllib.request.urlopen(req, timeout=60) as resp:  # noqa: S310 (trusted ADO URL)
        data = resp.read().decode("utf-8", "replace")
    return data if raw else json.loads(data)


def sanitize(value):
    return re.sub(r"[^A-Za-z0-9._-]+", "_", value or "").strip("_") or "unknown"


def ancestor(record, by_id, want_type):
    seen, cur = set(), record
    while cur is not None:
        pid = cur.get("parentId")
        if not pid or pid in seen:
            break
        seen.add(pid)
        cur = by_id.get(pid)
        if cur and cur.get("type") == want_type:
            return cur.get("name", "")
    return ""


def bulk_fallback(logs_dir):
    """Download a bounded slice of all task logs when no failed-task log was found."""
    sys.stderr.write("focus: no failed-task logs; falling back to bulk log index\n")
    try:
        index = fetch(BASE + "/logs?api-version=7.1")
    except Exception as err:  # noqa: BLE001
        sys.stderr.write("focus: bulk fallback failed: %s\n" % err)
        return
    for entry in index.get("value", [])[:50]:
        url = entry.get("url", "")
        if not url:
            continue
        try:
            body = fetch(url, raw=True)
            with open(os.path.join(logs_dir, "%s.log" % entry.get("id", "unknown")), "w") as fh:
                fh.write(body)
        except Exception:  # noqa: BLE001
            pass


def main():
    if not (EVIDENCE and TOKEN and BASE):
        sys.stderr.write(
            "focus: EVIDENCE/SYSTEM_ACCESSTOKEN/ADO_BUILD_BASE_URL not all set; skipping focus\n"
        )
        return

    logs_dir = os.path.join(EVIDENCE, "logs")
    os.makedirs(logs_dir, exist_ok=True)

    try:
        timeline = fetch(BASE + "/timeline?api-version=7.1")
    except Exception as err:  # noqa: BLE001
        sys.stderr.write("focus: timeline download failed: %s\n" % err)
        bulk_fallback(logs_dir)
        return

    with open(os.path.join(EVIDENCE, "timeline.json"), "w") as fh:
        json.dump(timeline, fh)

    records = timeline.get("records", [])
    by_id = {r.get("id"): r for r in records}

    failed = [r for r in records if r.get("result") == "failed"]
    leaf_tasks = [r for r in failed if r.get("type") == "Task" and (r.get("log") or {}).get("url")]
    targets = leaf_tasks or [r for r in failed if (r.get("log") or {}).get("url")]

    summary = []
    downloaded = 0
    for rec in targets[:MAX_LOGS]:
        stage = ancestor(rec, by_id, "Stage")
        job = ancestor(rec, by_id, "Job") or ancestor(rec, by_id, "Phase")
        task = rec.get("name", "unknown")
        issues = "; ".join(
            i.get("message", "")
            for i in (rec.get("issues") or [])
            if i.get("type") == "error"
        )
        label = "%s__%s__%s" % (sanitize(stage), sanitize(job), sanitize(task))
        url = (rec.get("log") or {}).get("url", "")
        try:
            body = fetch(url, raw=True)
            with open(os.path.join(logs_dir, label + ".log"), "w") as fh:
                fh.write(body)
            downloaded += 1
        except Exception as err:  # noqa: BLE001
            sys.stderr.write("focus: log download failed for %s: %s\n" % (label, err))
        summary.append({"stage": stage, "job": job, "task": task, "issues": issues})

    with open(os.path.join(EVIDENCE, "focus.txt"), "w") as fh:
        fh.write("Failed tasks (from ADO build timeline):\n")
        for s in summary:
            fh.write(
                "- STAGE='%s' JOB='%s' TASK='%s' :: %s\n"
                % (s["stage"], s["job"], s["task"], s["issues"])
            )
    with open(os.path.join(EVIDENCE, "failed-tasks.json"), "w") as fh:
        json.dump(summary, fh, indent=2)

    primary = summary[0] if summary else {"stage": "", "job": ""}
    print("focus: downloaded %d failed-task log(s) from %d failed record(s)" % (downloaded, len(summary)))
    print("##vso[task.setvariable variable=FAILED_STAGE]%s" % primary["stage"])
    print("##vso[task.setvariable variable=FAILED_JOB]%s" % primary["job"])

    if downloaded == 0:
        bulk_fallback(logs_dir)


if __name__ == "__main__":
    try:
        main()
    except Exception as err:  # noqa: BLE001
        # Never fail the analysis job on collection problems.
        sys.stderr.write("focus: unexpected error, continuing: %s\n" % err)
        sys.exit(0)
