#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
cd "$root"

python3 .pipelines/cni/state-migration-json/tests/pipeline_contract_test.py
