#!/usr/bin/env bash
set +x
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/../.." && pwd)"

fail() {
    printf 'windows-npm-lite: %s\n' "$*" >&2
    exit 1
}

legacy_admin_fixture() {
    local value
    if ! value="$(awk '
        {
            for (i = 1; i <= NF; i++) {
                if ($i == "--windows-admin-password") {
                    print $(i + 1)
                    count++
                }
            }
        }
        END { if (count != 1) exit 1 }
    ' "$1")"; then
        fail "legacy pipeline must contain exactly one disposable Windows-admin fixture"
    fi
    [[ "$value" =~ ^[a-zA-Z0-9!@#%_+=:.,/-]+$ ]] ||
        fail "legacy Windows-admin fixture is no longer a single supported literal argument"
    printf '%s' "$value"
}

validate_inputs() {
    [[ "${WINDOWS_NPM_IMAGE:-}" =~ ^[a-z0-9.-]+(:[0-9]+)?/[a-z0-9]+([._/-][a-z0-9]+)*(:[A-Za-z0-9_][A-Za-z0-9_.-]{0,127})?@sha256:[a-f0-9]{64}$ ]] ||
        fail "windowsNpmImage must be a complete immutable child reference"
    [[ "${BUILD_BUILDID:-}" =~ ^[1-9][0-9]*$ ]] ||
        fail "actual build ID is required for resource ownership"
    [[ "${SYSTEM_JOBID:-}" =~ ^[a-fA-F0-9-]{36}$ && "${SYSTEM_JOBATTEMPT:-}" =~ ^[1-9][0-9]*$ ]] ||
        fail "actual job ID and attempt are required"
    [[ "${NPM_LITE_WORK_DIR:-}" == /* && "${NPM_LITE_LOCATION:-}" =~ ^[a-z0-9]+$ ]] ||
        fail "a private absolute work directory and Azure location are required"
    legacy_admin_fixture "$script_dir/npm-cyc-win-tests-latest-release.yaml" > /dev/null
}

verify_metadata() {
    local manifest="$1" config="$2" manifest_hash config_hash config_size
    [[ -f "$manifest" && -f "$config" ]] || fail "candidate metadata must be regular files"
    [[ "$(wc -c < "$manifest")" -le 4194304 && "$(wc -c < "$config")" -le 4194304 ]] ||
        fail "candidate metadata exceeds 4 MiB"
    manifest_hash="$(sha256sum "$manifest")"
    config_hash="$(sha256sum "$config")"
    config_size="$(wc -c < "$config")"
    [[ "${manifest_hash%% *}" == "${WINDOWS_NPM_IMAGE##*@sha256:}" ]] ||
        fail "raw candidate manifest digest mismatch"
    jq -se --arg digest "sha256:${config_hash%% *}" --argjson size "$config_size" '
        length == 1 and (.[0] |
            .schemaVersion == 2 and
            (.mediaType == "application/vnd.oci.image.manifest.v1+json" or
             .mediaType == "application/vnd.docker.distribution.manifest.v2+json") and
            (.manifests == null or (.manifests | type == "array" and length == 0)) and
            (.layers | type == "array" and length > 0) and
            .config.digest == $digest and .config.size == $size)
    ' "$manifest" > /dev/null || fail "candidate child/config descriptor mismatch"
    jq -ser '
        select(length == 1) | .[0] |
        select(.os == "windows" and .architecture == "amd64") |
        ."os.version" | strings | select(test("\\A10\\.0\\.20348\\.[0-9]+\\z"))
    ' "$config" || fail "candidate config must identify the exact Windows2022/amd64 image revision"
}

prepare() {
    for tool in go jq kubectl crane timeout sha256sum awk; do
        command -v "$tool" > /dev/null || fail "required tool is unavailable: $tool"
    done
    validate_inputs
    mkdir -m 700 -- "$NPM_LITE_WORK_DIR"
    mkdir -m 700 -- "$NPM_LITE_WORK_DIR/registry"

    # Public nodes must pull the candidate without delegated staging credentials.
    export DOCKER_CONFIG="$NPM_LITE_WORK_DIR/registry"
    printf '{"auths":{}}\n' > "$DOCKER_CONFIG/config.json"
    if ! timeout --signal=INT --kill-after=10s 120s crane manifest "$WINDOWS_NPM_IMAGE" |
        head -c 4194305 > "$NPM_LITE_WORK_DIR/manifest.json"; then
        fail "fetching candidate manifest failed or exceeded its time/size bound"
    fi
    if ! timeout --signal=INT --kill-after=10s 120s crane config "$WINDOWS_NPM_IMAGE" |
        head -c 4194305 > "$NPM_LITE_WORK_DIR/image-config.json"; then
        fail "fetching candidate config failed or exceeded its time/size bound"
    fi
    local windows_os_version
    windows_os_version="$(verify_metadata "$NPM_LITE_WORK_DIR/manifest.json" "$NPM_LITE_WORK_DIR/image-config.json")"
    printf 'Verified Windows/amd64 candidate OS version: %s\n' "$windows_os_version"

    jq -s '.[0] * .[1]' "$script_dir/windows-npm-config.json" \
        "$repo_root/test/npm/windows-lite/config.json" > "$NPM_LITE_WORK_DIR/npm-config.json"
    jq --arg image "$WINDOWS_NPM_IMAGE" --rawfile config "$NPM_LITE_WORK_DIR/npm-config.json" '
        (.items[] | select(.kind == "DaemonSet") | .spec.template.spec.containers[0].image) = $image |
        (.items[] | select(.kind == "ConfigMap") | .data["azure-npm.json"]) = $config
    ' "$script_dir/windows-npm-lite.json" > "$NPM_LITE_WORK_DIR/deployment.json"
    (
        cd "$repo_root"
        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=readonly -trimpath \
            -o "$NPM_LITE_WORK_DIR/windows-npm-lite" ./test/npm/windows-lite
    )
}

load_owner() {
    local state="$NPM_LITE_WORK_DIR/owner.json"
    jq -e --arg build "$BUILD_BUILDID" --arg job "$SYSTEM_JOBID" --arg attempt "$SYSTEM_JOBATTEMPT" '
        .build == $build and .job == $job and .attempt == $attempt and
        (.owner | test("^[a-f0-9-]{36}$")) and
        .resourceGroup == ("e2e-npm-lite-" + .build + "-" + .owner) and
        .cluster == ("npm-lite-" + .build + "-" + (.owner[0:8]))
    ' "$state" > /dev/null || fail "resource ownership state does not match this job"
    resource_group="$(jq -er .resourceGroup "$state")"
    cluster="$(jq -er .cluster "$state")"
    owner="$(jq -er .owner "$state")"
}

provision_and_run() {
    validate_inputs
    local token resource_group cluster owner exists password
    [[ -x "$NPM_LITE_WORK_DIR/windows-npm-lite" && -f "$NPM_LITE_WORK_DIR/deployment.json" &&
        ! -e "$NPM_LITE_WORK_DIR/owner.json" ]] ||
        fail "prepared executor/deployment is missing or resource ownership already exists"
    token="$(cat /proc/sys/kernel/random/uuid)"
    resource_group="e2e-npm-lite-$BUILD_BUILDID-$token"
    cluster="npm-lite-$BUILD_BUILDID-${token:0:8}"
    exists="$(timeout 120s az group exists --name "$resource_group" --only-show-errors -o tsv)"
    [[ "$exists" == false ]] || fail "refusing to reuse a resource group"
    jq -n --arg build "$BUILD_BUILDID" --arg job "$SYSTEM_JOBID" --arg attempt "$SYSTEM_JOBATTEMPT" \
        --arg owner "$token" --arg resourceGroup "$resource_group" --arg cluster "$cluster" \
        '{build:$build, job:$job, attempt:$attempt, owner:$owner, resourceGroup:$resourceGroup, cluster:$cluster}' \
        > "$NPM_LITE_WORK_DIR/owner.json"
    load_owner
    printf 'Windows Lite test resource group: %s\n' "$resource_group"
    timeout 120s az group create --name "$resource_group" --location "$NPM_LITE_LOCATION" \
        --tags "acn-npm-lite-owner=$owner" --only-show-errors -o none

    # Reuse the legacy public disposable-test fixture without copying it or creating a credential.
    password="$(legacy_admin_fixture "$script_dir/npm-cyc-win-tests-latest-release.yaml")"
    timeout --signal=INT --kill-after=10s 45m az aks create \
        --resource-group "$resource_group" --name "$cluster" --location "$NPM_LITE_LOCATION" \
        --no-ssh-key --windows-admin-username e2eadmin --windows-admin-password "$password" \
        --network-plugin azure --vm-set-type VirtualMachineScaleSets \
        --node-vm-size Standard_D4s_v3 --node-count 1 \
        --only-show-errors -o none
    unset password
    timeout --signal=INT --kill-after=10s 15m az aks nodepool update \
        --resource-group "$resource_group" --cluster-name "$cluster" --name nodepool1 \
        --node-taints CriticalAddonsOnly=true:NoSchedule --only-show-errors -o none
    timeout --signal=INT --kill-after=10s 30m az aks nodepool add \
        --resource-group "$resource_group" --cluster-name "$cluster" --name awin22 \
        --os-type Windows --os-sku Windows2022 --node-vm-size Standard_D4s_v3 --node-count 2 \
        --only-show-errors -o none
    export KUBECONFIG="$NPM_LITE_WORK_DIR/kubeconfig"
    [[ ! -e "$KUBECONFIG" ]] || fail "refusing to replace an existing kubeconfig"
    timeout 120s az aks get-credentials --resource-group "$resource_group" --name "$cluster" \
        --file "$KUBECONFIG" --only-show-errors -o none
    chmod 600 "$KUBECONFIG"
    timeout --signal=INT --kill-after=10s 120s kubectl --kubeconfig "$KUBECONFIG" --request-timeout=60s \
        apply -f "$NPM_LITE_WORK_DIR/deployment.json"
    timeout --signal=INT --kill-after=10s 21m kubectl --kubeconfig "$KUBECONFIG" \
        rollout status daemonset/azure-npm-win -n kube-system --timeout=20m
    exec "$NPM_LITE_WORK_DIR/windows-npm-lite" \
        --kubeconfig "$KUBECONFIG" --daemonset azure-npm-win --npm-namespace kube-system \
        --npm-image "$WINDOWS_NPM_IMAGE"
}

cleanup_resources() {
    local resource_group cluster owner exists observed
    if [[ ! -f "$NPM_LITE_WORK_DIR/owner.json" ]]; then
        return
    fi
    load_owner
    exists="$(timeout 120s az group exists --name "$resource_group" --only-show-errors -o tsv)"
    if [[ "$exists" == false ]]; then
        return
    fi
    [[ "$exists" == true ]] || fail "resource-group existence could not be established"
    observed="$(timeout 120s az group show --name "$resource_group" \
        --query 'tags."acn-npm-lite-owner"' --only-show-errors -o tsv)"
    [[ "$observed" == "$owner" ]] || fail "refusing to delete an unowned resource group"
    timeout --signal=INT --kill-after=10s 23m az group delete \
        --name "$resource_group" --yes --only-show-errors
    exists="$(timeout 120s az group exists --name "$resource_group" --only-show-errors -o tsv)"
    [[ "$exists" == false ]] || fail "test resource-group cleanup was not confirmed"
}

cleanup_kubeconfig() {
    [[ "${NPM_LITE_WORK_DIR:-}" == /* ]] || fail "cleanup requires the private work directory"
    [[ -f "$NPM_LITE_WORK_DIR/owner.json" ]] || return 0
    local resource_group cluster owner
    load_owner
    rm -f -- "$NPM_LITE_WORK_DIR/kubeconfig"
}

cleanup() {
    [[ "${NPM_LITE_WORK_DIR:-}" == /* ]] || fail "cleanup requires the private work directory"
    trap cleanup_kubeconfig EXIT
    cleanup_resources
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    case "${1:-}" in
        prepare) prepare ;;
        run) provision_and_run ;;
        cleanup) cleanup ;;
        cleanup-kubeconfig) cleanup_kubeconfig ;;
        *) fail "expected prepare, run, cleanup, or cleanup-kubeconfig" ;;
    esac
fi
