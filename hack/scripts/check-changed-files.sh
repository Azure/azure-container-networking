#!/usr/bin/env bash
set -e

# Usage: check-changed-files.sh <target-branch>
# Outputs two lines:
#   RUN_WINDOWS_TESTS=true|false
#   RUN_CILIUM_TESTS=true|false

TARGET_BRANCH="${1:?target branch is required}"

# Get the merge base to compare against the common ancestor
MERGE_BASE=$(git merge-base HEAD "origin/$TARGET_BRANCH")
echo "Merge base commit: $MERGE_BASE"

echo "=== Files Changed Compared to $TARGET_BRANCH ==="
CHANGED_FILES=$(git diff --name-only "$MERGE_BASE...HEAD")

if [ -z "$CHANGED_FILES" ]; then
  echo "No files changed, running all tests"
  echo "RUN_WINDOWS_TESTS=true"
  echo "RUN_CILIUM_TESTS=true"
  exit 0
fi

echo "$CHANGED_FILES"

# all_files_match_skip_patterns <changed_files> <patterns...>
# Returns 0 (true) if every changed file matches at least one of the
# supplied patterns (regex for =~ match). Returns 1 otherwise.
all_files_match_skip_patterns() {
  local changed_files="$1"
  shift
  local patterns=("$@")

  for file in $changed_files; do
    local match_found=false
    for pattern in "${patterns[@]}"; do
      if [[ "$file" =~ $pattern ]]; then
        match_found=true
        break
      fi
    done
    if [[ "$match_found" == false ]]; then
      return 1
    fi
  done
  return 0
}

# --- Windows tests ---
WINDOWS_SKIP_PATTERNS=("_linux\.go$" "_linux_test\.go$")

if all_files_match_skip_patterns "$CHANGED_FILES" "${WINDOWS_SKIP_PATTERNS[@]}"; then
  RUN_WINDOWS_TESTS=false
else
  RUN_WINDOWS_TESTS=true
fi
echo "Run Windows Tests: $RUN_WINDOWS_TESTS"

# --- Cilium tests ---
CILIUM_SKIP_PATTERNS=("^cni/" "^ipam/" "^network/")

if all_files_match_skip_patterns "$CHANGED_FILES" "${CILIUM_SKIP_PATTERNS[@]}"; then
  RUN_CILIUM_TESTS=false
else
  RUN_CILIUM_TESTS=true
fi
echo "Run Cilium Tests: $RUN_CILIUM_TESTS"


echo "RUN_WINDOWS_TESTS=$RUN_WINDOWS_TESTS"
echo "RUN_CILIUM_TESTS=$RUN_CILIUM_TESTS"
