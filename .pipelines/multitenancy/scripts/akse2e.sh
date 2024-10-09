#!/bin/bash

set -ex

az storage file download --account-name acnshared -s aksdev -p ./azureconfig-$SCENARIO.yaml --auth-mode login --enable-file-backup-request-intent
az storage file download --account-name acnshared -s aksdev -p ./aksdev --auth-mode login --enable-file-backup-request-intent

chmod +x ./aksdev
az aks get-credentials -g $RG --name $1 --file ~/.kube/config

cat azureconfig-$SCENARIO.yaml

# run e2e with vars
./aksdev e2e run -n "CNI Swift v2" --azureconfig azureconfig-$SCENARIO.yaml \
    --var resource_group=$RG \
    --var aks_cluster=$PODSUBNET_CLUSTER_NAME \
    --var vnet_name=$VNET \
    --var vnet_nodesubnet_name=$NODE_SUBNET_NAME \
    --var vnet_podsubnet_name=$POD_SUBNET_NAME \
    --var subnet_token=$SUBNET_TOKEN \
    --var storage_account_name=$STORAGE_ACC \
    --var nat_gateway_name=$NAT_GW_NAME \
    --var public_ip_name=$PODSUBNET_CLUSTER_NAME-ip \
    --var aks_multitenant_cluster=$1 \
    --var service_ip=$SERVICE_IP \
    --var client_id=$USER_ASSIGNED_CLIENT_ID \
    --var keep_env=$KEEP_ENV
