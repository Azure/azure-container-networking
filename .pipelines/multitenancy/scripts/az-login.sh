#!/bin/bash

source ./.pipelines/multitenancy/scripts/utils.sh

function main() {
    # parse the arguments
    while [[ $# -gt 0 ]]; do
        key="$1"
        case $key in
            --service-principal)
                shift
                service_principal="$1"
                ;;
            --id-token)
                shift
                idtoken="$1"
                ;;
            --tenant)
                shift
                tenant="$1"
                ;;
            *)
                echo 1>&2 "unknown argument: $1"
                return 1
                ;;
        esac
        shift
    done
    export AZURE_CLIENT_ID="$service_principal"
    export AZURE_TENANT_ID="$tenant"
    if [[ -z "$tokenid" ]]; then
        echo >&2 "Password Auth Disabled. Please convert to workload identity."
    else
        workload_login "$service_principal" "$tenantid" "$idtoken"
    fi
}

# gets the service principal infomation for an app id
function get_sp_info() {
    local sp_appid
    sp_appid="${1}"
    utils::log az ad show --id "$sp_appid" --query id -otsv
    sp_oid="$cmd_out"
    utils::log az ad show --id "$sp_appid" --query name -otsv
    sp_name="$cmd_out"
}

function workload_login() {
    #export AZURE_AUTHORITY_HOST="$2"
    utils::setsecret AZURE_RESOURCE_BOOTSTRAP_CLIENT_ID "$1"
    utils::setvar AZURE_RESOURCE_BOOTSTRAP_CLIENT_TENANT_ID "$2"
    
    get_sp_info "$1"
    utils::setvar AZURE_RESOURCE_BOOTSTRAP_CLIENT_NAME "$sp_name"

    echo "$3" > wfi-token-file
    local wfi_filepath
    wfi_filepath=$(realpath wfi-token-file)
    export AZURE_FEDERATED_TOKEN_FILE="$wfi_filepath"
}
