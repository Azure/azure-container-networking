#!/usr/bin/env python3
"""Select and verify the upstream NetworkPolicy specs run against Windows NPM Lite.

usage: windows-npm-lite-upstream.py focus
       windows-npm-lite-upstream.py validate GINKGO_JSON_REPORT
"""

import json
import re
import sys

CONTAINER = "[sig-network] Netpol NetworkPolicy between server and client"
SUFFIX = "[Feature:NetworkPolicy]"

# TCP-context specs whose policies use only target pod selectors, numeric or
# protocol-only ports, empty rules, and ipBlock peers. Lite rejects peer pod and
# namespace selectors and named ports, and Windows NPM rejects SCTP.
SPECS = (
    "should support a 'default-deny-ingress' policy",
    "should support a 'default-deny-all' policy",
    "should support allow-all policy",
    "should enforce updated policy",
    "should deny ingress access to updated pod",
    "should deny egress from pods based on PodSelector",
    "should deny egress from all pods in a namespace",
    "should enforce ingress policy allowing any port traffic to a server on a specific protocol",
    "should enforce multiple ingress policies with ingress allow-all policy taking precedence",
    "should enforce multiple egress policies with egress allow-all policy taking precedence",
    "should stop enforcing policies after they are deleted",
    "should allow egress access to server in CIDR block",
    "should enforce except clause while egress access to server in CIDR block",
    "should ensure an IP overlapping both IPBlock.CIDR and IPBlock.Except is allowed",
    "should enforce policies to check ingress and egress policies can be controlled independently based on PodSelector",
    "should not allow access by TCP when a policy specifies only UDP",
)


def escape(text):
    # Go's RE2 rejects escaped spaces, so re.escape is not suitable.
    return re.sub(r"([\\.+*?()|\[\]{}^$])", r"\\\1", text)


def focus():
    names = "|".join(escape(name) for name in SPECS)
    return f"{escape(CONTAINER)} (?:{names}) {escape(SUFFIX)}$"


def full_text(spec):
    texts = list(spec.get("ContainerHierarchyTexts") or [])
    texts.append(spec.get("LeafNodeText") or "")
    return " ".join(text for text in texts if text)


def seconds(nanoseconds):
    return (nanoseconds or 0) / 1e9


def validate(path):
    try:
        with open(path, encoding="utf-8") as source:
            reports = json.load(source)
    except (OSError, ValueError) as error:
        return [f"cannot read the Ginkgo JSON report: {error}"]
    if not isinstance(reports, list) or len(reports) != 1 or not isinstance(reports[0], dict):
        return ["expected exactly one Ginkgo suite report"]
    report = reports[0]
    problems = []
    if (report.get("SuiteConfig") or {}).get("DryRun") is not False:
        problems.append("the report is not from a real run")
    if report.get("SuiteSucceeded") is not True:
        problems.append("the suite did not succeed")
    selected = (report.get("PreRunStats") or {}).get("SpecsThatWillRun")
    if selected != len(SPECS):
        problems.append(f"{selected} specs were selected, expected {len(SPECS)}")

    expected = {f"{CONTAINER} {name} {SUFFIX}": name for name in SPECS}
    found = {text: [] for text in expected}
    for spec in report.get("SpecReports") or []:
        if spec.get("LeafNodeType") != "It":
            continue
        text = full_text(spec)
        if text in found:
            found[text].append(spec)
        elif spec.get("State") not in ("skipped", "pending"):
            problems.append(f"unexpected spec {spec.get('State')}: {text}")

    for text, name in expected.items():
        if len(found[text]) != 1:
            problems.append(f"expected one result for {name!r}, found {len(found[text])}")
        for spec in found[text]:
            state = str(spec.get("State"))
            print(f"{state.upper():<11} {seconds(spec.get('RunTime')):7.1f}s  {name}")
            message = ((spec.get("Failure") or {}).get("Message") or "").strip()
            if message:
                print(f"    {message.splitlines()[0]}")
            if state != "passed":
                problems.append(f"{name!r} {state}")
    print(f"Suite run time: {seconds(report.get('RunTime')):.1f}s")
    return problems


def main(args):
    if args == ["focus"]:
        print(focus())
        return 0
    if len(args) == 2 and args[0] == "validate":
        problems = validate(args[1])
        for problem in problems:
            print(f"windows-npm-lite-upstream: {problem}", file=sys.stderr)
        return 1 if problems else 0
    print(__doc__.strip(), file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
