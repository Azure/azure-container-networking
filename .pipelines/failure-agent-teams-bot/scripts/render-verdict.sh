#!/usr/bin/env bash
#
# render_verdict <incident.json>
#
# Echoes the Markdown body for the Teams incident reply.
#
# Preferred path — verdict-led: when the incident carries the model's
# self-contained `finalVerdict`, that narrative IS the reply. finalVerdict is
# already written as a hand-triage-style writeup — it leads with the confirmed
# root cause and its cited source, quotes the confirming artifact in a code
# fence, rejects the wrong hypothesis (symptom vs cause), reasons about
# expired/absent evidence, carries a cross-node/stage/image uniformity table
# when that is the falsification signal, and ends with owner routing plus the
# concrete "what should be done" steps. We render it verbatim and append only
# the two concrete, non-prose artifacts that reinforce it without restating the
# structured fields the narrative already covers: the raw evidence snippets
# (file:line log excerpts) and the "capture next run" evidence-gap list. This
# keeps the reply a clean, grounded verdict rather than finalVerdict followed by
# a full restatement of every structured field.
#
# Fallback path — structured: incidents that predate finalVerdict (older
# schema) render the full labeled triage assembled from the individual contract
# fields (root cause, top anomaly, failing unit, symptom vs cause, causal chain,
# falsification, node assessment, evidence, gaps, recommended action / fix), so
# nothing is lost for legacy incidents.
#
# Best-effort and side-effect free: a missing file or missing jq yields empty
# output and never fails. Every field access is type-guarded, so a field the
# model returns in an unexpected shape skips only its own section rather than
# blanking the whole verdict.
#
# Sourced by notify-incident.sh to render the incident verdict for the Teams ping.

