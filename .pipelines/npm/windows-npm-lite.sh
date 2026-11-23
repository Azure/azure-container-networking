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
    for tool in go jq kubectl kubelogin crane timeout sha256sum awk; do
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
        # Microsoft Go 1.26's default system crypto backend requires cgo; select its cgo-free OpenSSL backend.
        GOEXPERIMENT=ms_nocgo_opensslcrypto CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
            go build -mod=readonly -trimpath -o "$NPM_LITE_WORK_DIR/windows-npm-lite" ./test/npm/windows-lite
    )
}

load_owner() {
    local state="$NPM_LITE_WORK_DIR/owner.json"
    jq -e --arg build "$BUILD_BUILDID" --arg job "$SYSTEM_JOBID" --arg attempt "$SYSTEM_JOBATTEMPT" '
        .build == $build and .job == $job and .attempt == $attempt and
        .owner == (.build + ":" + .job + ":" + .attempt) and
        (.resourceGroup == ("e2e-npm-lite-" + .build) or
            .resourceGroup == ("e2e-npm-lite-upstream-" + .build)) and
        .cluster == "conformance-win-lite"
    ' "$state" > /dev/null || fail "resource ownership state does not match this job"
    resource_group="$(jq -er .resourceGroup "$state")"
    cluster="$(jq -er .cluster "$state")"
    owner="$(jq -er .owner "$state")"
}

run_nodepool_operation() {
    local deadline=$((SECONDS + $1)) remaining output status
    shift
    while true; do
        remaining=$((deadline - SECONDS))
        ((remaining > 0)) || fail "nodepool operation exceeded its provisioning deadline"
        if output="$(timeout --signal=INT --kill-after=10s "${remaining}s" az aks nodepool "$@" 2>&1)"; then
            [[ -z "$output" ]] || printf '%s\n' "$output"
            return 0
        else
            status=$?
        fi
        printf 'windows-npm-lite: nodepool %s exited %s\n%s\n' "$1" "$status" "$output" >&2
        if [[ "$status" -ne 1 ||
            "$output" != *"ERROR: (OperationNotAllowed) Operation is not allowed because there's an in-progress "* ||
            "$output" != *"on the managed cluster"* ]]; then
            return "$status"
        fi
        if ((deadline - SECONDS <= 10)); then
            fail "nodepool operation exhausted its provisioning deadline"
        fi
        printf 'windows-npm-lite: AKS is busy; retrying nodepool %s in 10 seconds\n' "$1" >&2
        sleep 10
    done
}

provision() {
    validate_inputs
    local resource_group="$1" cluster node_resource_group owner exists password
    [[ -x "$NPM_LITE_WORK_DIR/windows-npm-lite" && -f "$NPM_LITE_WORK_DIR/deployment.json" &&
        ! -e "$NPM_LITE_WORK_DIR/owner.json" ]] ||
        fail "prepared executor/deployment is missing or resource ownership already exists"
    cluster="conformance-win-lite"
    node_resource_group="MC_${resource_group}_${cluster}_${NPM_LITE_LOCATION}"
    [[ "${#node_resource_group}" -le 80 ]] || fail "default AKS node resource-group name exceeds 80 characters"
    owner="$BUILD_BUILDID:$SYSTEM_JOBID:$SYSTEM_JOBATTEMPT"
    exists="$(timeout 120s az group exists --name "$resource_group" --only-show-errors -o tsv)"
    [[ "$exists" == false ]] || fail "refusing to reuse a resource group"
    jq -n --arg build "$BUILD_BUILDID" --arg job "$SYSTEM_JOBID" --arg attempt "$SYSTEM_JOBATTEMPT" \
        --arg owner "$owner" --arg resourceGroup "$resource_group" --arg cluster "$cluster" \
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
        --enable-aad --enable-azure-rbac \
        --no-ssh-key --windows-admin-username e2eadmin --windows-admin-password "$password" \
        --network-plugin azure --vm-set-type VirtualMachineScaleSets \
        --node-vm-size Standard_D4s_v3 --node-count 1 \
        --only-show-errors -o none
    unset password
    run_nodepool_operation 900 update \
        --resource-group "$resource_group" --cluster-name "$cluster" --name nodepool1 \
        --node-taints CriticalAddonsOnly=true:NoSchedule --only-show-errors -o none
    run_nodepool_operation 1800 add \
        --resource-group "$resource_group" --cluster-name "$cluster" --name awin22 \
        --os-type Windows --os-sku Windows2022 --node-vm-size Standard_D4s_v3 --node-count 2 \
        --only-show-errors -o none
    export KUBECONFIG="$NPM_LITE_WORK_DIR/kubeconfig"
    [[ ! -e "$KUBECONFIG" ]] || fail "refusing to replace an existing kubeconfig"
    timeout 120s az aks get-credentials --resource-group "$resource_group" --name "$cluster" \
        --file "$KUBECONFIG" --only-show-errors -o none
    timeout 120s kubelogin convert-kubeconfig -l azurecli --kubeconfig "$KUBECONFIG"
    chmod 600 "$KUBECONFIG"
    timeout --signal=INT --kill-after=10s 120s kubectl --kubeconfig "$KUBECONFIG" --request-timeout=60s \
        apply -f "$NPM_LITE_WORK_DIR/deployment.json"
    timeout --signal=INT --kill-after=10s 21m kubectl --kubeconfig "$KUBECONFIG" \
        rollout status daemonset/azure-npm-win -n kube-system --timeout=20m
}

