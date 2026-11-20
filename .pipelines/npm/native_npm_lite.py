#!/usr/bin/env python3
"""Run native Windows NPM Lite coverage without copying the private harness."""

import argparse
import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import sys
import time
import urllib.error
import urllib.request


PIPELINE_ID = 391699
API_ROOT = "https://dev.azure.com/msazure/One/_apis/"
REGION = "australiaeast"
STORAGE_ACCOUNT = "acndevshared"
STORAGE_GROUP = "acn-shared-binaries"
BASE_SHARE = "persistent-cluster-regular-runs"
PLAN = "CNI Singularity SwiftV2 Accelnet"
TESTS = {
    "swiftv2_npmlite_pod_to_pod_singularity": {
        "handler": "testcase_swiftv2_npmLite_pod_to_pod_singularity",
        "topic": "npmLitePodToPodSingularity",
        "completion": "delete namespace in the cluster",
    },
    "swiftv2_npmlite_default_deny_nonswift_singularity": {
        "handler": "testcase_swiftv2_npmLite_default_deny_nonswift_singularity",
        "topic": "npmLiteDefaultDenyNonSwiftSingularity",
        "completion": "default-deny non-SwiftV2 validation complete",
    },
}


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class DevOps:
    def __init__(self):
        self.token = os.environ["SYSTEM_ACCESSTOKEN"]
        if not self.token or self.token.startswith("$("):
            raise RuntimeError("System.AccessToken is required")
        self.opener = urllib.request.build_opener(NoRedirect())

    def request(self, route, payload=None, method=None, text=False):
        headers = {"Authorization": f"Bearer {self.token}"}
        headers["Accept"] = "text/plain" if text else "application/json"
        data = None
        if payload is not None:
            data = json.dumps(payload).encode()
            headers["Content-Type"] = "application/json"
        separator = "&" if "?" in route else "?"
        request = urllib.request.Request(
            f"{API_ROOT}{route}{separator}api-version=7.1",
            data=data,
            headers=headers,
            method=method,
        )
        # A lost queue/cancel response must not cause another mutation.
        attempts = 3 if request.get_method() == "GET" else 1
        for attempt in range(attempts):
            try:
                with self.opener.open(request, timeout=60) as response:
                    content = response.read(20 * 1024 * 1024 + 1)
                if len(content) > 20 * 1024 * 1024:
                    raise RuntimeError(f"native response exceeds the size limit: {route}")
                return content.decode() if text else json.loads(content)
            except urllib.error.HTTPError as error:
                if error.code in (429, 502, 503, 504) and attempt + 1 < attempts:
                    time.sleep(10 * (attempt + 1))
                    continue
                raise RuntimeError(
                    f"{request.get_method()} {route} returned HTTP {error.code}; "
                    "no permissions were changed"
                ) from error
        raise RuntimeError(f"native request did not complete: {route}")


def az(*arguments):
    result = subprocess.run(
        ["az", *arguments, "--only-show-errors", "-o", "json"],
        text=True,
        capture_output=True,
        check=False,
        timeout=300,
    )
    if result.returncode:
        raise RuntimeError(f"az {' '.join(arguments)} failed: {result.stderr.strip()}")
    return json.loads(result.stdout) if result.stdout.strip() else None


def owned_resources(build_id, attempt, subscription):
    if not re.fullmatch(r"[1-9][0-9]*", build_id) or not re.fullmatch(
        r"[1-9][0-9]*", attempt
    ):
        raise ValueError("positive Build.BuildId and System.StageAttempt are required")
    prefix = f"npm{build_id}a{attempt}"
    return {
        "subscription": subscription,
        "prefix": prefix,
        "group": f"sing-{prefix}-winacc-persistent-{REGION}",
        "share": f"{prefix}-{BASE_SHARE}",
    }


def share_arguments(resources):
    return (
        "--subscription", resources["subscription"],
        "--resource-group", STORAGE_GROUP,
        "--storage-account", STORAGE_ACCOUNT,
        "--name", resources["share"],
    )


def group_exists(resources):
    result = az(
        "group", "exists", "--subscription", resources["subscription"],
        "--name", resources["group"],
    )
    if not isinstance(result, bool):
        raise RuntimeError("resource-group existence could not be verified")
    return result