render_verdict() {
  local incident="${1:-}"
  [[ -n "$incident" && -f "$incident" ]] || return 0
  command -v jq >/dev/null 2>&1 || return 0

  # A non-empty string finalVerdict selects the verdict-led path.
  local final_verdict body
  final_verdict="$(jq -r '(.finalVerdict // "") | if type == "string" then . else "" end' "$incident" 2>/dev/null || true)"
  if [[ -n "$(printf '%s' "$final_verdict" | tr -d '[:space:]')" ]]; then
    body="$(_render_verdict_led "$incident")"
  else
    body="$(_render_structured "$incident")"
  fi

  # Nothing to say — stay silent so notify-incident.sh can fall back.
  [[ -n "$(printf '%s' "$body" | tr -d '[:space:]')" ]] || return 0

  # The verdict lands verbatim in the Teams reply, but a Teams Adaptive Card
  # TextBlock renders only a subset of Markdown: fenced code blocks and pipe
  # tables do not render, so flatten them to the supported subset (inline-code
  # bullets and "cell — cell" lines) before sending. The rich report.md keeps
  # the original Markdown — only the Teams reply is downgraded.
  body="$(printf '%s' "$body" | _sanitize_teams_md)"

  # The notifier rejects a reply whose text exceeds 2000 characters (HTTP 400),
  # so keep the rendered verdict comfortably under that, leaving headroom for the
  # retention line notify-incident.sh appends after this returns. notify_reply
  # enforces the hard 2000 limit as a backstop; this cap is the graceful one that
  # points the on-call at the full write-up.
  local max=1800
  if (( ${#body} > max )); then
    body="${body:0:max}"$'\n\n_… verdict truncated; open the run’s analysis summary for the full write-up._'
  fi

  printf '%s\n' "$body"
}

# _sanitize_teams_md flattens Markdown that a Teams Adaptive Card TextBlock does
# not render into the subset it does. Reads stdin, writes stdout.
#   - fenced code blocks (``` … ```)      -> each inner line as an inline-code bullet
#   - pipe tables (| a | b |)             -> "- a — b" bullet rows (separator rows dropped)
#   - ATX headings (#, ##, …)             -> bold text
# Bold, italics, links, inline code, and ordered/unordered lists already render,
# so they pass through untouched.
_sanitize_teams_md() {
  awk '
    function emit(s) {
      if (pending_blank && s != "") { print ""; pending_blank = 0 }
      print s
      if (s == "") pending_blank = 0
    }
    BEGIN { in_fence = 0; pending_blank = 0 }
    {
      line = $0

      # Toggle fenced code blocks; drop the ``` marker lines themselves.
      if (line ~ /^[[:space:]]*```/) { in_fence = 1 - in_fence; next }
      if (in_fence) {
        l = line
        gsub(/`/, "", l)
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", l)
        if (l != "") emit("- `" l "`")
        next
      }

      # ATX heading -> bold (own paragraph).
      if (line ~ /^[[:space:]]*#{1,6}[[:space:]]+/) {
        sub(/^[[:space:]]*#{1,6}[[:space:]]+/, "", line)
        sub(/[[:space:]]*#*[[:space:]]*$/, "", line)
        emit("**" line "**"); pending_blank = 1
        next
      }

      # Drop table separator rows (|---|:--:|).
      if (index(line, "|") > 0) {
        t = line
        gsub(/[[:space:]|:\-]/, "", t)
        if (t == "" && line ~ /-/) next
      }

      # Flatten table data rows to "- cell — cell — cell".
      if (line ~ /^[[:space:]]*\|.*\|[[:space:]]*$/) {
        row = line
        sub(/^[[:space:]]*\|/, "", row)
        sub(/\|[[:space:]]*$/, "", row)
        n = split(row, cells, /\|/)
        out = ""
        for (i = 1; i <= n; i++) {
          c = cells[i]
          gsub(/^[[:space:]]+|[[:space:]]+$/, "", c)
          if (c == "") continue
          out = (out == "" ? c : out " — " c)
        }
        if (out != "") emit("- " out)
        next
      }

      # A whole-line-bold header renders on its own line only if a paragraph
      # break follows it — Teams collapses a single "\n" between two such lines.
      if (line ~ /^\*\*.+\*\*$/) { emit(line); pending_blank = 1; next }

      emit(line)
    }
  '
}

# _render_verdict_led emits finalVerdict verbatim, then the raw evidence snippets
# and the capture-next-run gaps as reinforcing appendices.
_render_verdict_led() {
  local incident="$1"
  jq -r '
    def str($v): ($v // "") | if type == "string" and . != "" then . else empty end;
    def objs($v): ($v // []) | if type == "array" then map(select(type == "object")) else [] end;
    [
      (try (str(.finalVerdict)) catch empty),
      (try ( (objs(.errorSnippets)[0:2]) as $es
        | if ($es | length) > 0 then
            "**Evidence snippets**\n" +
            ($es
              | map(
                  ((.snippet // "") | if type == "string" then . else "" end | split("\n")) as $lines
                  | "**" + (.file // "?") + ":" + ((.line // 0) | tostring) + "**\n"
                    + "```text\n"
                    + ($lines[0:12] | join("\n"))
                    + (if ($lines | length) > 12 then "\n… (truncated)" else "" end)
                    + "\n```")
              | join("\n\n"))
          else empty end ) catch empty),
      (try ( objs(.evidenceGaps) as $eg
        | if ($eg | length) > 0 then
            "**Evidence gaps — capture next run**\n" +
            ($eg
              | map("- " + (.missing // "")
                    + (if (.whereItLives // "") != "" then " @ `" + .whereItLives + "`" else "" end)
                    + (if (.whyMissing // "") != "" then " — " + .whyMissing else "" end)
                    + (if (.howToCapture // "") != "" then " — capture: `" + .howToCapture + "`" else "" end))
              | join("\n"))
          else empty end ) catch empty)
    ]
    | map(select(. != null and . != ""))
    | join("\n\n")
  ' "$incident" 2>/dev/null || return 0
}

# _render_structured is the legacy full triage render for incidents that carry
# no finalVerdict. Each section is wrapped in `try … catch empty` and every
# field access is type-guarded (str/objects/arrays-of-objects), so a single
# field the model returns in an unexpected shape skips only its own section
# instead of throwing and blanking the whole verdict.
_render_structured() {
  local incident="$1"
  jq -r '
    # Only render a scalar field when it is actually a non-empty string.
    def str($v): ($v // "") | if type == "string" and . != "" then . else empty end;
    def line($label; $val): (str($val) | "**" + $label + "**\n" + .);
    # Arrays reduced to just their object / string elements, empty on any drift.
    def objs($v):  ($v // []) | if type == "array" then map(select(type == "object")) else [] end;
    def strs($v):  ($v // []) | if type == "array" then map(select(type == "string" and . != "")) else [] end;
    [
      (try line("Likely root cause"; .rootCauseSummary) catch empty),
      (try line("Top anomaly"; .topAnomaly) catch empty),
      (try line("Failing unit"; .failingUnit) catch empty),
      (try ( objs(.symptomVsCause) as $sc
        | if ($sc | length) > 0 then
            "**Symptom vs cause**\n" +
            ($sc
              | map("- `" + (.classification // "?") + "` — " + (.signal // "")
                    + (if (.justification // "") != "" then " (" + .justification + ")" else "" end))
              | join("\n"))
          else empty end ) catch empty),
      (try ( objs(.causalChain) as $cc
        | if ($cc | length) > 0 then
            "**Causal chain**\n" +
            ($cc | to_entries
              | map(((.key + 1) | tostring) + ". " + (.value.step // "")
                    + (if (.value.timestamp // "") != "" then " _[" + .value.timestamp + "]_" else "" end)
                    + (if (.value.citation // "") != "" then " — " + .value.citation else "" end))
              | join("\n"))
          else empty end ) catch empty),
      (try ( (.falsification | if type == "object" then . else {} end) as $f
        | if (($f.hypothesis // "") != "") then
            "**Falsification** — " + ($f.outcome // "inconclusive") + "\n"
            + "- Hypothesis: " + ($f.hypothesis // "") + "\n"
            + "- If true: " + ($f.ifTrueExpect // "") + "\n"
            + "- If false: " + ($f.ifFalseExpect // "") + "\n"
            + "- Observed: " + ($f.correlationResult // "")
          else empty end ) catch empty),
      (try line("Node assessment"; .nodeAssessment) catch empty),
      (try ( strs(.topEvidence) as $te
        | if ($te | length) > 0 then
            "**Top evidence**\n" + ($te | map("- " + .) | join("\n"))
          else empty end ) catch empty),
      (try ( (objs(.errorSnippets)[0:2]) as $es
        | if ($es | length) > 0 then
            "**Evidence snippets**\n" +
            ($es
              | map(
                  ((.snippet // "") | if type == "string" then . else "" end | split("\n")) as $lines
                  | "**" + (.file // "?") + ":" + ((.line // 0) | tostring) + "**\n"
                    + "```text\n"
                    + ($lines[0:12] | join("\n"))
                    + (if ($lines | length) > 12 then "\n… (truncated)" else "" end)
                    + "\n```")
              | join("\n\n"))
          else empty end ) catch empty),
      (try ( objs(.evidenceGaps) as $eg
        | if ($eg | length) > 0 then
            "**Evidence gaps — capture next run**\n" +
            ($eg
              | map("- " + (.missing // "")
                    + (if (.whereItLives // "") != "" then " @ `" + .whereItLives + "`" else "" end)
                    + (if (.whyMissing // "") != "" then " — " + .whyMissing else "" end)
                    + (if (.howToCapture // "") != "" then " — capture: `" + .howToCapture + "`" else "" end))
              | join("\n"))
          else empty end ) catch empty),
      (try ( strs(.knownUnknowns) as $ku
        | if ($ku | length) > 0 then
            "**Known unknowns**\n" + ($ku | map("- " + .) | join("\n"))
          else empty end ) catch empty),
      (try line("What to do"; .recommendedAction) catch empty),
      (try line("Proposed fix"; .proposedFix) catch empty)
    ]
    | join("\n\n")
  ' "$incident" 2>/dev/null || return 0
}
