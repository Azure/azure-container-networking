#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
cd "$root"

if ! python3 -c 'import yaml' >/dev/null 2>&1; then
	echo "PyYAML is required: install it with 'python3 -m pip install PyYAML'" >&2
	exit 1
fi

python3 .pipelines/cni/state-migration-json/tests/pipeline_contract_test.py
