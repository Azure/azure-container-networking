#!/usr/bin/env bash
#
# Posts a `/faa` ask-mode answer.md to the shared ACN Pipeline Notifier bot as a
# threaded reply that @mentions the initiator, reusing notify_reply from
# notify-bot.sh. The reply threads onto the target run's card because the calling
# task sets NOTIFY_RUN_ID = targetBuildId.
#
# Usage (from an AzureCLI@2 step with addSpnToEnvironment: true):
#   notify-answer.sh <answer.md> [initiator-upn]
#
# initiator-upn may also be supplied via the INITIATOR_UPN env var. Requires the
# same env as notify-bot.sh (NOTIFIER_*, NOTIFY_*), exported by the caller.
#
# Best-effort: a missing/empty answer file is a quiet no-op and never fails the
# build. The answer is already Markdown, so no rendering is needed.

set -uo pipefail

ANSWER="${1:-}"
INITIATOR_UPN="${2:-${INITIATOR_UPN:-}}"

if [[ -z "$ANSWER" ]]; then
  echo "notify-answer: usage: notify-answer.sh <answer.md> [initiator-upn]" >&2
  exit 0
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=notify-bot.sh
source "$SCRIPT_DIR/notify-bot.sh"

if [[ ! -f "$ANSWER" ]]; then
  echo "notify-answer: no answer file at $ANSWER, skipping" >&2
  exit 0
fi

text="$(cat "$ANSWER")"
if [[ -z "${text//[[:space:]]/}" ]]; then
  echo "notify-answer: answer is empty, skipping" >&2
  exit 0
fi

reply_args=(--text "$text" --tag "faa-ask")
if [[ -n "$INITIATOR_UPN" ]]; then
  reply_args+=(--mention-user "$INITIATOR_UPN")
fi

notify_reply "${reply_args[@]}"