def share_exists(resources):
    result = az("storage", "share-rm", "exists", *share_arguments(resources))
    if not isinstance(result, dict) or not isinstance(result.get("exists"), bool):
        raise RuntimeError("file-share existence could not be verified")
    return result["exists"]


def save_state(path, state):
    temporary = path.with_suffix(".tmp")
    temporary.write_text(json.dumps(state, indent=2) + "\n", encoding="utf-8")
    temporary.replace(path)
    summary = {
        key: state[key]
        for key in ("source_version", "runs", "tests", "cleanup_verified")
        if key in state
    }
    path.with_suffix(".summary.json").write_text(
        json.dumps(summary, indent=2) + "\n", encoding="utf-8"
    )
    print("##vso[task.setvariable variable=NATIVE_NPM_SUMMARY_READY]true", flush=True)


def parameters(resources, mode):
    return {
        "runMode": mode,
        "devPrefix": resources["prefix"],
        "vmssSubscriptionId": resources["subscription"],
        "clusterSubscriptionId": resources["subscription"],
        "westus2": False,
        "centraluseuap": False,
        "australiaeast": True,
        "accelnetl1vh": True,
        "windowsib": False,
        "linuxacc": False,
        "linuxib": False,
        "updateComponents": False,
        "isPreserveClustersAndResources": False,
        "useAgentTestSubscription": False,
        "useAcnPublic": False,
        "npmversion": "none",
    }


def queue(api, path, state, mode):
    state["pending_mode"] = mode
    save_state(path, state)
    source = {"refName": "refs/heads/master"}
    if "source_version" in state:
        source["version"] = state["source_version"]
    response = api.request(
        f"pipelines/{PIPELINE_ID}/runs",
        {
            "resources": {"repositories": {"self": source}},
            "templateParameters": parameters(state["resources"], mode),
        },
    )
    run = {
        "mode": mode,
        "id": int(response["id"]),
        "url": response["_links"]["web"]["href"],
    }
    state["runs"].append(run)
    if "source_version" not in state:
        resolved = response["resources"]["repositories"]["self"]
        if (
            resolved["refName"] != "refs/heads/master"
            or not re.fullmatch(r"[0-9a-f]{40}", resolved["version"])
        ):
            save_state(path, state)
            raise RuntimeError("native setup did not resolve an immutable master revision")
        state["source_version"] = resolved["version"]
    del state["pending_mode"]
    save_state(path, state)
    print(f"Native {mode} run: {run['url']}", flush=True)
    return run


def read_build(api, state, run):
    build = api.request(f"build/builds/{run['id']}")
    if (
        build["definition"]["id"] != PIPELINE_ID
        or build["sourceVersion"] != state["source_version"]
    ):
        raise RuntimeError("native build identity or pinned source does not match")
    return build


def wait(api, path, state, run, minutes):
    deadline = time.monotonic() + minutes * 60
    previous = None
    while time.monotonic() < deadline:
        build = read_build(api, state, run)
        status = build["status"]
        if status != previous:
            print(f"Native {run['mode']} {run['id']}: {status}", flush=True)
            previous = status
        if status == "completed":
            run["result"] = build["result"]
            save_state(path, state)
            return build["result"]
        time.sleep(30)
    raise TimeoutError(f"native {run['mode']} run {run['id']} exceeded {minutes} minutes")


def test_results(log):
    records = {name: [] for name in TESTS}
    completed = set()
    for line in log.splitlines():
        if not any(name in line for name in TESTS):
            continue
        fields = dict(
            token.split("=", 1) for token in shlex.split(line) if "=" in token
        )
        name = fields.get("e2eStep")
        if name not in TESTS or fields.get("e2ePlan") != PLAN:
            continue
        expected = TESTS[name]
        if fields.get("e2eHandler") != expected["handler"]:
            continue
        if (
            fields.get("topic") == expected["topic"]
            and fields.get("step") == expected["completion"]
        ):
            completed.add(name)
        if fields.get("topic") == "runHandlerStep/result":
            records[name].append(fields)
    results = {}
    for name, entries in records.items():
        status = "missing"
        duration = 0
        if len(entries) > 1:
            status = "ambiguous"
        elif entries:
            duration_text = entries[0].get("executionDurationInMilliseconds", "")
            duration = int(duration_text) if duration_text.isdecimal() else 0
            # MarkSkipped can still emit executionPassed=true in the native runner.
            # Require the handler's completion marker as well as its result.
            status = (
                "passed"
                if entries[0].get("executionPassed") == "true"
                and duration > 0
                and name in completed
                else "failed_or_skipped"
            )
        results[name] = {"status": status, "duration_ms": duration}
    return results


