#!/usr/bin/env python3
"""Fail-closed Copilot hook blocking public GitHub mutations."""

from __future__ import annotations

import json
import os
import re
import shlex
import sys
from typing import Any


DENIAL_REASON = (
    "Repository public identity boundary: AI agents may prepare local work "
    "but must not mutate GitHub or push as a contributor."
)
FORGE_CLI_PATTERN = re.compile(
    r"(?<![\w.-])(?:[^\s;&|()]+/)?(?:gh|hub)(?:\.exe)?(?=\s|$)",
    re.IGNORECASE,
)
GITHUB_HOST_PATTERN = re.compile(
    r"(?:api\.github\.com|github\.com)",
    re.IGNORECASE,
)
HTTP_MUTATION_PATTERN = re.compile(
    r"(?:"
    r"(?:^|\s)(?:-X|--request)(?:=|\s+)(?:POST|PUT|PATCH|DELETE)\b"
    r"|(?:^|\s)(?:-d|--data(?:-raw|-binary|-urlencode)?)(?:=|\s)"
    r"|(?:^|\s)(?:-T|--upload-file)(?:=|\s)"
    r"|\bmutation\b"
    r"|\bgit-receive-pack\b"
    r")",
    re.IGNORECASE,
)


def decision(value: str, reason: str = "") -> None:
    payload = {"permissionDecision": value}
    if reason:
        payload["permissionDecisionReason"] = reason
    print(json.dumps(payload))


def tokens(command: str) -> list[str]:
    try:
        lexer = shlex.shlex(command, posix=True, punctuation_chars=";&|()")
        lexer.whitespace_split = True
        lexer.commenters = ""
        return list(lexer)
    except ValueError:
        return []


def is_git_push(command: str) -> bool:
    parsed = tokens(command)
    for index, token in enumerate(parsed):
        if os.path.basename(token).lower() not in {"git", "git.exe"}:
            continue
        cursor = index + 1
        while cursor < len(parsed):
            candidate = parsed[cursor]
            if candidate in {"-C", "-c", "--git-dir", "--work-tree"}:
                cursor += 2
                continue
            if candidate.startswith(("--git-dir=", "--work-tree=", "--namespace=")):
                cursor += 1
                continue
            if candidate.startswith("-"):
                cursor += 1
                continue
            return candidate.lower() in {"push", "send-pack"}
    return False


def denied(payload: dict[str, Any]) -> bool:
    tool_name = payload.get("toolName", payload.get("tool_name", ""))
    tool_args = payload.get("toolArgs", payload.get("tool_input"))
    if not isinstance(tool_name, str):
        return True

    lowered_tool_name = tool_name.lower()
    normalized = tool_name.rsplit(".", 1)[-1].lower()
    if "github" in lowered_tool_name and normalized not in {"web_fetch", "web_search"}:
        return True
    if normalized not in {"bash", "powershell"}:
        return False

    if isinstance(tool_args, str):
        try:
            tool_args = json.loads(tool_args)
        except json.JSONDecodeError:
            return True
    if not isinstance(tool_args, dict):
        return True
    command = tool_args.get("command", tool_args.get("script"))
    if not isinstance(command, str):
        return True
    return (
        FORGE_CLI_PATTERN.search(command) is not None
        or is_git_push(command)
        or (
            GITHUB_HOST_PATTERN.search(command) is not None
            and HTTP_MUTATION_PATTERN.search(command) is not None
        )
    )


def main() -> int:
    try:
        payload = json.load(sys.stdin)
    except (json.JSONDecodeError, OSError):
        decision("deny", DENIAL_REASON)
        return 0
    if not isinstance(payload, dict) or denied(payload):
        decision("deny", DENIAL_REASON)
    else:
        print("{}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