# The executor starts about ten kubectl processes per probe round within its 90-second
# convergence limit, so it uses one Entra ID token instead of an Azure CLI call per process.
use_static_token() {
    local config server_id response token expires_on
    config="$(timeout 60s kubectl config view --raw --kubeconfig "$KUBECONFIG" -o json)" ||
        fail "reading the kubeconfig failed"
    server_id="$(printf '%s' "$config" | jq -er '
        select((.users | length) == 1 and (.contexts | length) == 1 and
            .contexts[0].context.user == .users[0].name) |
        .users[0].user.exec.args as $args | ($args | index("--server-id")) as $i |
        select($i != null) | $args[$i + 1] | strings | select(test("\\A[a-f0-9-]{36}\\z"))
    ')" || fail "kubeconfig must have exactly one Azure CLI exec user"
    response="$(timeout 120s az account get-access-token --resource "$server_id" --only-show-errors -o json)" ||
        fail "cluster Entra ID token request failed"
    token="$(printf '%s' "$response" | jq -er '.accessToken | strings | select(length > 0)')" ||
        fail "cluster Entra ID token response has no access token"
    expires_on="$(printf '%s' "$response" | jq -er '.expires_on | numbers | floor')" ||
        fail "cluster Entra ID token response has no expiry"
    unset response
    # The token must outlive the executor's 35-minute deadline, with a minute of margin.
    ((expires_on - $(date +%s) >= 2160)) || fail "cluster Entra ID token expires before the executor deadline"
    # An inherited default ACL overrides the umask, so restrict the file before it holds the token.
    : > "$KUBECONFIG.tmp"
    chmod 600 -- "$KUBECONFIG.tmp"
    if ! printf '%s' "$config" | NPM_LITE_TOKEN="$token" jq '.users[0].user = {token: env.NPM_LITE_TOKEN}' \
        > "$KUBECONFIG.tmp"; then
        rm -f -- "$KUBECONFIG.tmp"
        fail "writing the token kubeconfig failed"
    fi
    unset token
    mv -f -- "$KUBECONFIG.tmp" "$KUBECONFIG"
}

provision_and_run() {
    provision "e2e-npm-lite-$BUILD_BUILDID"
    use_static_token
    exec "$NPM_LITE_WORK_DIR/windows-npm-lite" \
        --kubeconfig "$KUBECONFIG" --daemonset azure-npm-win --npm-namespace kube-system \
        --npm-image "$WINDOWS_NPM_IMAGE"
}

