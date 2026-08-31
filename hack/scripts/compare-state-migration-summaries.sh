#!/usr/bin/env bash

set -euo pipefail

if [[ $# -ne 9 ]]; then
	echo "usage: $0 <baseline> <candidate> <expected-backend> <expected-authority> <expected-schema> <state-relation> <boot-relation> <baseline-pod-state> <candidate-pod-state>" >&2
	exit 2
fi

baseline=$1
candidate=$2
expected_backend=$3
expected_authority=$4
expected_schema=$5
state_relation=$6
boot_relation=$7
baseline_pod_state=$8
candidate_pod_state=$9

for artifact in "$baseline" "$candidate" "$baseline_pod_state" "$candidate_pod_state"; do
	if [[ ! -s "$artifact" ]]; then
		echo "state migration evidence is missing or empty: $artifact" >&2
		exit 1
	fi
done

case "$expected_backend" in
json | bolt) ;;
*)
	echo "unsupported expected backend: $expected_backend" >&2
	exit 2
	;;
esac

case "$state_relation" in
none | exact | changed) ;;
*)
	echo "unsupported state relation: $state_relation" >&2
	exit 2
	;;
esac

case "$boot_relation" in
none | same | changed) ;;
*)
	echo "unsupported boot relation: $boot_relation" >&2
	exit 2
	;;
esac

if [[ ! "$expected_schema" =~ ^[0-9]+$ ]]; then
	echo "expected schema must be an unsigned integer: $expected_schema" >&2
	exit 2
fi

if ! jq -e \
	--arg backend "$expected_backend" \
	--arg authority "$expected_authority" \
	--argjson schema "$expected_schema" '
	(.checks | type == "array" and length > 0)
	and all(.checks[];
		.validationPass == true
		and .converged == true
		and (((.missingIPs // []) | length) == 0)
		and (((.unexpectedIPs // []) | length) == 0)
		and (((.duplicateIPs // []) | length) == 0)
	)
	and (
		if $backend == "bolt" then
			([.checks[] | select((.stateBackend // "") != "")]) as $persistent
			| ($persistent | length) > 0
			and all($persistent[];
				.stateBackend == $backend
				and .authority == $authority
				and .schemaVersion == $schema
				and ((.bootID // "") | length) > 0
				and .dbFilePresent == true
				and .dbFileSizeBytes > 0
			)
		else
			all(.checks[]; (.stateBackend // "") == "")
		end
	)
' "$candidate" >/dev/null; then
	echo "candidate summary failed strict validation: $candidate" >&2
	jq . "$candidate" >&2 || true
	exit 1
fi

if ! jq -e -n \
	--slurpfile baseline "$baseline" \
	--slurpfile candidate "$candidate" \
	--slurpfile baselinePods "$baseline_pod_state" \
	--slurpfile candidatePods "$candidate_pod_state" \
	--arg relation "$state_relation" '
	def normalized_state($summary):
		[
			$summary.checks[]
			| select(.checkName != "cns persistent metadata")
			| {
				checkName,
				nodeName,
				expectedCount,
				actualCount,
				missingIPs: ((.missingIPs // []) | sort),
				unexpectedIPs: ((.unexpectedIPs // []) | sort),
				duplicateIPs: ((.duplicateIPs // []) | sort)
			}
		]
		| sort_by(.checkName, .nodeName);
	def normalized_pods($pods):
		[
			$pods[]
			| {
				namespace,
				name,
				nodeName,
				phase,
				podIPs: ((.podIPs // []) | sort)
			}
		]
		| sort_by(.namespace, .name);

	(normalized_state($baseline[0])) as $before
	| (normalized_state($candidate[0])) as $after
	| (normalized_pods($baselinePods[0])) as $beforePods
	| (normalized_pods($candidatePods[0])) as $afterPods
	| if $relation == "none" then
		true
	elif $relation == "exact" then
		$before == $after
		and ($beforePods | length) > 0
		and $beforePods == $afterPods
	else
		($beforePods | length) > 0
		and ($afterPods | length) > 0
		and $beforePods != $afterPods
		and $before != $after
	end
' >/dev/null; then
	echo "state relation '$state_relation' failed between $baseline and $candidate" >&2
	exit 1
fi

if ! jq -e -n \
	--slurpfile baseline "$baseline" \
	--slurpfile candidate "$candidate" \
	--arg relation "$boot_relation" '
	def boots($summary):
		[
			$summary.checks[]
			| select((.stateBackend // "") != "")
			| {nodeName, bootID}
		]
		| sort_by(.nodeName);

	(boots($baseline[0])) as $before
	| (boots($candidate[0])) as $after
	| if $relation == "none" then
		true
	elif $relation == "same" then
		($before | length) > 0
		and $before == $after
	else
		($before | length) > 0
		and ($before | map(.nodeName)) == ($after | map(.nodeName))
		and ([range(0; $before | length) as $i | $before[$i].bootID != $after[$i].bootID] | all)
	end
' >/dev/null; then
	echo "boot relation '$boot_relation' failed between $baseline and $candidate" >&2
	exit 1
fi

jq -n \
	--arg baseline "$baseline" \
	--arg candidate "$candidate" \
	--arg baselinePodState "$baseline_pod_state" \
	--arg candidatePodState "$candidate_pod_state" \
	--arg expectedBackend "$expected_backend" \
	--arg stateRelation "$state_relation" \
	--arg bootRelation "$boot_relation" \
	'{
		baseline: $baseline,
		candidate: $candidate,
		baselinePodState: $baselinePodState,
		candidatePodState: $candidatePodState,
		expectedBackend: $expectedBackend,
		stateRelation: $stateRelation,
		bootRelation: $bootRelation,
		result: "pass"
	}'
