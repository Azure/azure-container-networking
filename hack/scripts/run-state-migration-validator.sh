#!/usr/bin/env bash

set -euo pipefail

if [[ $# -lt 5 || $4 != "--" ]]; then
	echo "usage: $0 <attempts> <interval-seconds> <diagnostics-dir> -- <command> [args...]" >&2
	exit 2
fi

attempts=$1
interval_seconds=$2
diagnostics_dir=$3
shift 4

if [[ ! "$attempts" =~ ^[1-9][0-9]*$ ]]; then
	echo "attempts must be a positive integer: $attempts" >&2
	exit 2
fi
if [[ ! "$interval_seconds" =~ ^[0-9]+$ ]]; then
	echo "interval-seconds must be a nonnegative integer: $interval_seconds" >&2
	exit 2
fi

mkdir -p "$diagnostics_dir"
rm -f "$diagnostics_dir"/attempt-*.log
: >"$diagnostics_dir/status.tsv"

nonretryable_pattern='state is empty|failed to (unmarshal|parse)|invalid (state |live pod )?ip|duplicate|unsupported|corrupt|malformed|schema mismatch|summary .*invalid'
retryable_pattern='state file validation failed|failed to exec into privileged pod|failed to get privileged pod|there are no privileged pods|connection refused|i/o timeout|tls handshake timeout|transport is closing|unexpected eof|not yet converged|timed out waiting'

for ((attempt = 1; attempt <= attempts; attempt++)); do
	log_path=$(printf "%s/attempt-%02d.log" "$diagnostics_dir" "$attempt")
	echo "state validator attempt $attempt of $attempts"

	set +e
	"$@" 2>&1 | tee "$log_path"
	pipeline_status=("${PIPESTATUS[@]}")
	set -e

	command_status=${pipeline_status[0]}
	tee_status=${pipeline_status[1]}
	printf "%d\t%d\n" "$attempt" "$command_status" >>"$diagnostics_dir/status.tsv"
	if ((tee_status != 0)); then
		echo "writing validator diagnostics failed with status $tee_status" >&2
		exit "$tee_status"
	fi
	if ((command_status == 0)); then
		exit 0
	fi
	if grep -Eiq "$nonretryable_pattern" "$log_path"; then
		echo "state validator reported a nonretryable semantic failure on attempt $attempt" >&2
		exit "$command_status"
	fi
	if ! grep -Eiq "$retryable_pattern" "$log_path"; then
		echo "state validator failure was not classified as retryable on attempt $attempt" >&2
		exit "$command_status"
	fi
	if ((attempt == attempts)); then
		echo "state validator exhausted $attempts attempts" >&2
		exit "$command_status"
	fi

	echo "state validator has not converged; retrying in $interval_seconds seconds" >&2
	sleep "$interval_seconds"
done
