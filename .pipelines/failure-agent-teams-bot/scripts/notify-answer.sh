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

# Tag/stage label for the reply and standalone card. Defaults to the build-ask
# tag; history mode sets NOTIFY_ANSWER_TAG=faa-history so its replies are
# distinguishable and don't reuse the build-ask thread semantics.
TAG="${NOTIFY_ANSWER_TAG:-faa-ask}"

# When NOTIFY_ANSWER_STANDALONE is set (history mode), there is never a prior card
# to thread onto, so skip the notify_reply attempt (which would always 404) and
# post the standalone card directly. Build-ask leaves this unset and threads.
STANDALONE="${NOTIFY_ANSWER_STANDALONE:-}"

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

# Build-ask threads the answer onto the target run's existing FAA card. History
# mode (NOTIFY_ANSWER_STANDALONE set) has no such card, so it skips straight to
# the standalone card below.
if [[ -z "$STANDALONE" ]]; then
  reply_args=(--text "$text" --tag "$TAG")
  if [[ -n "$INITIATOR_UPN" ]]; then
    reply_args+=(--mention-user "$INITIATOR_UPN")
  fi

  # Try to thread the answer onto the target run's existing FAA card. notify_reply
  # is best-effort (always returns 0), so detect delivery from its output: it
  # prints "replied (HTTP 2xx)" on success, or a non-2xx line (e.g. 404 when no
  # thread exists for this build) on failure.
  reply_out="$(notify_reply "${reply_args[@]}" 2>&1)"
  printf '%s\n' "$reply_out"

  if printf '%s' "$reply_out" | grep -q 'replied (HTTP'; then
    exit 0
  fi

  # No thread to reply to (build never posted a card, or the thread aged out).
  # Fall through to a standalone card so the answer still reaches the channel.
  echo "notify-answer: no existing thread; posting a standalone answer card" >&2
fi

# Standalone card: reaches the channel with no prior card, and creates the
# (source, runId) thread so any later reply can attach to it.
card_args=(--status succeeded --stage "$TAG" --severity info \
  --title "FAA answer" --summary "$text")
if [[ -n "$INITIATOR_UPN" ]]; then
  card_args+=(--cc-label "Asked by" --cc-user "$INITIATOR_UPN")
fi
notify_status "${card_args[@]}"
