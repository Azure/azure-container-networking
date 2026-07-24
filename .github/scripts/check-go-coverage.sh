#!/usr/bin/env bash

set -euo pipefail
export LC_ALL=C

if [[ $# -ne 1 || ! "$1" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
	echo "usage: $0 <minimum-percent>" >&2
	exit 2
fi

awk -v threshold="$1" '
	$1 == "total:" && $(NF - 1) == "(statements)" && $NF ~ /^[0-9]+([.][0-9]+)?%$/ {
		total = $NF
		sub(/%$/, "", total)
		found++
	}
	END {
		if (found != 1) {
			print "coverage check: malformed go tool cover report" > "/dev/stderr"
			exit 2
		}
		if ((total + 0) < (threshold + 0)) {
			printf "coverage check: %.1f%% is below %.1f%%\n", total, threshold > "/dev/stderr"
			exit 1
		}
		printf "coverage check: %.1f%% meets %.1f%% threshold\n", total, threshold
	}
'
