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
# output and never fails. Absent fields are skipped, so an older incident that
# only carries a proposed fix still renders cleanly (the caller falls back).
#
# Sourced by notify-incident.sh (single-incident ping) and notify-run-summary.sh
# (multi-stage aggregation) so both render the verdict identically.

render_verdict() {
  local incident="${1:-}"
  [[ -n "$incident" && -f "$incident" ]] || return 0
  command -v jq >/dev/null 2>&1 || return 0

  jq -r '
    def line($label; $val): if ($val // "") != "" then "**" + $label + "**\n" + $val else empty end;
    [
      line("Likely root cause";
           (if ((.rootCauseSummary // "") != "") then .rootCauseSummary else (.finalVerdict // "") end)),
      line("Top anomaly"; .topAnomaly),
      line("Failing unit"; .failingUnit),
      ( if ((.symptomVsCause // []) | length) > 0 then
          "**Symptom vs cause**\n" +
          ((.symptomVsCause // [])
            | map("- `" + (.classification // "?") + "` — " + (.signal // "")
                  + (if (.justification // "") != "" then " (" + .justification + ")" else "" end))
            | join("\n"))
        else empty end ),
      ( if ((.causalChain // []) | length) > 0 then
          "**Causal chain**\n" +
          ((.causalChain // []) | to_entries
            | map(((.key + 1) | tostring) + ". " + (.value.step // "")
                  + (if (.value.timestamp // "") != "" then " _[" + .value.timestamp + "]_" else "" end)
                  + (if (.value.citation // "") != "" then " — " + .value.citation else "" end))
            | join("\n"))
        else empty end ),
      ( if ((.falsification // null) != null) and ((.falsification.hypothesis // "") != "") then
          "**Falsification** — " + (.falsification.outcome // "inconclusive") + "\n"
          + "- Hypothesis: " + (.falsification.hypothesis // "") + "\n"
          + "- If true: " + (.falsification.ifTrueExpect // "") + "\n"
          + "- If false: " + (.falsification.ifFalseExpect // "") + "\n"
          + "- Observed: " + (.falsification.correlationResult // "")
        else empty end ),
      line("Node assessment"; .nodeAssessment),
      ( if ((.topEvidence // []) | length) > 0 then
          "**Top evidence**\n" + ((.topEvidence // []) | map("- " + .) | join("\n"))
        else empty end ),
      ( if ((.errorSnippets // []) | length) > 0 then
          "**Evidence snippets**\n" +
          (( (.errorSnippets // [])[0:2] )
            | map(
                ((.snippet // "") | split("\n")) as $lines
                | "**" + (.file // "?") + ":" + ((.line // 0) | tostring) + "**\n"
                  + "```text\n"
                  + ($lines[0:12] | join("\n"))
                  + (if ($lines | length) > 12 then "\n… (truncated)" else "" end)
                  + "\n```")
            | join("\n\n"))
        else empty end ),
      ( if ((.evidenceGaps // []) | length) > 0 then
          "**Evidence gaps — capture next run**\n" +
          ((.evidenceGaps // [])
            | map("- " + (.missing // "")
                  + (if (.whereItLives // "") != "" then " @ `" + .whereItLives + "`" else "" end)
                  + (if (.whyMissing // "") != "" then " — " + .whyMissing else "" end)
                  + (if (.howToCapture // "") != "" then " — capture: `" + .howToCapture + "`" else "" end))
            | join("\n"))
        else empty end ),
      ( if ((.knownUnknowns // []) | length) > 0 then
          "**Known unknowns**\n" + ((.knownUnknowns // []) | map("- " + .) | join("\n"))
        else empty end ),
      line("What to do"; .recommendedAction),
      line("Proposed fix"; .proposedFix)
    ]
    | join("\n\n")
  ' "$incident" 2>/dev/null || return 0
}
