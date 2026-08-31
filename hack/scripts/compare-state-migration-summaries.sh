#!/usr/bin/env bash

set -euo pipefail

if [[ $# -ne 12 ]]; then
	echo "usage: $0 <baseline> <candidate> <expected-backend> <expected-authority> <expected-schema> <state-relation> <boot-relation> <pod-relation> <baseline-pod-state> <candidate-pod-state> <baseline-metadata-dir> <candidate-metadata-dir>" >&2
	exit 2
fi

baseline=$1
candidate=$2
expected_backend=$3
expected_authority=$4
expected_schema=$5
state_relation=$6
boot_relation=$7
pod_relation=$8
baseline_pod_state=$9
candidate_pod_state=${10}
baseline_metadata_dir=${11}
candidate_metadata_dir=${12}

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
if [[ "$pod_relation" == "inherit" ]]; then
	pod_relation=$state_relation
fi
case "$pod_relation" in
none | exact | identity | changed) ;;
*)
	echo "unsupported pod relation: $pod_relation" >&2
	exit 2
	;;
esac
if [[ ! "$expected_schema" =~ ^[0-9]+$ ]]; then
	echo "expected schema must be an unsigned integer: $expected_schema" >&2
	exit 2
fi

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
baseline_backend=$(jq -er '.stateBackend | strings' "$baseline") || {
	echo "baseline summary backend is missing or malformed: $baseline" >&2
	exit 1
}
baseline_abs=$(realpath "$baseline")
candidate_abs=$(realpath "$candidate")
if ! (
	cd "$repo_root"
	go run ./test/validate/cmd/summarydiff \
		-baseline "$baseline_abs" \
		-candidate "$baseline_abs" \
		-expected-backend "$baseline_backend" >/dev/null
); then
	echo "baseline summary failed strict Go validation: $baseline" >&2
	exit 1
fi
if ! (
	cd "$repo_root"
	go run ./test/validate/cmd/summarydiff \
		-baseline "$candidate_abs" \
		-candidate "$candidate_abs" \
		-expected-backend "$expected_backend" >/dev/null
); then
	echo "candidate summary failed strict Go validation: $candidate" >&2
	exit 1
fi

validate_summary() {
	local path=$1
	local backend=$2
	jq -e --arg backend "$backend" '
		.stateBackend == $backend
		and (.checks | type == "array" and length > 0)
		and ([.checks[] | [.checkName, .nodeName]] | unique | length) == (.checks | length)
		and all(.checks[];
			(.checkName | type == "string" and length > 0)
			and (.nodeName | type == "string" and length > 0)
			and (.livePodCount | type == "number" and . >= 0 and floor == .)
			and (.expected | type == "array")
			and (.actual | type == "array")
			and (if .livePodCount > 0 then (.expected | length) > 0 else true end)
			and ([.expected[] | [.podID, .ip]] | unique | length) == (.expected | length)
			and ([.actual[] | [.podID, .ip]] | unique | length) == (.actual | length)
			and all(.expected[], .actual[];
				(.podID | type == "string" and length > 0)
				and (.ip | type == "string" and test("^[0-9A-Fa-f:.]+$"))
			)
			and ([.expected[] | .ip] | unique | length) == (.expected | length)
			and ([.actual[] | .ip] | unique | length) == (.actual | length)
			and ([.expected[] | {podID, ip}] | sort_by(.podID, .ip))
				== ([.actual[] | {podID, ip}] | sort_by(.podID, .ip))
		)
	' "$path" >/dev/null
}

if ! validate_summary "$baseline" "$(jq -r '.stateBackend // empty' "$baseline")"; then
	echo "baseline summary failed strict validation: $baseline" >&2
	exit 1
fi
if ! validate_summary "$candidate" "$expected_backend"; then
	echo "candidate summary failed strict validation: $candidate" >&2
	exit 1
fi

if ! jq -e -n \
	--slurpfile baseline "$baseline" \
	--slurpfile candidate "$candidate" \
	--arg relation "$state_relation" '
	def normalized($summary):
		[
			$summary.checks[]
			| {
				checkName,
				nodeName,
				livePodCount,
				expected: ([.expected[] | {podID, ip}] | sort_by(.podID, .ip)),
				actual: ([.actual[] | {podID, ip}] | sort_by(.podID, .ip))
			}
		]
		| sort_by(.checkName, .nodeName);
	(normalized($baseline[0])) as $before
	| (normalized($candidate[0])) as $after
	| if $relation == "none" then true
	  elif $relation == "exact" then $before == $after
	  else
		($before | map([.checkName, .nodeName])) == ($after | map([.checkName, .nodeName]))
		and ([range(0; $before | length) as $i
			| $after[$i].livePodCount >= $before[$i].livePodCount] | all)
		and $before != $after
	  end
' >/dev/null; then
	echo "state relation '$state_relation' failed between $baseline and $candidate" >&2
	exit 1
fi

validate_pods='
	type == "array"
	and length > 0
	and ([.[] | [.namespace, .name]] | unique | length) == length
	and all(.[];
		(.namespace | type == "string" and length > 0)
		and (.name | type == "string" and length > 0)
		and (.nodeName | type == "string" and length > 0)
		and .phase == "Running"
		and (.podIPs | type == "array" and length > 0)
		and ([.podIPs[]] | unique | length) == (.podIPs | length)
	)
