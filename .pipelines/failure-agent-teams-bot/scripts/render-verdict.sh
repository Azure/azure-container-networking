#!/usr/bin/env bash
#
# render_verdict <incident.json>
#
# Echoes a Markdown block that renders the failure-agent's full root-cause
# verdict — leading with the likely root cause (falling back to the final
# verdict when no root-cause summary is present), then top anomaly, failing
# unit, the symptom-vs-cause split, the cited causal chain, the falsification
# test, node assessment, top evidence, the concrete evidence snippets (capped
# and truncated), evidence gaps, known unknowns, and the recommended action /
# proposed fix — in the same engineer-facing shape as a hand-written triage
# writeup. This is what lets the Teams ping deliver a grounded verdict instead
# of a "check these logs" pointer.
#
# Best-effort and side-effect free: a missing file or missing jq yields empty
# output and never fails. Each section is independently type-guarded, so a field
# the model returns in an unexpected shape skips only its own section rather than
# blanking the whole verdict; an incident that only carries a proposed fix still
# renders cleanly.
#
# Sourced by notify-incident.sh to render the incident verdict for the Teams ping.

render_verdict() {
  local incident="${1:-}"
  [[ -n "$incident" && -f "$incident" ]] || return 0
  command -v jq >/dev/null 2>&1 || return 0

  # Each section is wrapped in `try … catch empty` and every field access is
  # type-guarded (str/objects/arrays-of-objects), so a single field the model
  # returns in an unexpected shape skips only its own section instead of
  # throwing and blanking the whole verdict (which would drop the caller to its
  # thin legacy fallback). A well-formed incident renders identically to before.
  jq -r '
    # Only render a scalar field when it is actually a non-empty string.
    def str($v): ($v // "") | if type == "string" and . != "" then . else empty end;
    def line($label; $val): (str($val) | "**" + $label + "**\n" + .);
    # Arrays reduced to just their object / string elements, empty on any drift.
    def objs($v):  ($v // []) | if type == "array" then map(select(type == "object")) else [] end;
    def strs($v):  ($v // []) | if type == "array" then map(select(type == "string" and . != "")) else [] end;
    [
      (try line("Likely root cause";
           (if (str(.rootCauseSummary) // "") != "" then .rootCauseSummary else .finalVerdict end)) catch empty),
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
