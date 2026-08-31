#!/usr/bin/env bash

set -euo pipefail
export LC_ALL=C

if [[ $# -ne 2 || ! "$2" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
	echo "usage: $0 <coverprofile> <minimum-percent>" >&2
	exit 2
fi

profile="$1"
threshold="$2"
if [[ ! -f "$profile" || ! -r "$profile" ]]; then
	echo "coverage check: cannot read profile $profile" >&2
	exit 2
fi
awk -v threshold="$threshold" '
	BEGIN {
		if ((threshold + 0) > 100) {
			print "coverage check: minimum percent must not exceed 100" > "/dev/stderr"
			invalid = 1
			exit 2
		}
	}

	function malformed(detail) {
		printf "coverage check: malformed profile at line %d: %s\n", NR, detail > "/dev/stderr"
		invalid = 1
		exit 2
	}

	NR == 1 {
		if ($0 !~ /^mode: (set|count|atomic)$/) {
			malformed("invalid mode header")
		}
		header = 1
		next
	}

	$1 == "mode:" {
		malformed("multiple mode headers")
	}

	{
		if (NF != 3) {
			malformed("data row must have three fields")
		}
		if ($1 !~ /^.+:[0-9]+\.[0-9]+,[0-9]+\.[0-9]+$/) {
			malformed("invalid source range")
		}
		if ($2 !~ /^[0-9]+$/) {
			malformed("statement count is not a non-negative integer")
		}
		if ($3 !~ /^[0-9]+$/) {
			malformed("execution count is not a non-negative integer")
		}
		statements = $2 + 0
		total += statements
		if (($3 + 0) > 0) {
			covered += statements
		}
		rows++
	}

	END {
		if (invalid) {
			exit 2
		}
		if (header != 1 || rows == 0) {
			print "coverage check: profile has no data rows" > "/dev/stderr"
			exit 2
		}
		if (total <= 0) {
			print "coverage check: profile has zero statements" > "/dev/stderr"
			exit 2
		}
		percent = covered * 100 / total
		if ((covered * 100) < ((threshold + 0) * total)) {
			printf "coverage check: %d/%d statements = %.9f%% is below %.6f%%\n", \
				covered, total, percent, threshold > "/dev/stderr"
			exit 1
		}
		printf "coverage check: %d/%d statements = %.9f%% meets %.6f%% threshold\n", \
			covered, total, percent, threshold
	}
' "$profile"