'
if ! jq -e "$validate_pods" "$baseline_pod_state" >/dev/null ||
	! jq -e "$validate_pods" "$candidate_pod_state" >/dev/null; then
	echo "pod state failed strict validation" >&2
	exit 1
fi

if ! jq -e -n \
	--slurpfile baselinePods "$baseline_pod_state" \
	--slurpfile candidatePods "$candidate_pod_state" \
	--arg relation "$pod_relation" '
	def normalized($pods):
		[$pods[] | {namespace, name, nodeName, phase, podIPs: (.podIPs | sort)}]
		| sort_by(.namespace, .name);
	def identities($pods):
		normalized($pods) | map(del(.podIPs));
	(normalized($baselinePods[0])) as $before
	| (normalized($candidatePods[0])) as $after
	| if $relation == "none" then true
	  elif $relation == "exact" then $before == $after
	  elif $relation == "identity" then identities($baselinePods[0]) == identities($candidatePods[0])
	  else ($after | length) >= ($before | length) and $before != $after
	  end
' >/dev/null; then
	echo "pod relation '$pod_relation' failed between $baseline_pod_state and $candidate_pod_state" >&2
	exit 1
fi

metadata_json() {
	local directory=$1
	if [[ ! -d "$directory" ]]; then
		return 1
	fi
	local files=("$directory"/*.json)
	if [[ ! -e "${files[0]}" ]]; then
		return 1
	fi
	jq -s 'sort_by(.nodeName)' "${files[@]}"
}

if [[ "$expected_backend" == "bolt" ]]; then
	candidate_metadata=$(metadata_json "$candidate_metadata_dir") || {
		echo "candidate persistent metadata is missing: $candidate_metadata_dir" >&2
		exit 1
	}
	if ! jq -e \
		--arg authority "$expected_authority" \
		--argjson schema "$expected_schema" '
		type == "array"
		and length > 0
		and ([.[].nodeName] | unique | length) == length
		and all(.[];
			(.nodeName | type == "string" and length > 0)
			and .status.backend == "bbolt"
			and .status.authority == $authority
			and .status.schemaVersion == $schema
			and .status.generation > 0
			and .status.bootPresent == true
			and .status.storagePresent == true
			and .status.databaseBytes > 0
			and .status.invariantStatus == "healthy"
			and .snapshot.Metadata.authority == $authority
			and .snapshot.Metadata.schemaVersion == $schema
			and .snapshot.Metadata.generation == .status.generation
			and ((.snapshot.Metadata.bootID // "") | length) > 0
		)
	' <<<"$candidate_metadata" >/dev/null; then
		echo "candidate persistent metadata failed strict validation" >&2
		exit 1
	fi

	summary_nodes=$(jq -c '[.checks[].nodeName] | unique | sort' "$candidate")
	metadata_nodes=$(jq -c '[.[].nodeName] | unique | sort' <<<"$candidate_metadata")
	if [[ "$summary_nodes" != "$metadata_nodes" ]]; then
		echo "persistent metadata node coverage does not match validation summary" >&2
		exit 1
	fi
fi

if [[ "$boot_relation" != "none" ]]; then
	baseline_boot_state="$(dirname "$baseline_pod_state")/node-boots.json"
	candidate_boot_state="$(dirname "$candidate_pod_state")/node-boots.json"
	if [[ ! -s "$baseline_boot_state" || ! -s "$candidate_boot_state" ]]; then
		echo "node boot evidence is missing for boot relation '$boot_relation'" >&2
		exit 1
	fi
	validate_boots='
		type == "array"
		and length > 0
		and ([.[].nodeName] | unique | length) == length
		and all(.[];
			(.nodeName | type == "string" and length > 0)
			and (.bootID | type == "string" and length > 0)
		)
	'
	if ! jq -e "$validate_boots" "$baseline_boot_state" >/dev/null ||
		! jq -e "$validate_boots" "$candidate_boot_state" >/dev/null; then
		echo "node boot evidence failed strict validation" >&2
		exit 1
	fi
	if ! jq -e -n \
		--slurpfile before "$baseline_boot_state" \
		--slurpfile after "$candidate_boot_state" \
		--arg relation "$boot_relation" '
		($before[0] | sort_by(.nodeName)) as $a
		| ($after[0] | sort_by(.nodeName)) as $b
		| ($a | length) > 0
		and ($a | map(.nodeName)) == ($b | map(.nodeName))
		and if $relation == "same" then $a == $b
		    else [range(0; $a | length) as $i | $a[$i].bootID != $b[$i].bootID] | all
		    end
	' >/dev/null; then
		echo "boot relation '$boot_relation' failed between metadata captures" >&2
		exit 1
	fi
fi

jq -n \
	--arg baseline "$baseline" \
	--arg candidate "$candidate" \
	--arg expectedBackend "$expected_backend" \
	--arg stateRelation "$state_relation" \
	--arg bootRelation "$boot_relation" \
	--arg podRelation "$pod_relation" \
	'{
		baseline: $baseline,
		candidate: $candidate,
		expectedBackend: $expectedBackend,
		stateRelation: $stateRelation,
		bootRelation: $bootRelation,
		podRelation: $podRelation,
		result: "pass"
	}'
