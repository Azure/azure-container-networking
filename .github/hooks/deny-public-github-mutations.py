#!/usr/bin/env python3
"""Fail-closed Copilot hook blocking AI-authored GitHub communication."""

from __future__ import annotations

import json
import re
import shlex
import sys
from typing import Any


DENIAL_REASON = (
    "Public identity boundary: Copilot must not author comments, replies, "
    "reviews, reactions, thread resolutions, or PR/issue text as the user. "
    "Draft the exact text for the user to publish."
)
GITHUB_HOST_PATTERN = re.compile(
    r"(?:api\.github\.com|github\.com)",
    re.IGNORECASE,
)
HTTP_MUTATION_PATTERN = re.compile(
    r"(?:"
    r"(?:^|\s)(?:-X|--request|--method)(?:=|\s+)(?:POST|PUT|PATCH|DELETE)\b"
    r"|(?:^|\s)(?:-d|--data(?:-raw|-binary|-urlencode)?)(?:=|\s)"
    r"|(?:^|\s)(?:-T|--upload-file)(?:=|\s)"
    r")",
    re.IGNORECASE,
)
AUTHORSHIP_ENDPOINT_PATTERN = re.compile(
    r"(?:"
    r"/issues(?:\?|$)"
    r"|/issues/\d+/comments(?:/|$)"
    r"|/issues/comments/\d+(?:/|$)"
    r"|/pulls/\d+/(?:comments|reviews|requested_reviewers)(?:/|$)"
    r"|/pulls/comments/\d+(?:/|$)"
    r"|/reactions(?:/|$)"
    r"|/issues/\d+(?:\?|$)"
    r"|/pulls/\d+(?:\?|$)"
    r")",
    re.IGNORECASE,
)
GRAPHQL_AUTHORSHIP_PATTERN = re.compile(
    r"\b(?:"
    r"addComment"
    r"|addPullRequestReview(?:Comment|Thread)?"
    r"|addReaction"
    r"|closeIssue"
    r"|deleteIssueComment"
    r"|deletePullRequestReview(?:Comment)?"
    r"|dismissPullRequestReview"
    r"|lockLockable"
    r"|markPullRequestReadyForReview"
    r"|minimizeComment"
    r"|removeReaction"
    r"|reopenIssue"
    r"|resolveReviewThread"
    r"|submitPullRequestReview"
    r"|unmarkIssueAsDuplicate"
    r"|unresolveReviewThread"
    r"|unlockLockable"
    r"|updateIssue(?:Comment)?"
    r"|updatePullRequest"
    r")\b",
    re.IGNORECASE,
)
MCP_AUTHORSHIP_PATTERN = re.compile(
    r"(?:"
    r"(?:add|create|delete|dismiss|edit|resolve|submit|unresolve|update)"
    r".*(?:comment|reaction|review|thread)"
    r"|(?:close|edit|lock|reopen|unlock|update)_(?:issue|pull_request)"
    r"|create_(?:issue|pull_request)"
    r")",
    re.IGNORECASE,
)
BLOCKED_GH_ACTIONS = {
    "discussion": {
        "close",
        "comment",
        "create",
        "edit",
        "reopen",
    },
    "issue": {
        "close",
        "comment",
        "create",
        "edit",
        "lock",
        "pin",
        "reopen",
        "transfer",
        "unlock",
        "unpin",
    },
    "pr": {
        "close",
        "comment",
        "create",
        "edit",
        "lock",
        "ready",
        "reopen",
        "review",
        "unlock",
    },
}
PROTECTED_PATHS = (
    # Matching is substring containment, so bare filenames cover every install
    # location without hardcoding one: the repository copy under .github/hooks,
    # a per-user copy under ~/.copilot/hooks, and a system copy under
    # /usr/local/libexec. Keep these location-independent so the repository and
    # per-user copies of this script cannot drift apart.
    "deny-public-github-mutations.py",
    "public-identity-guard.json",
    "copilot-instructions.md",
    "agents.md",
)
FILE_MUTATION_PATTERN = re.compile(
    r"(?:"
    r"(?<![\w.-])(?:rm|mv|cp|install|chmod|chown|truncate|unlink)\b"
    r"|(?:^|\s)(?:sed|perl)\s+[^\n]*-[^\n]*i"
    r"|(?:^|\s)(?:tee|dd)\b"
    r"|>{1,2}"
    r")",
    re.IGNORECASE,
)
WRITE_TOOLS = {
    "apply_patch",
    "create",
    "edit",
    "str_replace_editor",
    "write",
}


