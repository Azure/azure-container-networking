#!/usr/bin/env bash
# Update cilium-family image tags in hack/aks/deploy.mk to the newest tags
# published on MCR. CILIUM tags stay within the current minor to avoid
# unintentional minor bumps; sidecar images pick the newest clean semver tag.
#
# Requires: skopeo, jq (both are already assumed by build/images.mk).
#
# Usage:
#   hack/scripts/update-cilium-versions.sh                  # write updates in place
#   hack/scripts/update-cilium-versions.sh --check          # dry-run, exit 1 if updates available
#   hack/scripts/update-cilium-versions.sh --minor 1.18     # also bump the CILIUM minor pin
#
# The CILIUM minor is normally derived from the current CILIUM_VERSION_TAG /
# EBPF_CILIUM_VERSION_TAG value. Two ways to bump it:
#   * pass --minor X.Y on the command line, or
#   * hand-edit those tags to a bare minor (e.g. `v1.18` or `1.18`); the
#     script will resolve the newest patch under that minor.

set -euo pipefail

DEPLOY_MK="hack/aks/deploy.mk"
if [[ ! -f "$DEPLOY_MK" ]]; then
    echo "must run from repo root ($DEPLOY_MK not found)" >&2
    exit 2
fi

check=0
minor_override=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --check) check=1; shift ;;
        --minor) minor_override="${2:-}"; shift 2 ;;
        --minor=*) minor_override="${1#--minor=}"; shift ;;
        *) echo "unknown argument: $1" >&2
           echo "usage: $0 [--check] [--minor X.Y]" >&2
           exit 2 ;;
    esac
done

# read a `VAR ?= <value>` line from deploy.mk
get_var() {
    grep -E "^$1[[:space:]]" "$DEPLOY_MK" | head -n1 | awk -F'?=' '{print $2}' | tr -d '[:space:]'
}

# newest tag from MCR matching regex, natural-sorted
latest_tag() {
    local image="$1" regex="$2"
    skopeo list-tags "docker://$image" 2>/dev/null | jq -r '.Tags[]' | grep -E "$regex" | sort -V | tail -n1
}

# rewrite `VAR ?= <value>` line in place, preserving indentation before the value
set_var() {
    local var="$1" new="$2"
    sed -i -E "s|^($var[[:space:]]*\?=[[:space:]]*)[^[:space:]#]+|\1$new|" "$DEPLOY_MK"
}

# derive a `vX.Y` minor pin from a deploy.mk value. accepts:
#   full tag `v1.17.7-250927` -> `v1.17`
#   bare minor `v1.18` or `1.18` -> `v1.18`
minor_of() {
    local v="$1"
    # strip a leading `v` if present, then take the first two dotted fields
    v="${v#v}"
    echo "v$(echo "$v" | awk -F. '{print $1"."$2}')"
}

# normalize a --minor CLI arg to `vX.Y`
if [[ -n "$minor_override" ]]; then
    minor_override="v${minor_override#v}"
    if [[ ! "$minor_override" =~ ^v[0-9]+\.[0-9]+$ ]]; then
        echo "--minor must be X.Y (got: $minor_override)" >&2
        exit 2
    fi
fi

CILIUM_MINOR="${minor_override:-$(minor_of "$(get_var CILIUM_VERSION_TAG)")}"
EBPF_CILIUM_MINOR="${minor_override:-$(minor_of "$(get_var EBPF_CILIUM_VERSION_TAG)")}"

# DIR / EBPF_CILIUM_DIR select which manifest tree gets applied. If we're
# picking a different minor than those declare, the manifests won't line up.
DIR_MINOR="v$(get_var DIR)"
EBPF_DIR_MINOR="v$(get_var EBPF_CILIUM_DIR)"
if [[ "$CILIUM_MINOR" != "$DIR_MINOR" ]]; then
    echo "  WARN CILIUM minor $CILIUM_MINOR != DIR $DIR_MINOR -- update DIR and add test/integration/manifests/cilium/$DIR_MINOR" >&2
fi
if [[ "$EBPF_CILIUM_MINOR" != "$EBPF_DIR_MINOR" ]]; then
    echo "  WARN EBPF CILIUM minor $EBPF_CILIUM_MINOR != EBPF_CILIUM_DIR $EBPF_DIR_MINOR" >&2
fi

updates=0
maybe_update() {
    local var="$1" image="$2" regex="$3"
    local current new
    current="$(get_var "$var")"
    new="$(latest_tag "$image" "$regex")" || true
    if [[ -z "$new" ]]; then
        printf "  ! %-40s no matching tags at %s\n" "$var" "$image"
        return
    fi
    if [[ "$current" == "$new" ]]; then
        printf "  = %-40s %s\n" "$var" "$current"
        return
    fi
    printf "  ~ %-40s %s -> %s\n" "$var" "$current" "$new"
    updates=$((updates + 1))
    if (( check == 0 )); then
        set_var "$var" "$new"
    fi
}

# clean semver, optional single `-N` suffix (excludes `-g<sha>` dev builds)
SEMVER='^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9]+)?$'

echo "resolving latest tags from MCR..."

maybe_update CILIUM_VERSION_TAG \
    "$(get_var CILIUM_IMAGE_REGISTRY)/cilium/cilium" \
    "^${CILIUM_MINOR//./\\.}\\.[0-9]+-[0-9]+$"

maybe_update EBPF_CILIUM_VERSION_TAG \
    "$(get_var EBPF_CILIUM_IMAGE_REGISTRY)/cilium/cilium" \
    "^${EBPF_CILIUM_MINOR//./\\.}\\.[0-9]+-[0-9]+$"

maybe_update CILIUM_LOG_COLLECTOR_VERSION_TAG \
    "$(get_var CILIUM_LOG_COLLECTOR_IMAGE_REGISTRY)/cilium-log-collector" \
    "$SEMVER"

maybe_update AZURE_IPTABLES_MONITOR_TAG \
    "$(get_var AZURE_IPTABLES_MONITOR_IMAGE_REGISTRY)/azure-iptables-monitor" \
    "$SEMVER"

maybe_update AZURE_IP_MASQ_MERGER_TAG \
    "$(get_var AZURE_IP_MASQ_MERGER_IMAGE_REGISTRY)/azure-ip-masq-merger" \
    "$SEMVER"

maybe_update IPV6_HP_BPF_VERSION \
    "$(get_var IPV6_IMAGE_REGISTRY)/ipv6-hp-bpf" \
    "$SEMVER"

if (( updates == 0 )); then
    echo "everything up to date"
    exit 0
fi

if (( check == 1 )); then
    echo "$updates update(s) available -- run 'make cilium-versions' to apply"
    exit 1
fi
echo "wrote $updates update(s) to $DEPLOY_MK"