def verify_tests(api, path, state, run):
    timeline = api.request(f"build/builds/{run['id']}/timeline")
    tasks = [
        record for record in timeline["records"]
        if record["type"] == "Task"
        and record["name"] == "Run Singularity Tests (win-acc)"
    ]
    if len(tasks) != 1 or not tasks[0].get("log"):
        raise RuntimeError("expected exactly one native Windows test task with a log")
    task = tasks[0]
    log = api.request(
        f"build/builds/{run['id']}/logs/{int(task['log']['id'])}", text=True
    )
    state["tests"] = test_results(log)
    save_state(path, state)
    for name, result in state["tests"].items():
        print(f"{name}: {result['status']}", flush=True)
    if (
        task.get("result") != "succeeded"
        or run["result"] != "succeeded"
        or any(result["status"] != "passed" for result in state["tests"].values())
    ):
        raise RuntimeError(
            "native Windows NPM Lite validation failed; both requested handlers "
            "must finish successfully without skips"
        )


def run_tests(api, path, resources):
    if path.exists():
        raise RuntimeError("native run state already exists; refusing to queue duplicate tests")
    if group_exists(resources) or share_exists(resources):
        raise RuntimeError("native test resources already exist; refusing to reuse or delete them")
    state = {"resources": resources, "runs": []}
    save_state(path, state)
    setup = queue(api, path, state, "setup")
    if wait(api, path, state, setup, 360) != "succeeded":
        raise RuntimeError(f"native environment setup failed: {setup['url']}")
    tests = queue(api, path, state, "regular")
    wait(api, path, state, tests, 180)
    verify_tests(api, path, state, tests)


def cleanup(api, path, resources):
    if not path.exists():
        print("Native setup did not start; no owned resources to clean up.")
        return
    state = json.loads(path.read_text(encoding="utf-8"))
    if state["resources"] != resources:
        raise RuntimeError("native cleanup ownership does not match this build attempt")
    if state.get("pending_mode"):
        raise RuntimeError(
            f"native {state['pending_mode']} queue outcome is unknown for "
            f"{resources['prefix']}; reconcile the native run before cleanup"
        )
    if not state["runs"]:
        print("No native run was queued; no owned resources to clean up.")
        return
    for run in state["runs"]:
        build = read_build(api, state, run)
        if build["status"] != "completed":
            api.request(
                f"build/builds/{run['id']}", {"status": "cancelling"}, method="PATCH"
            )
            wait(api, path, state, run, 20)
    teardown = queue(api, path, state, "cleanup")
    result = wait(api, path, state, teardown, 120)
    if group_exists(resources):
        raise RuntimeError(f"native cleanup left resource group {resources['group']}")
    if share_exists(resources):
        az("storage", "share-rm", "delete", *share_arguments(resources), "--yes")
    if share_exists(resources):
        raise RuntimeError(f"native cleanup left file share {resources['share']}")
    state["cleanup_verified"] = True
    save_state(path, state)
    print("Verified native resource group and per-run file share are absent.", flush=True)
    if result != "succeeded":
        raise RuntimeError(f"native cleanup pipeline failed despite resource removal: {teardown['url']}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("run", "cleanup"))
    parser.add_argument("--state", required=True, type=Path)
    arguments = parser.parse_args()
    account = az("account", "show")
    resources = owned_resources(
        os.environ["BUILD_BUILDID"], os.environ["SYSTEM_STAGEATTEMPT"], account["id"]
    )
    api = DevOps()
    if arguments.action == "run":
        run_tests(api, arguments.state, resources)
    else:
        cleanup(api, arguments.state, resources)


if __name__ == "__main__":
    try:
        main()
    except (
        RuntimeError, ValueError, KeyError, OSError,
        subprocess.TimeoutExpired, urllib.error.URLError,
    ) as error:
        print(f"Native NPM Lite error: {error}", file=sys.stderr, flush=True)
        sys.exit(1)