def deny() -> None:
    print(
        json.dumps(
            {
                "permissionDecision": "deny",
                "permissionDecisionReason": DENIAL_REASON,
            }
        )
    )


def allow() -> None:
    print("{}")


def shell_tokens(command: str) -> list[str] | None:
    try:
        lexer = shlex.shlex(command, posix=True, punctuation_chars=";&|()")
        lexer.whitespace_split = True
        lexer.commenters = ""
        return list(lexer)
    except ValueError:
        return None


def gh_authorship_denied(command: str) -> bool:
    tokens = shell_tokens(command)
    if tokens is None:
        return True

    for index, token in enumerate(tokens):
        if token.rsplit("/", 1)[-1].lower() not in {"gh", "gh.exe", "hub", "hub.exe"}:
            continue
        segment: list[str] = []
        for candidate in tokens[index + 1 :]:
            if candidate in {";", "&&", "||", "|", "&", "(", ")"}:
                break
            segment.append(candidate.lower())

        for group, actions in BLOCKED_GH_ACTIONS.items():
            if group not in segment:
                continue
            group_index = segment.index(group)
            if any(action in segment[group_index + 1 :] for action in actions):
                return True

        if "api" in segment:
            if GRAPHQL_AUTHORSHIP_PATTERN.search(command):
                return True
            if HTTP_MUTATION_PATTERN.search(command) and AUTHORSHIP_ENDPOINT_PATTERN.search(command):
                return True
    return False


def shell_command_denied(command: str) -> bool:
    if any(path in command for path in PROTECTED_PATHS):
        if FILE_MUTATION_PATTERN.search(command):
            return True
    if gh_authorship_denied(command):
        return True
    return bool(
        GITHUB_HOST_PATTERN.search(command)
        and HTTP_MUTATION_PATTERN.search(command)
        and AUTHORSHIP_ENDPOINT_PATTERN.search(command)
    )


def tool_denied(tool_name: str, tool_args: Any) -> bool:
    lowered_tool_name = tool_name.lower()
    normalized = tool_name.rsplit(".", 1)[-1].lower()

    if "github" in lowered_tool_name and MCP_AUTHORSHIP_PATTERN.search(normalized):
        return True

    if normalized in WRITE_TOOLS:
        serialized_args = json.dumps(tool_args, sort_keys=True, default=str)
        return any(path in serialized_args for path in PROTECTED_PATHS)

    if normalized not in {"bash", "powershell"}:
        return False

    if isinstance(tool_args, str):
        try:
            tool_args = json.loads(tool_args)
        except json.JSONDecodeError:
            return True
    if not isinstance(tool_args, dict):
        return True

    command = tool_args.get("command")
    if command is None:
        command = tool_args.get("script")
    if not isinstance(command, str):
        return True
    return shell_command_denied(command)


def evaluate(payload: dict[str, Any]) -> bool:
    tool_name = payload.get("toolName", payload.get("tool_name", ""))
    tool_args = payload.get("toolArgs", payload.get("tool_input"))
    if not isinstance(tool_name, str) or not tool_name.strip():
        return True
    return tool_denied(tool_name, tool_args)