# With ApplyInBackground, Windows NPM starts its pod controller three minutes after startup
# (npm/npm.go) and applies no policy to pods until then, so specs must start after this marker.
wait_for_npm_bootup() {
    local deadline=$((SECONDS + 300)) start=$SECONDS names pod logs pending
    local marker='^I[0-9]{4}[[:blank:]]+[^[:blank:]]+[[:blank:]]+[0-9]+[[:blank:]]+dataplane\.go:[0-9]+\][[:blank:]]+\[DataPlane\] finished bootup phase$'
    local -a pods=()
    printf 'Waiting for the azure-npm-win pods to finish the NPM bootup phase\n'
    while true; do
        names="$(timeout 120s kubectl --kubeconfig "$KUBECONFIG" --request-timeout=60s get pods -n kube-system \
            -l k8s-app=azure-npm-win -o jsonpath='{.items[*].metadata.name}')" || names=""
        read -r -a pods <<< "$names"
        pending=""
        ((${#pods[@]} > 0)) || pending=" (no pods found)"
        for pod in "${pods[@]}"; do
            if [[ ! "$pod" =~ ^[a-z0-9-]+$ ]]; then
                printf 'windows-npm-lite: unexpected azure-npm-win pod name\n' >&2
                return 1
            fi
            logs="$(timeout 120s kubectl --kubeconfig "$KUBECONFIG" --request-timeout=60s \
                logs -n kube-system "$pod" 2> /dev/null)" || logs=""
            grep -Eq -- "$marker" <<< "${logs//$'\r'/}" || pending+=" $pod"
        done
        if [[ -z "$pending" ]]; then
            printf 'NPM bootup phase finished on all %d pods after %d seconds\n' "${#pods[@]}" "$((SECONDS - start))"
            return 0
        fi
        if ((SECONDS >= deadline)); then
            printf 'windows-npm-lite: NPM bootup phase did not finish within 5 minutes on:%s\n' "$pending" >&2
            return 1
        fi
        sleep 10
    done
}

run_upstream_netpol() {
    local focus="$1" dir="$NPM_LITE_WORK_DIR/upstream" resource_group cluster owner version fqdn digest actual
    local status=0
    load_owner
    mkdir -m 700 -- "$dir"
    version="$(timeout 120s az aks show --resource-group "$resource_group" --name "$cluster" \
        --query currentKubernetesVersion --only-show-errors -o tsv)"
    [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "cluster Kubernetes version is not a release version"
    fqdn="$(timeout 120s az aks show --resource-group "$resource_group" --name "$cluster" \
        --query fqdn --only-show-errors -o tsv)"
    [[ "$fqdn" =~ ^[a-z0-9.-]+$ ]] || fail "cluster API server FQDN is invalid"
    printf 'Running upstream NetworkPolicy specs from the official v%s test archive\n' "$version"
    timeout --signal=INT --kill-after=10s 10m curl -fsSL --proto '=https' --retry 3 \
        -o "$dir/kubernetes-test.tar.gz" "https://dl.k8s.io/v$version/kubernetes-test-linux-amd64.tar.gz"
    digest="$(timeout --signal=INT --kill-after=10s 120s curl -fsSL --proto '=https' --retry 3 \
        "https://dl.k8s.io/v$version/kubernetes-test-linux-amd64.tar.gz.sha256")"
    [[ "$digest" =~ ^[a-f0-9]{64}$ ]] || fail "published test archive checksum is invalid"
    actual="$(sha256sum "$dir/kubernetes-test.tar.gz")"
    [[ "${actual%% *}" == "$digest" ]] || fail "test archive checksum mismatch"
    tar -xzf "$dir/kubernetes-test.tar.gz" -C "$dir" --strip-components=3 kubernetes/test/bin/e2e.test
    rm -f -- "$dir/kubernetes-test.tar.gz"
    if ! wait_for_npm_bootup; then
        collect_upstream_diagnostics "$dir/report.json" || true
        return 1
    fi

    # Allowing one not-ready node tolerates the tainted Linux system node.
    KUBERNETES_SERVICE_HOST="$fqdn" KUBERNETES_SERVICE_PORT=443 \
        timeout --signal=INT --kill-after=10m 190m "$dir/e2e.test" \
        --provider=local --kubeconfig="$KUBECONFIG" --node-os-distro=windows --allowed-not-ready-nodes=1 \
        --ginkgo.focus="$focus" --ginkgo.timeout=180m --ginkgo.v \
        --ginkgo.json-report="$dir/report.json" || status=$?
    if ! python3 "$script_dir/windows-npm-lite-upstream.py" validate "$dir/report.json" && ((status == 0)); then
        status=1
    fi
    collect_upstream_diagnostics "$dir/report.json" || true
    return "$status"
}

# Best-effort artifact contents: Lite pod logs, cluster pod placement, and the spec report.
# Only the report is copied from the private work directory, so the kubeconfig stays unpublished.
collect_upstream_diagnostics() {
    local report="$1" out names pod
    local -a pods=()
    [[ "${SYSTEM_DEFAULTWORKINGDIRECTORY:-}" == /* ]] || return 0
    out="$SYSTEM_DEFAULTWORKINGDIRECTORY/npmLogs_windows-lite-upstream"
    mkdir -p -- "$out" || return 0
    timeout 120s kubectl --kubeconfig "$KUBECONFIG" --request-timeout=60s get nodes,pods -A -o wide \
        > "$out/nodes-and-pods.txt" 2>&1
    names="$(timeout 120s kubectl --kubeconfig "$KUBECONFIG" --request-timeout=60s get pods -n kube-system \
        -l k8s-app=azure-npm-win -o jsonpath='{.items[*].metadata.name}')"
    read -r -a pods <<< "$names"
    for pod in "${pods[@]}"; do
        [[ "$pod" =~ ^[a-z0-9-]+$ ]] || continue
        timeout 300s kubectl --kubeconfig "$KUBECONFIG" --request-timeout=240s logs -n kube-system "$pod" \
            --timestamps > "$out/$pod.log" 2>&1
        timeout 300s kubectl --kubeconfig "$KUBECONFIG" --request-timeout=240s logs -n kube-system "$pod" \
            --timestamps --previous > "$out/previous-$pod.log" 2> /dev/null || rm -f -- "$out/previous-$pod.log"
    done
    [[ ! -f "$report" ]] || cp -- "$report" "$out/report.json"
    printf '##vso[task.setvariable variable=NPM_LITE_UPSTREAM_LOGS]collected\n'
}

provision_and_run_upstream() {
    local tool focus
    for tool in curl tar python3; do
        command -v "$tool" > /dev/null || fail "required tool is unavailable: $tool"
    done
    focus="$(python3 "$script_dir/windows-npm-lite-upstream.py" focus)"
    provision "e2e-npm-lite-upstream-$BUILD_BUILDID"
    run_upstream_netpol "$focus"
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
    rm -f -- "$NPM_LITE_WORK_DIR/kubeconfig" "$NPM_LITE_WORK_DIR/kubeconfig.tmp"
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
        run-upstream) provision_and_run_upstream ;;
        cleanup) cleanup ;;
        cleanup-kubeconfig) cleanup_kubeconfig ;;
        *) fail "expected prepare, run, run-upstream, cleanup, or cleanup-kubeconfig" ;;
    esac
fi
