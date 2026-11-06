#!/usr/bin/env python3
"""Queue Dalec template preparation and require an identity-bound result."""

import argparse
import io
import json
import os
import re
import subprocess
import sys
import time
import urllib.parse
import urllib.request
import zipfile

BASE = "https://dev.azure.com/msazure/One"
API = BASE + "/_apis"
STATUSES = {"PR_CREATED", "PR_REUSED", "NO_CHANGE_VERIFIED", "NOT_APPLICABLE"}


class ArtifactRedirects(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, response, code, message, headers, url):
        destination = urllib.parse.urlparse(url)
        if destination.scheme != "https" or not destination.hostname:
            raise ValueError("artifact redirect must use HTTPS")
        same_service = destination.hostname == "msazure.artifacts.visualstudio.com" or (
            destination.hostname == "dev.azure.com" and destination.path.startswith("/msazure/")
        )
        if not same_service and not destination.hostname.endswith((".vsblob.vsassets.io", ".vsblob.visualstudio.com", ".blob.core.windows.net")):
            raise ValueError("unexpected artifact redirect host")
        redirected = super().redirect_request(request, response, code, message, headers, url)
        if redirected is not None and not same_service:
            redirected.remove_header("Authorization")
        return redirected


def get(url, token, body=None, accept="application/json"):
    parsed = urllib.parse.urlparse(url)
    if parsed.scheme != "https" or parsed.hostname != "dev.azure.com" or not parsed.path.startswith("/msazure/One/"):
        raise ValueError("unexpected pipeline API URL")
    request = urllib.request.Request(
        url, data=None if body is None else json.dumps(body).encode(),
        headers={"Authorization": "Bearer " + token, "Content-Type": "application/json", "Accept": accept},
    )
    with urllib.request.build_opener(ArtifactRedirects()).open(request, timeout=60) as response:
        return response.read(4 * 1024 * 1024 + 1)


def api(url, token, body=None):
    return json.loads(get(url, token, body))


def validate_result(result, tag, sha, run_id):
    if result.get("schema") != 1 or result.get("repository") != "Azure/azure-container-networking":
        raise ValueError("unexpected preparation result schema or repository")
    if (result.get("tag"), result.get("source_sha"), str(result.get("run_id"))) != (tag, sha, str(run_id)):
        raise ValueError("preparation result does not match this release request")
    status = result.get("status")
    if status not in STATUSES:
        raise ValueError("template preparation did not succeed")
    prs = result.get("pull_requests")
    if not isinstance(prs, list):
        raise ValueError("preparation result is missing PR information")
    if status in {"PR_CREATED", "PR_REUSED"} and not prs:
        raise ValueError("preparation did not open the required PR")
    if any(not isinstance(pr, str) or not re.fullmatch(r"https://dev\.azure\.com/msazure/CloudNativeCompute/_git/aks-managed-dalec/pullrequest/[1-9]\d*", pr) for pr in prs):
        raise ValueError("unexpected preparation PR URL")
    prefix = tag.split("/")[0] if "/" in tag else ""
    if status == "NOT_APPLICABLE" and prefix not in {"dropgz", "cni-dropgz", "azure-block-iptables", "zapai"}:
        raise ValueError("an onboarded release cannot skip template preparation")


def result_from_zip(data):
    if len(data) > 4 * 1024 * 1024:
        raise ValueError("preparation artifact exceeds size limit")
    with zipfile.ZipFile(io.BytesIO(data)) as archive:
        files = [item for item in archive.infolist() if item.filename in ("result.json", "preparation-result/result.json")]
        if len(files) != 1 or files[0].file_size > 65536:
            raise ValueError("preparation artifact must contain one bounded result.json")
        return json.loads(archive.read(files[0]))


def prepare(pipeline_id, tag, sha, token, timeout_seconds=1800):
    if not re.fullmatch(r"[1-9]\d*", pipeline_id):
        raise ValueError("pipeline ID must be a positive integer")
    if not re.fullmatch(r"[0-9a-f]{40}", sha) or not re.fullmatch(r"(?:[a-z][a-z0-9-]*/)?v\d+\.\d+\.\d+", tag):
        raise ValueError("invalid release identity")
    run = api(API + f"/pipelines/{pipeline_id}/runs?api-version=7.1", token, {
        "resources": {"repositories": {"self": {"refName": "refs/heads/main"}}},
        "templateParameters": {"tag": tag, "targetSha": sha},
    })
    run_id = run["id"]
    if not isinstance(run_id, int) or run_id < 1:
        raise ValueError("invalid pipeline run ID")
    print(f"Template preparation: {BASE}/_build/results?buildId={run_id}", flush=True)
    deadline = time.monotonic() + timeout_seconds
    while run.get("state") != "completed":
        if time.monotonic() >= deadline:
            raise TimeoutError("template preparation timed out; inspect the pipeline before retrying")
        time.sleep(min(15, max(0, deadline - time.monotonic())))
        run = api(API + f"/pipelines/{pipeline_id}/runs/{run_id}?api-version=7.1", token)
    if run.get("result") != "succeeded":
        raise ValueError(f"template preparation pipeline failed: {run.get('result')}")
    artifact_url = API + f"/build/builds/{run_id}/artifacts?artifactName=preparation-result&api-version=7.1"
    result = result_from_zip(get(artifact_url + "&%24format=zip", token, accept="application/zip"))
    validate_result(result, tag, sha, run_id)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--pipeline-id", required=True)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--sha", required=True)
    args = parser.parse_args()
    token = subprocess.check_output(
        ["az", "account", "get-access-token", "--resource", "499b84ac-1321-427f-aa17-267ca6975798",
         "--query", "accessToken", "-o", "tsv"], text=True, timeout=60,
    ).strip()
    result = prepare(args.pipeline_id, args.tag, args.sha, token)
    summary = "Dalec template preparation: " + result["status"] + "\n"
    summary += "".join(f"\n- {pr}" for pr in result["pull_requests"])
    print(summary)
    with open(os.environ["GITHUB_STEP_SUMMARY"], "a") as output:
        output.write(summary + "\n")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, OSError, subprocess.SubprocessError, TimeoutError, zipfile.BadZipFile) as error:
        print(f"Dalec template preparation failed: {error}", file=sys.stderr)
        sys.exit(1)