def self_test() -> None:
    denied = [
        {"toolName": "bash", "toolArgs": {"command": "gh pr comment 1 -b x"}},
        {"toolName": "bash", "toolArgs": {"command": "gh pr create --fill"}},
        {"toolName": "bash", "toolArgs": {"command": "gh issue create -t x -b y"}},
        {"toolName": "bash", "toolArgs": {"command": "gh pr view 'unterminated"}},
        {
            "toolName": "bash",
            "toolArgs": {"command": "gh pr review 1 --approve"},
        },
        {
            "toolName": "bash",
            "toolArgs": {
                "command": "gh api --method POST repos/o/r/issues/1/comments"
            },
        },
        {
            "toolName": "bash",
            "toolArgs": {
                "command": "gh api graphql -f "
                "'query=mutation { resolveReviewThread(input:{threadId:\"x\"}) { clientMutationId } }'"
            },
        },
        {"toolName": "github-mcp-server.add_issue_comment", "toolArgs": {}},
        {"toolName": "github-mcp-server.create_pull_request", "toolArgs": {}},
        {"toolName": "github-mcp-server.create_pull_request_review", "toolArgs": {}},
        {"toolArgs": {}},
        {"toolName": "", "toolArgs": {}},
        {"toolName": "  ", "toolArgs": {}},
        {
            "toolName": "apply_patch",
            "toolArgs": {
                "patch": "*** Update File: .github/hooks/public-identity-guard.json"
            },
        },
        {
            "toolName": "bash",
            "toolArgs": {
                "command": "rm .github/hooks/public-identity-guard.json"
            },
        },
        # The guard must protect itself wherever it is installed, not only in
        # the repository copy.
        {
            "toolName": "bash",
            "toolArgs": {
                "command": "rm ~/.copilot/hooks/public-identity-guard.json"
            },
        },
        {
            "toolName": "bash",
            "toolArgs": {
                "command": "sudo rm /opt/hooks/deny-public-github-mutations.py"
            },
        },
        {
            "toolName": "edit",
            "toolArgs": {
                "path": "~/.copilot/hooks/deny-public-github-mutations.py",
                "old_str": "deny()",
                "new_str": "allow()",
            },
        },
        {
            "toolName": "bash",
            "toolArgs": {"command": "sed -i 's/deny/allow/' agents.md"},
        },
        {
            "toolName": "bash",
            "toolArgs": {
                "command": "echo x > ~/.copilot/copilot-instructions.md"
            },
        },
    ]
    allowed = [
        {"toolName": "bash", "toolArgs": {"command": "git status --short"}},
        {"toolName": "bash", "toolArgs": {"command": "git fetch origin main"}},
        {"toolName": "bash", "toolArgs": {"command": "git push origin HEAD"}},
        {"toolName": "bash", "toolArgs": {"command": "gh pr view 1"}},
        {
            "toolName": "bash",
            "toolArgs": {"command": "gh pr view 1 & echo comment"},
        },
        {"toolName": "bash", "toolArgs": {"command": "gh workflow run ci.yml"}},
        {"toolName": "github-mcp-server.get_pull_request", "toolArgs": {}},
        {
            "toolName": "bash",
            "toolArgs": {"command": "curl https://api.github.com/repos/o/r"},
        },
        {
            "toolName": "web_fetch",
            "toolArgs": {"url": "https://github.com/o/r/pull/1"},
        },
        {
            "toolName": "bash",
            "toolArgs": {
                "command": "stat .github/hooks/public-identity-guard.json"
            },
        },
    ]

    assert all(evaluate(payload) for payload in denied)
    assert all(not evaluate(payload) for payload in allowed)


def main() -> int:
    if sys.argv[1:] == ["--self-test"]:
        self_test()
        return 0

    try:
        payload = json.load(sys.stdin)
    except (json.JSONDecodeError, OSError):
        deny()
        return 0

    if not isinstance(payload, dict) or evaluate(payload):
        deny()
    else:
        allow()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
