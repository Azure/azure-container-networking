#!/bin/bash

set -ex
source ./.pipelines/multitenancy/scripts/utils.sh

function main() {
    # parse the arguments
    while [[ $# -gt 0 ]]; do
        key="$1"
        case "$key" in
        --test-dir)
            shift
            test_dir="$1"
            ;;
        --mt-test-cluster)
            shift
            mt_test_cluster="$1"
            ;;
        --scenario)
            shift
            scenario="$1"
            ;;            
        *)
            echo 1>&2 "unknown argument: $1"
            return 1
            ;;
        esac
        shift
    done
    run_tests "$test_dir" "$mt_test_cluster" "$scenario"
}

function run_tests() {
    local test_dir
    test_dir="${1}"
    local mt_test_cluster
    mt_test_cluster="${2}"
    local scenario
    scenario="${3}"

    STEP="runAKSe2e"
    cd $test_dir

    #get configvars
    export_envVars $scenario

    if [[ "$ENABLED" == "false" ]]; then
        echo "scenario: $scenario skipped"
        return 0
    fi
    
    # Get the OIDC Issuer URL
    export AKS_OIDC_ISSUER="$(az aks show -n "$mt_test_cluster" -g "$RG" --query "oidcIssuerProfile.issuerUrl" -otsv)"

    # Federate the identity
    az identity federated-credential create \
        --name "$FEDERATED_IDENTITY_CREDENTIAL_PREFIX-$mt_test_cluster" \
        --identity-name "$USER_ASSIGNED_IDENTITY_NAME" \
        --resource-group "$RG" \
        --issuer "$AKS_OIDC_ISSUER" \
        --subject system:serviceaccount:mtpod-to-service-endpoint:workload-identity-sa

    # Get identity client ID
    export USER_ASSIGNED_CLIENT_ID=$(az identity show --resource-group "$RG" --name "$USER_ASSIGNED_IDENTITY_NAME" --query 'clientId' -o tsv)
    
    # Run aks e2e test suite
    chmod +x ./akse2e.sh
    ./akse2e.sh $mt_test_cluster && passed="true" || passed="false"

    #Clean up user-assigned identity
    az identity federated-credential delete --name "$FEDERATED_IDENTITY_CREDENTIAL_PREFIX-$mt_test_cluster" --identity-name "$USER_ASSIGNED_IDENTITY_NAME" --resource-group "$RG" --yes

    if [[ "$passed" == "true" ]]; then
        echo "Tests passed"
    else
        echo "Tests failed"
        return 1
    fi
}

main $@
