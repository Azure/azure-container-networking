#!/usr/bin/env bash

set -euo pipefail

script="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/check-go-coverage.sh"
work_dir="${CNS_STATE_COVERAGE_TEST_DIR:-$(pwd)/output/coverage-check-tests}"
profile="$work_dir/profile.out"
mkdir -p "$work_dir"
trap 'rm -rf "$work_dir"' EXIT

write_profile() {
	local covered="$1"
	local uncovered="$2"
	cat >"$profile" <<EOF
mode: atomic
example.go:1.1,1.2 $covered 1
example.go:2.1,2.2 $uncovered 0
EOF
}

expect_pass() {
	local name="$1"
	if ! "$script" "$profile" 90 >/dev/null; then
		echo "$name unexpectedly failed" >&2
		exit 1
	fi
}

expect_fail() {
	local name="$1"
	if "$script" "$profile" 90 >/dev/null 2>&1; then
		echo "$name unexpectedly passed" >&2
		exit 1
	fi
}

for percentage in 8995 8997 8999; do
	write_profile "$percentage" "$((10000 - percentage))"
	expect_fail "${percentage:0:2}.${percentage:2} percent"
done

write_profile 9000 1000
output="$("$script" "$profile" 90)"
grep -Fq "9000/10000 statements = 90.000000000%" <<<"$output"

write_profile 9001 999
expect_pass "above-threshold coverage"

if "$script" "$work_dir/missing.out" 90 >/dev/null 2>&1; then
	echo "missing profile unexpectedly passed" >&2
	exit 1
fi

malformed_profiles=(
	""
	"mode: unknown"
	"mode: atomic"
	$'mode: atomic\nmode: atomic'
	$'mode: atomic\nnot-a-data-row'
	$'mode: atomic\nexample.go:bad 1 1'
	$'mode: atomic\nexample.go:1.1,1.2 words 1'
	$'mode: atomic\nexample.go:1.1,1.2 -1 1'
	$'mode: atomic\nexample.go:1.1,1.2 1 words'
	$'mode: atomic\nexample.go:1.1,1.2 1 -1'
	$'mode: atomic\nexample.go:1.1,1.2 0 1'
)
for contents in "${malformed_profiles[@]}"; do
	printf '%s\n' "$contents" >"$profile"
	expect_fail "malformed profile"
done

write_profile 1 0
if "$script" "$profile" invalid >/dev/null 2>&1; then
	echo "invalid threshold unexpectedly passed" >&2
	exit 1
fi
if "$script" "$profile" 100.1 >/dev/null 2>&1; then
	echo "threshold above 100 unexpectedly passed" >&2
	exit 1
fi
