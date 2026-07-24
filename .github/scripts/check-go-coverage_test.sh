#!/usr/bin/env bash

set -euo pipefail

script="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/check-go-coverage.sh"

report() {
	printf 'example.go:1:\tExample\t100.0%%\ntotal:\t(statements)\t%s%%\n' "$1"
}

report 90.1 | "$script" 90 >/dev/null
report 90.0 | "$script" 90 >/dev/null

if report 89.9 | "$script" 90 >/dev/null 2>&1; then
	echo "below-threshold coverage unexpectedly passed" >&2
	exit 1
fi

if printf 'not a coverage report\n' | "$script" 90 >/dev/null 2>&1; then
	echo "malformed coverage unexpectedly passed" >&2
	exit 1
fi
