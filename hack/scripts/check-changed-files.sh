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

RUN_WINDOWS_TESTS=false
if [ -z "$CHANGED_FILES" ]; then
  echo "No files changed, running all"
  RUN_WINDOWS_TESTS=true
else
  echo "$CHANGED_FILES"
fi

# Check if all changed files match Linux/test patterns
LINUX_TEST_PATTERNS=(".*linux\.go$" ".*test\.go$")

for file in $CHANGED_FILES; do
  match_found=false
  for pattern in "${LINUX_TEST_PATTERNS[@]}"; do
    if [[ "$file" =~ $pattern ]]; then
      match_found=true
      break
    fi
  done
  if [ "$match_found" = false ]; then
    RUN_WINDOWS_TESTS=true
    break
  fi
done

echo "Run Windows Tests: $RUN_WINDOWS_TESTS"

# Check if any cilium-relevant directories were modified
CILIUM_DIRS=("cns/" "azure-ipam/" "azure-ip-masq-merger/" "azure-iptables-monitor/" "bpf-prog/")
RUN_CILIUM_TESTS=false
if [ -z "$CHANGED_FILES" ]; then
  echo "No files changed, running all (including cilium)"
  RUN_CILIUM_TESTS=true
else
  for file in $CHANGED_FILES; do
    for dir in "${CILIUM_DIRS[@]}"; do
      if [[ "$file" == ${dir}* ]]; then
        RUN_CILIUM_TESTS=true
        break 2
      fi
    done
  done
fi

echo "Run Cilium Tests: $RUN_CILIUM_TESTS"

# Output in a machine-readable way
echo "RUN_WINDOWS_TESTS=$RUN_WINDOWS_TESTS"
echo "RUN_CILIUM_TESTS=$RUN_CILIUM_TESTS"
